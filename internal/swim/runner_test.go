package swim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExecuteNextStep_SuccessAdvancesCursor(t *testing.T) {
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
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
		},
	})

	res, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath})
	if err != nil {
		t.Fatalf("ExecuteNextStep: %v", err)
	}
	if res.Outcome != "applied" {
		t.Fatalf("outcome = %q, want applied", res.Outcome)
	}
	if res.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", res.Cursor)
	}

	j := readJournal(t, journalPath)
	if j.Cursor != 1 {
		t.Fatalf("journal cursor = %d, want 1", j.Cursor)
	}
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	if j.Events[0].Outcome != "applied" {
		t.Fatalf("event outcome = %q, want applied", j.Events[0].Outcome)
	}
}

func TestExecuteNextStep_FailureKeepsCursorAndAppendsEvent(t *testing.T) {
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
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "exit 7"}},
		},
	})

	res, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath})
	if err != nil {
		t.Fatalf("ExecuteNextStep: %v", err)
	}
	if res.Outcome != "failed" {
		t.Fatalf("outcome = %q, want failed", res.Outcome)
	}
	if res.ExitCode != 7 {
		t.Fatalf("exit_code = %d, want 7", res.ExitCode)
	}
	if res.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", res.Cursor)
	}

	j := readJournal(t, journalPath)
	if j.Cursor != 0 {
		t.Fatalf("journal cursor = %d, want 0", j.Cursor)
	}
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	if j.Events[0].Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", j.Events[0].Attempt)
	}

	res2, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath})
	if err != nil {
		t.Fatalf("second ExecuteNextStep: %v", err)
	}
	if res2.Outcome != "failed" {
		t.Fatalf("second outcome = %q, want failed", res2.Outcome)
	}
	j = readJournal(t, journalPath)
	if len(j.Events) != 2 {
		t.Fatalf("events len after retry = %d, want 2", len(j.Events))
	}
	if j.Events[1].Attempt != 2 {
		t.Fatalf("attempt after retry = %d, want 2", j.Events[1].Attempt)
	}
}

func TestExecuteNextStep_DoneWhenCursorAtEnd(t *testing.T) {
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
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
		},
	})

	writeJournal(t, journalPath, Journal{SchemaVersion: 1, SchedulePath: schedulePath, Cursor: 1, Events: []JournalEvent{}})

	res, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath})
	if err != nil {
		t.Fatalf("ExecuteNextStep: %v", err)
	}
	if !res.Done {
		t.Fatalf("Done = false, want true")
	}
	if res.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", res.Cursor)
	}
}

func TestExecuteNextStep_ExpectCursorMismatch(t *testing.T) {
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
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
		},
	})
	writeJournal(t, journalPath, Journal{SchemaVersion: 1, SchedulePath: schedulePath, Cursor: 0, Events: []JournalEvent{}})

	expected := 1
	if _, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath, ExpectCursor: &expected}); err == nil {
		t.Fatalf("expected cursor mismatch error")
	}
}

func writeSchedule(t *testing.T, path string, rows []ScheduleRow) {
	t.Helper()
	body, err := json.MarshalIndent(map[string]any{
		"schema_version": 2,
		"execution":      rows,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal schedule: %v", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write schedule: %v", err)
	}
}

func writeJournal(t *testing.T, path string, j Journal) {
	t.Helper()
	body, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		t.Fatalf("marshal journal: %v", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write journal: %v", err)
	}
}

func readJournal(t *testing.T, path string) Journal {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var j Journal
	if err := json.Unmarshal(body, &j); err != nil {
		t.Fatalf("decode journal: %v", err)
	}
	return j
}
