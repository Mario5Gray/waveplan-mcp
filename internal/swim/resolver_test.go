package swim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveNext_Table(t *testing.T) {
	taskID := "T9.9"
	sched := &Schedule{SchemaVersion: 2, Execution: []ScheduleRow{
		{Seq: 1, StepID: "S1_T9.9_implement", TaskID: taskID, Action: "implement", Requires: StatusWrapper{TaskStatus: string(StatusAvailable)}, Produces: StatusWrapper{TaskStatus: string(StatusTaken)}},
		{Seq: 2, StepID: "S1_T9.9_review", TaskID: taskID, Action: "review", Requires: StatusWrapper{TaskStatus: string(StatusTaken)}, Produces: StatusWrapper{TaskStatus: string(StatusReviewTaken)}},
		{Seq: 3, StepID: "S1_T9.9_end_review", TaskID: taskID, Action: "end_review", Requires: StatusWrapper{TaskStatus: string(StatusReviewTaken)}, Produces: StatusWrapper{TaskStatus: string(StatusReviewEnded)}},
	}}

	cases := []struct {
		name     string
		cursor   int
		live     Status
		wantAct  ActionKind
		wantCode ResolutionCode
		wantStep string
		wantRow  bool
	}{
		{"ready at start", 0, StatusAvailable, ActionReady, ResolutionReady, "S1_T9.9_implement", true},
		{"blocked precondition", 1, StatusAvailable, ActionBlocked, ResolutionBlockedPrecondition, "S1_T9.9_review", true},
		{"cursor drift", 0, StatusTaken, ActionDrift, ResolutionCursorDrift, "S1_T9.9_implement", true},
		{"done at end", 3, StatusAvailable, ActionDone, ResolutionDone, "", false},
		{"bad cursor negative", -1, StatusAvailable, ActionBlocked, ResolutionBadCursor, "", false},
		{"bad cursor over", 4, StatusAvailable, ActionBlocked, ResolutionBadCursor, "", false},
		{"ready mid-flight", 2, StatusReviewTaken, ActionReady, ResolutionReady, "S1_T9.9_end_review", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := &Journal{SchemaVersion: 1, Cursor: tc.cursor, Events: []JournalEvent{}}
			snap := snapshotWithTaskStatus(taskID, tc.live)
			d := ResolveNext(sched, j, snap)
			if d.Action != tc.wantAct {
				t.Fatalf("Action = %q, want %q", d.Action, tc.wantAct)
			}
			if d.Code != tc.wantCode {
				t.Fatalf("Code = %q, want %q", d.Code, tc.wantCode)
			}
			if d.Cursor != tc.cursor {
				t.Fatalf("Cursor = %d, want %d", d.Cursor, tc.cursor)
			}
			if tc.wantRow {
				if d.Row.StepID != tc.wantStep {
					t.Fatalf("Row.StepID = %q, want %q", d.Row.StepID, tc.wantStep)
				}
			} else if d.Row.StepID != "" {
				t.Fatalf("expected empty row, got step %q", d.Row.StepID)
			}
		})
	}
}

func TestResolveNext_UnknownPendingBlocksCursor(t *testing.T) {
	taskID := "T9.9"
	sched := &Schedule{SchemaVersion: 2, Execution: []ScheduleRow{
		{Seq: 1, StepID: "S1_T9.9_implement", TaskID: taskID, Action: "implement", Requires: StatusWrapper{TaskStatus: string(StatusAvailable)}, Produces: StatusWrapper{TaskStatus: string(StatusTaken)}},
		{Seq: 2, StepID: "S1_T9.9_review", TaskID: taskID, Action: "review", Requires: StatusWrapper{TaskStatus: string(StatusTaken)}, Produces: StatusWrapper{TaskStatus: string(StatusReviewTaken)}},
	}}
	j := &Journal{
		SchemaVersion: 1,
		Cursor:        0,
		Events: []JournalEvent{
			{EventID: "E0009", StepID: "S1_T9.9_review", Seq: 2, Outcome: "unknown"},
		},
	}
	snap := snapshotWithTaskStatus(taskID, StatusAvailable)

	d := ResolveNext(sched, j, snap)
	if d.Action != ActionBlocked {
		t.Fatalf("Action = %q, want %q", d.Action, ActionBlocked)
	}
	if d.Code != ResolutionUnknownPending {
		t.Fatalf("Code = %q, want %q", d.Code, ResolutionUnknownPending)
	}
	if d.Cursor != 0 {
		t.Fatalf("Cursor = %d, want 0", d.Cursor)
	}
	if d.Reason == "" {
		t.Fatalf("Reason should be non-empty for unknown_pending")
	}
}

func TestResolveNext_LiveSchedule_ReadyForFreshJournal(t *testing.T) {
	sched := loadExpectedScheduleFixtureSchedule(t)
	j := &Journal{SchemaVersion: 1, Cursor: 0, Events: []JournalEvent{}}
	snap := snapshotWithTaskStatus(sched.Execution[0].TaskID, StatusAvailable)

	d := ResolveNext(&sched, j, snap)
	if d.Action != ActionReady {
		t.Fatalf("Action = %q, want %q", d.Action, ActionReady)
	}
	if d.Code != ResolutionReady {
		t.Fatalf("Code = %q, want %q", d.Code, ResolutionReady)
	}
	if d.Row.StepID != sched.Execution[0].StepID {
		t.Fatalf("row step = %q, want %q", d.Row.StepID, sched.Execution[0].StepID)
	}
}

func TestResolveNext_LiveSchedule_DoneWhenCursorPastEnd(t *testing.T) {
	sched := loadExpectedScheduleFixtureSchedule(t)
	j := &Journal{SchemaVersion: 1, Cursor: len(sched.Execution), Events: []JournalEvent{}}
	snap := snapshotWithTaskStatus("T0.0", StatusAvailable)

	d := ResolveNext(&sched, j, snap)
	if d.Action != ActionDone {
		t.Fatalf("Action = %q, want %q", d.Action, ActionDone)
	}
	if d.Code != ResolutionDone {
		t.Fatalf("Code = %q, want %q", d.Code, ResolutionDone)
	}
}

func loadExpectedScheduleFixtureSchedule(t *testing.T) Schedule {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, expectedScheduleFixture))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := ValidateSchedule(body); err != nil {
		t.Fatalf("fixture schedule invalid: %v", err)
	}
	var sched Schedule
	if err := json.Unmarshal(body, &sched); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if len(sched.Execution) == 0 {
		t.Fatal("fixture schedule has no rows")
	}
	return sched
}
