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
	ResolutionReady               ResolutionCode = "ready"
	ResolutionBlockedPrecondition ResolutionCode = "blocked_precondition"
	ResolutionCursorDrift         ResolutionCode = "cursor_drift"
	ResolutionDone                ResolutionCode = "done"
	ResolutionBadCursor           ResolutionCode = "bad_cursor"
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
	if cur == n {
		return Decision{Action: ActionDone, Code: ResolutionDone, Cursor: cur}
	}

	row := sched.Execution[cur]
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
