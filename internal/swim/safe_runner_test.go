package swim

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteNextStepSafe_LockBusy(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "adopt-drift-schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")
	lockPath := filepath.Join(dir, "custom.lock")

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
	writeStateSnapshot(t, statePath, StatusAvailable)

	lock, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer func() {
		if err := lock.Release(); err != nil {
			t.Fatalf("Release: %v", err)
		}
	}()

	_, err = ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		LockPath:     lockPath,
	})
	if !errors.Is(err, ErrLockBusy) {
		t.Fatalf("error = %v, want ErrLockBusy", err)
	}
}

func TestExecuteNextStepSafe_RecoversUnknownBeforeResolve(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

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
	writeStateSnapshot(t, statePath, StatusAvailable)
	writeJournal(t, journalPath, Journal{
		SchemaVersion: 1,
		SchedulePath:  schedulePath,
		Cursor:        0,
		Events: []JournalEvent{
			{
				EventID:     "E0001",
				StepID:      "S1_T1.1_implement",
				Seq:         1,
				TaskID:      "T1.1",
				Action:      "implement",
				Attempt:     1,
				StartedOn:   "2026-05-08T00:00:00Z",
				StateBefore: StatusWrapper{TaskStatus: string(StatusAvailable)},
				StateAfter:  StatusWrapper{TaskStatus: string(StatusTaken)},
			},
		},
	})

	invoked := 0
	res, err := ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		InvokeFn: func(_ []string, _ string) error {
			invoked++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteNextStepSafe: %v", err)
	}
	if res.Outcome != "blocked" {
		t.Fatalf("outcome = %q, want blocked", res.Outcome)
	}
	if invoked != 0 {
		t.Fatalf("invoke count = %d, want 0", invoked)
	}

	j := readJournal(t, journalPath)
	if got := j.Events[0].Outcome; got != "unknown" {
		t.Fatalf("recovered outcome = %q, want unknown", got)
	}
	if j.Events[0].CompletedOn == "" {
		t.Fatal("expected completed_on after unknown recovery")
	}
}

func TestExecuteNextStepSafe_BlocksOnSnapshotDriftBeforeInvoke(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

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

	var reads int
	invoked := 0
	res, err := ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			if reads == 1 {
				return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
			}
			return snapshotWithTaskStatus("T1.1", StatusTaken), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			invoked++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteNextStepSafe: %v", err)
	}
	if res.Outcome != "blocked" {
		t.Fatalf("outcome = %q, want blocked", res.Outcome)
	}
	if invoked != 0 {
		t.Fatalf("invoke count = %d, want 0", invoked)
	}
	if res.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", res.Cursor)
	}

	j := readJournal(t, journalPath)
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	if got := j.Events[0].Outcome; got != "blocked" {
		t.Fatalf("event outcome = %q, want blocked", got)
	}
	if j.Events[0].Reason == "" {
		t.Fatal("expected non-empty blocked reason")
	}
}

func TestExecuteNextStepSafe_AdoptsExactProducedStateOnCursorDrift(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "adopt-drift-schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

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
	receiptPath := dispatchReceiptAbsPath(schedulePath, "S1_T1.1_implement", 1)
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll receipt dir: %v", err)
	}
	if err := os.WriteFile(receiptPath, []byte(`{"ok":true}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile receipt: %v", err)
	}

	invoked := 0
	res, err := ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			return snapshotWithTaskStatus("T1.1", StatusTaken), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			invoked++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteNextStepSafe: %v", err)
	}
	if res.Outcome != "applied" {
		t.Fatalf("outcome = %q, want applied", res.Outcome)
	}
	if res.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", res.Cursor)
	}
	if invoked != 0 {
		t.Fatalf("invoke count = %d, want 0", invoked)
	}

	j := readJournal(t, journalPath)
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	if got := j.Events[0].Outcome; got != "applied" {
		t.Fatalf("event outcome = %q, want applied", got)
	}
	if j.Events[0].Reason == "" || !strings.Contains(j.Events[0].Reason, "idempotent_adopt") {
		t.Fatalf("event reason = %q, want idempotent_adopt note", j.Events[0].Reason)
	}
}

func TestExecuteNextStepSafe_PersistsDispatchStateAfterSuccessfulInvoke(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "persist-dispatch-schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

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
	writeStateSnapshot(t, statePath, StatusAvailable)

	invoked := 0
	res, err := ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		InvokeFn: func(_ []string, _ string) error {
			invoked++
			receiptPath := os.Getenv("SWIM_DISPATCH_RECEIPT_PATH")
			if receiptPath == "" {
				t.Fatal("missing SWIM_DISPATCH_RECEIPT_PATH")
			}
			if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
				t.Fatalf("MkdirAll receipt dir: %v", err)
			}
			if err := os.WriteFile(receiptPath, []byte(`{"ok":true}`+"\n"), 0o644); err != nil {
				t.Fatalf("WriteFile receipt: %v", err)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteNextStepSafe: %v", err)
	}
	if res.Outcome != "applied" {
		t.Fatalf("outcome = %q, want applied", res.Outcome)
	}
	if invoked != 1 {
		t.Fatalf("invoke count = %d, want 1", invoked)
	}

	snap, err := ReadStateSnapshot(statePath)
	if err != nil {
		t.Fatalf("ReadStateSnapshot: %v", err)
	}
	if got := snap.StatusOf("T1.1"); got != StatusTaken {
		t.Fatalf("status = %q, want %q", got, StatusTaken)
	}
}

func TestExecuteNextStepSafe_AdoptsPriorDispatchReceiptWhenStateFileIsStale(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "adopt-prior-receipt-schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

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
	writeStateSnapshot(t, statePath, StatusAvailable)

	receiptPath := dispatchReceiptAbsPath(schedulePath, "S1_T1.1_implement", 1)
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll receipt dir: %v", err)
	}
	if err := os.WriteFile(receiptPath, []byte(`{"ok":true}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile receipt: %v", err)
	}

	invoked := 0
	res, err := ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		InvokeFn: func(_ []string, _ string) error {
			invoked++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteNextStepSafe: %v", err)
	}
	if res.Outcome != "applied" {
		t.Fatalf("outcome = %q, want applied", res.Outcome)
	}
	if invoked != 0 {
		t.Fatalf("invoke count = %d, want 0", invoked)
	}

	j := readJournal(t, journalPath)
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	if got := j.Events[0].Reason; got == "" || !strings.Contains(got, "receipt_adopt") {
		t.Fatalf("event reason = %q, want receipt_adopt note", got)
	}

	snap, err := ReadStateSnapshot(statePath)
	if err != nil {
		t.Fatalf("ReadStateSnapshot: %v", err)
	}
	if got := snap.StatusOf("T1.1"); got != StatusTaken {
		t.Fatalf("status = %q, want %q", got, StatusTaken)
	}
}

func TestExecuteNextStepSafe_AppliesOnStableSnapshotsAndProducedState(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "apply-stable-schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

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

	var reads int
	invoked := 0
	res, err := ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			if reads < 3 {
				return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
			}
			return snapshotWithTaskStatus("T1.1", StatusTaken), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			invoked++
			receiptPath := os.Getenv("SWIM_DISPATCH_RECEIPT_PATH")
			if receiptPath == "" {
				t.Fatal("missing SWIM_DISPATCH_RECEIPT_PATH")
			}
			if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
				t.Fatalf("MkdirAll receipt dir: %v", err)
			}
			if err := os.WriteFile(receiptPath, []byte(`{"ok":true}`+"\n"), 0o644); err != nil {
				t.Fatalf("WriteFile receipt: %v", err)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteNextStepSafe: %v", err)
	}
	if res.Outcome != "applied" {
		t.Fatalf("outcome = %q, want applied", res.Outcome)
	}
	if res.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", res.Cursor)
	}
	if invoked != 1 {
		t.Fatalf("invoke count = %d, want 1", invoked)
	}

	j := readJournal(t, journalPath)
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	if got := j.Events[0].Outcome; got != "applied" {
		t.Fatalf("event outcome = %q, want applied", got)
	}
	if j.Events[0].CompletedOn == "" {
		t.Fatal("expected completed_on on applied event")
	}
}

func TestExecuteNextStepSafe_FixInjectsPriorStdoutPath(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "fix-prior-stdout-schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_fix",
			TaskID:   "T1.1",
			Action:   "fix",
			Requires: StatusWrapper{TaskStatus: string(StatusReviewTaken)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
		},
	})

	priorStdout := ".waveplan/swim/fix-prior-stdout-schedule/logs/S1_T1.1_review.1.stdout.log"
	writeJournal(t, journalPath, Journal{
		SchemaVersion: 1,
		SchedulePath:  schedulePath,
		Cursor:        0,
		Events: []JournalEvent{
			{
				EventID:     "E0001",
				StepID:      "S1_T1.1_review",
				Seq:         1,
				TaskID:      "T1.1",
				Action:      "review",
				Attempt:     1,
				StartedOn:   "2026-05-11T00:00:00Z",
				CompletedOn: "2026-05-11T00:00:01Z",
				Outcome:     "applied",
				StateBefore: StatusWrapper{TaskStatus: string(StatusTaken)},
				StateAfter:  StatusWrapper{TaskStatus: string(StatusReviewTaken)},
				StdoutPath:  priorStdout,
			},
		},
	})

	var capturedPriorPath string
	var reads int
	_, err := ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			if reads < 3 {
				return snapshotWithTaskStatus("T1.1", StatusReviewTaken), nil
			}
			return snapshotWithTaskStatus("T1.1", StatusTaken), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			capturedPriorPath = os.Getenv("SWIM_PRIOR_STDOUT_PATH")
			receiptPath := os.Getenv("SWIM_DISPATCH_RECEIPT_PATH")
			if receiptPath != "" {
				if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
					t.Fatalf("MkdirAll receipt dir: %v", err)
				}
				if err := os.WriteFile(receiptPath, []byte(`{"ok":true}`+"\n"), 0o644); err != nil {
					t.Fatalf("WriteFile receipt: %v", err)
				}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteNextStepSafe: %v", err)
	}

	if !strings.HasSuffix(capturedPriorPath, priorStdout) {
		t.Fatalf("SWIM_PRIOR_STDOUT_PATH = %q, want path ending in %q", capturedPriorPath, priorStdout)
	}
	if !filepath.IsAbs(capturedPriorPath) {
		t.Fatalf("SWIM_PRIOR_STDOUT_PATH = %q, want absolute path", capturedPriorPath)
	}
}

func TestExecuteNextStepSafe_BlocksOnPostconditionMismatch(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

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

	var reads int
	res, err := ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteNextStepSafe: %v", err)
	}
	if res.Outcome != "blocked" {
		t.Fatalf("outcome = %q, want blocked", res.Outcome)
	}
	if res.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", res.Cursor)
	}

	j := readJournal(t, journalPath)
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	if got := j.Events[0].Outcome; got != "blocked" {
		t.Fatalf("event outcome = %q, want blocked", got)
	}
	if j.Events[0].Reason == "" {
		t.Fatal("expected non-empty blocked reason")
	}
}

func writeStateSnapshot(t *testing.T, path string, status Status) {
	t.Helper()
	taskID := "T1.1"
	snap := snapshotWithTaskStatus(taskID, status)

	body, err := json.MarshalIndent(snap.raw, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
}
