package swim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApply_AppliedHappyPath(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "apply-happy-schedule.json")
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
	report, err := Apply(ApplyOptions{
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
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "applied" {
		t.Fatalf("status = %q, want applied", report.Status)
	}
	if report.StepID != "S1_T1.1_implement" {
		t.Fatalf("step_id = %q, want S1_T1.1_implement", report.StepID)
	}
	if report.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", report.ExitCode)
	}
	if report.StdoutPath == "" || report.StderrPath == "" {
		t.Fatalf("expected log paths, got stdout=%q stderr=%q", report.StdoutPath, report.StderrPath)
	}
}

func TestApply_BlockedPrecondition(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "blocked-precondition-schedule.json")
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
		{
			Seq:      2,
			StepID:   "S1_T1.1_review",
			TaskID:   "T1.1",
			Action:   "review",
			Requires: StatusWrapper{TaskStatus: string(StatusTaken)},
			Produces: StatusWrapper{TaskStatus: string(StatusReviewTaken)},
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
		},
	})
	writeStateSnapshot(t, statePath, StatusAvailable)
	writeJournal(t, journalPath, Journal{SchemaVersion: 1, SchedulePath: schedulePath, Cursor: 1, Events: []JournalEvent{}})

	report, err := Apply(ApplyOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", report.Status)
	}
	if !strings.Contains(report.Reason, "precondition_unmet") {
		t.Fatalf("reason = %q, want precondition_unmet", report.Reason)
	}
}

func TestApply_BlockedPostcondition(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "blocked-postcondition-schedule.json")
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
	report, err := Apply(ApplyOptions{
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
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", report.Status)
	}
	if !strings.Contains(report.Reason, "postcondition_mismatch") {
		t.Fatalf("reason = %q, want postcondition_mismatch", report.Reason)
	}
}

func TestApply_AdoptsExactProducedStateOnRetry(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "adopt-dispatch-schedule.json")
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

	report, err := Apply(ApplyOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			return snapshotWithTaskStatus("T1.1", StatusTaken), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			t.Fatal("InvokeFn should not be called when adopting state")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "applied" {
		t.Fatalf("status = %q, want applied", report.Status)
	}
	if report.StepID != "S1_T1.1_implement" {
		t.Fatalf("step_id = %q, want S1_T1.1_implement", report.StepID)
	}

	j := readJournal(t, journalPath)
	if j.Cursor != 1 {
		t.Fatalf("journal cursor = %d, want 1", j.Cursor)
	}
	if len(j.Events) != 1 {
		t.Fatalf("journal events len = %d, want 1", len(j.Events))
	}
	if !strings.Contains(j.Events[0].Reason, "idempotent_adopt") {
		t.Fatalf("reason = %q, want idempotent_adopt note", j.Events[0].Reason)
	}
}

func TestApply_IncompleteDispatchWhenStateAdvancedButReceiptMissing(t *testing.T) {
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
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "exit 1"}},
		},
	})

	var reads int
	report, err := Apply(ApplyOptions{
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
			return exitError(1)
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "incomplete_dispatch" {
		t.Fatalf("status = %q, reason = %q, want incomplete_dispatch", report.Status, report.Reason)
	}
	if !strings.Contains(report.Reason, "receipt_missing") {
		t.Fatalf("reason = %q, want receipt_missing", report.Reason)
	}

	j := readJournal(t, journalPath)
	if j.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", j.Cursor)
	}
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	if got := j.Events[0].Outcome; got != "blocked" {
		t.Fatalf("event outcome = %q, want blocked", got)
	}
}

func TestApply_RedispatchesWhenStateAheadAndReceiptMissing(t *testing.T) {
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

	invoked := 0
	report, err := Apply(ApplyOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
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
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "applied" {
		t.Fatalf("status = %q, reason = %q, want applied", report.Status, report.Reason)
	}
	if invoked != 1 {
		t.Fatalf("invoke count = %d, want 1", invoked)
	}

	j := readJournal(t, journalPath)
	if j.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", j.Cursor)
	}
}

func TestApply_AdoptsDispatchStepWhenReceiptPresent(t *testing.T) {
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
	receiptPath := dispatchReceiptAbsPath(schedulePath, "S1_T1.1_implement", 1)
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll receipt dir: %v", err)
	}
	if err := os.WriteFile(receiptPath, []byte(`{"ok":true}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile receipt: %v", err)
	}

	report, err := Apply(ApplyOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			return snapshotWithTaskStatus("T1.1", StatusTaken), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			t.Fatal("InvokeFn should not be called when receipt is present")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "applied" {
		t.Fatalf("status = %q, want applied", report.Status)
	}
}

func TestApply_LockBusyWithDiagnostic(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")
	lockPath := filepath.Join(dir, "swim.lock")

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
	if err := WriteLockHolder(lock.f, 99999, "2026-05-09T00:00:00Z"); err != nil {
		t.Fatalf("WriteLockHolder: %v", err)
	}

	report, err := Apply(ApplyOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		LockPath:     lockPath,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "lock_busy" {
		t.Fatalf("status = %q, want lock_busy", report.Status)
	}
	if !strings.Contains(report.Hint, "pid=99999") {
		t.Fatalf("hint = %q, want pid=99999", report.Hint)
	}
}

func TestApply_UnknownPendingSurfaces(t *testing.T) {
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

	report, err := Apply(ApplyOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "unknown_pending" {
		t.Fatalf("status = %q, want unknown_pending", report.Status)
	}
	if report.StepID != "S1_T1.1_implement" {
		t.Fatalf("step_id = %q, want S1_T1.1_implement", report.StepID)
	}
	if !strings.Contains(report.Hint, "ack-unknown") {
		t.Fatalf("hint = %q, want ack-unknown guidance", report.Hint)
	}
}

func TestApply_DoneAtEnd(t *testing.T) {
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
	writeJournal(t, journalPath, Journal{SchemaVersion: 1, SchedulePath: schedulePath, Cursor: 1, Events: []JournalEvent{}})

	report, err := Apply(ApplyOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "done" {
		t.Fatalf("status = %q, want done", report.Status)
	}
	if report.Reason != "" {
		t.Fatalf("reason = %q, want empty", report.Reason)
	}
}

func TestApply_InvokeNonzeroExit(t *testing.T) {
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
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "exit 2"}},
		},
	})

	report, err := Apply(ApplyOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			return exitError(2)
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", report.Status)
	}
	if report.ExitCode == 0 {
		t.Fatalf("exit_code = %d, want non-zero", report.ExitCode)
	}
	if !strings.Contains(report.Reason, "exit") {
		t.Fatalf("reason = %q, want exit detail", report.Reason)
	}
}

func TestApply_FixAction_HappyPath(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "fix-happy-schedule.json")
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

	var reads int
	report, err := Apply(ApplyOptions{
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
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "applied" {
		t.Fatalf("status = %q, want applied", report.Status)
	}
	if report.StepID != "S1_T1.1_fix" {
		t.Fatalf("step_id = %q, want S1_T1.1_fix", report.StepID)
	}
}

func TestApply_FixAction_IncompleteDispatch(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "fix-incomplete-schedule.json")
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
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "exit 1"}},
		},
	})

	var reads int
	report, err := Apply(ApplyOptions{
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
			return exitError(1)
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "incomplete_dispatch" {
		t.Fatalf("status = %q, reason = %q, want incomplete_dispatch", report.Status, report.Reason)
	}
	if !strings.Contains(report.Reason, "receipt_missing") {
		t.Fatalf("reason = %q, want receipt_missing", report.Reason)
	}
	j := readJournal(t, journalPath)
	if j.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", j.Cursor)
	}
}

func TestApply_JSONRoundTrip(t *testing.T) {
	report := &ApplyReport{
		Status:     "done",
		StepID:     "S1_T1.1_implement",
		Seq:        1,
		ExitCode:   2,
		StdoutPath: "a",
		StderrPath: "b",
		Reason:     "r",
		Hint:       "h",
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	wire := string(body)
	for _, key := range []string{"status", "step_id", "stdout_path", "stderr_path", "exit_code", "reason", "hint"} {
		if !strings.Contains(wire, `"`+key+`"`) {
			t.Fatalf("missing json key %q in %s", key, wire)
		}
	}
}
