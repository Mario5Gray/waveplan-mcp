package swim

import "fmt"

// ActionKind is the high-level next-step decision category.
type ActionKind string

const (
	ActionReady   ActionKind = "ready"
	ActionBlocked ActionKind = "blocked"
	ActionDone    ActionKind = "done"
	ActionDrift   ActionKind = "drift"
)

// ResolutionCode is a machine-readable resolver outcome code.
type ResolutionCode string

const (
	ResolutionReady                 ResolutionCode = "ready"
	ResolutionBlockedPrecondition   ResolutionCode = "blocked_precondition"
	ResolutionBlockedPendingSidecar ResolutionCode = "blocked_pending_sidecar"
	ResolutionCursorDrift           ResolutionCode = "cursor_drift"
	ResolutionUnknownPending        ResolutionCode = "unknown_pending"
	ResolutionDone                  ResolutionCode = "done"
	ResolutionBadCursor             ResolutionCode = "bad_cursor"
)

// Decision is the read-only resolver result for current cursor row.
type Decision struct {
	Action ActionKind     `json:"action"`
	Row    ScheduleRow    `json:"row,omitempty"`
	Code   ResolutionCode `json:"code"`
	Reason string         `json:"reason,omitempty"`
	Cursor int            `json:"cursor"`
}

var statusOrder = map[Status]int{
	StatusAvailable:   0,
	StatusTaken:       1,
	StatusReviewTaken: 2,
	StatusReviewEnded: 3,
	StatusCompleted:   4,
}

// ResolveNext returns what should happen next for journal.cursor without
// mutating journal or state.
func ResolveNext(sched *Schedule, journal *Journal, snap *StateSnapshot) Decision {
	n := len(sched.Execution)
	cur := journal.Cursor
	if cur < 0 || cur > n {
		return Decision{
			Action: ActionBlocked,
			Code:   ResolutionBadCursor,
			Cursor: cur,
			Reason: fmt.Sprintf("bad_cursor: cursor=%d execution_len=%d", cur, n),
		}
	}
	if ev, ok := firstUnknownPending(journal, cur); ok {
		return Decision{
			Action: ActionBlocked,
			Code:   ResolutionUnknownPending,
			Cursor: cur,
			Reason: fmt.Sprintf("unknown_pending: event_id=%s step_id=%s seq=%d", ev.EventID, ev.StepID, ev.Seq),
		}
	}
	if cur == n {
		return Decision{Action: ActionDone, Code: ResolutionDone, Cursor: cur}
	}

	row := sched.Execution[cur]
	if pendingStepID, pendingAction, ok := pendingSidecarStepForTask(sched, cur, row); ok {
		return Decision{
			Action: ActionBlocked,
			Row:    row,
			Code:   ResolutionBlockedPendingSidecar,
			Cursor: cur,
			Reason: fmt.Sprintf(
				"pending_sidecar: action=%s task=%s pending_step_id=%s pending_action=%s",
				row.Action,
				row.TaskID,
				pendingStepID,
				pendingAction,
			),
		}
	}
	e := Evaluate(row, snap)
	if e.Allowed {
		return Decision{Action: ActionReady, Row: row, Code: ResolutionReady, Cursor: cur}
	}

	if e.Code == FailureCodePreconditionUnmet {
		actual := Status("<nil>")
		if snap != nil {
			actual = snap.StatusOf(row.TaskID)
		}
		required := Status(row.Requires.TaskStatus)
		aRank, aOK := statusOrder[actual]
		rRank, rOK := statusOrder[required]
		if aOK && rOK && aRank > rRank {
			return Decision{
				Action: ActionDrift,
				Row:    row,
				Code:   ResolutionCursorDrift,
				Cursor: cur,
				Reason: fmt.Sprintf("cursor_drift: action=%s requires=%s actual=%s (state ahead)", row.Action, required, actual),
			}
		}
		return Decision{Action: ActionBlocked, Row: row, Code: ResolutionBlockedPrecondition, Cursor: cur, Reason: e.Reason}
	}

	return Decision{Action: ActionBlocked, Row: row, Code: ResolutionCode(e.Code), Cursor: cur, Reason: e.Reason}
}

func firstUnknownPending(journal *Journal, cursor int) (JournalEvent, bool) {
	if journal == nil {
		return JournalEvent{}, false
	}
	for _, ev := range journal.Events {
		// seq is 1-based, cursor is 0-based index into execution rows.
		if ev.Outcome == "unknown" && cursor < ev.Seq {
			return ev, true
		}
	}
	return JournalEvent{}, false
}

func pendingSidecarStepForTask(sched *Schedule, cursor int, current ScheduleRow) (string, string, bool) {
	if sched == nil || (current.Action != "end_review" && current.Action != "finish") {
		return "", "", false
	}
	for i := cursor + 1; i < len(sched.Execution); i++ {
		row := sched.Execution[i]
		if row.TaskID != current.TaskID || row.Source != scheduleRowSourceReviewSidecar {
			continue
		}
		return row.StepID, row.Action, true
	}
	return "", "", false
}
