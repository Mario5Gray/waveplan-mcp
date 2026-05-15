package swim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadJournalView_AllowsEquivalentSchedulePathForms(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_implement",
			TaskID:   "T1.1",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"/bin/true"}},
		},
	})

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}

	relativeSchedulePath, err := filepath.Rel(wd, schedulePath)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}

	writeJournal(t, journalPath, Journal{
		SchemaVersion: 1,
		SchedulePath:  relativeSchedulePath,
		Cursor:        0,
		Events:        []JournalEvent{},
	})

	view, err := ReadJournalView(journalPath, schedulePath, "", 0)
	if err != nil {
		t.Fatalf("ReadJournalView: %v", err)
	}
	if len(view.MergedExecution) != 1 {
		t.Fatalf("merged_execution len = %d, want 1", len(view.MergedExecution))
	}
	if got := view.MergedExecution[0].StepID; got != "S1_T1.1_implement" {
		t.Fatalf("merged_execution[0].step_id = %q, want S1_T1.1_implement", got)
	}
}

func TestReadJournalView_RejectsDifferentSchedulePaths(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule-a.json")
	otherSchedulePath := filepath.Join(dir, "schedule-b.json")
	journalPath := filepath.Join(dir, "journal.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_implement",
			TaskID:   "T1.1",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"/bin/true"}},
		},
	})
	writeSchedule(t, otherSchedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S9_T9.9_implement",
			TaskID:   "T9.9",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"/bin/true"}},
		},
	})

	writeJournal(t, journalPath, Journal{
		SchemaVersion: 1,
		SchedulePath:  otherSchedulePath,
		Cursor:        0,
		Events:        []JournalEvent{},
	})

	_, err := ReadJournalView(journalPath, schedulePath, "", 0)
	if err == nil {
		t.Fatal("ReadJournalView() error = nil, want schedule_path mismatch")
	}
	if !strings.Contains(err.Error(), "journal schedule_path mismatch") {
		t.Fatalf("ReadJournalView() error = %q, want schedule_path mismatch", err)
	}
}
