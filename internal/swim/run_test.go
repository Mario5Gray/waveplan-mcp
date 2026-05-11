package swim

import (
	"os"
	"path/filepath"
	"testing"
)

// invokeFnWithReceipt returns an InvokeFn that writes a dispatch receipt when
// SWIM_DISPATCH_RECEIPT_PATH is set (dispatch steps) and succeeds silently for
// non-dispatch steps. Required after the receipt model was introduced.
func invokeFnWithReceipt(t *testing.T) func([]string, string) error {
	t.Helper()
	return func(_ []string, _ string) error {
		receiptPath := os.Getenv("SWIM_DISPATCH_RECEIPT_PATH")
		if receiptPath == "" {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
			t.Fatalf("MkdirAll receipt dir: %v", err)
		}
		if err := os.WriteFile(receiptPath, []byte(`{"ok":true}`+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile receipt: %v", err)
		}
		return nil
	}
}

func TestRun_DefaultStopsOnFirstNonApplied(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")

	var reads int
	report, err := Run(RunOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			switch reads {
			case 1, 2:
				return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
			case 3:
				return snapshotWithTaskStatus("T1.1", StatusTaken), nil
			default:
				return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
			}
		},
		InvokeFn: invokeFnWithReceipt(t),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(report.Steps))
	}
	if report.Steps[0].Status != "applied" {
		t.Fatalf("step0 status = %q, want applied", report.Steps[0].Status)
	}
	if report.Steps[1].Status != "blocked" {
		t.Fatalf("step1 status = %q, want blocked", report.Steps[1].Status)
	}
	if report.Stopped != "blocked" {
		t.Fatalf("stopped = %q, want blocked", report.Stopped)
	}
}

func TestRun_UntilActionFinish(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")

	var reads int
	report, err := Run(RunOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		Until:        "finish",
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			switch {
			case reads <= 2:
				return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
			case reads <= 5:
				return snapshotWithTaskStatus("T1.1", StatusTaken), nil
			case reads <= 8:
				return snapshotWithTaskStatus("T1.1", StatusReviewTaken), nil
			case reads <= 11:
				return snapshotWithTaskStatus("T1.1", StatusReviewEnded), nil
			default:
				return snapshotWithTaskStatus("T1.1", StatusCompleted), nil
			}
		},
		InvokeFn: invokeFnWithReceipt(t),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Steps) != 4 {
		t.Fatalf("steps len = %d, want 4", len(report.Steps))
	}
	if report.Stopped != "until" || !report.ReachedUntil {
		t.Fatalf("stopped=%q reached_until=%v, want until/true", report.Stopped, report.ReachedUntil)
	}
	if report.Steps[3].StepID != "S1_T1.1_finish" {
		t.Fatalf("last step_id = %q, want S1_T1.1_finish", report.Steps[3].StepID)
	}
}

func TestRun_UntilSeq(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{Seq: 1, StepID: "S1_T1.1_implement", TaskID: "T1.1", Action: "implement", Requires: StatusWrapper{TaskStatus: "available"}, Produces: StatusWrapper{TaskStatus: "taken"}, Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 2, StepID: "S1_T1.1_review", TaskID: "T1.1", Action: "review", Requires: StatusWrapper{TaskStatus: "taken"}, Produces: StatusWrapper{TaskStatus: "review_taken"}, Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 3, StepID: "S1_T1.1_end_review", TaskID: "T1.1", Action: "end_review", Requires: StatusWrapper{TaskStatus: "review_taken"}, Produces: StatusWrapper{TaskStatus: "review_ended"}, Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 4, StepID: "S1_T1.1_finish", TaskID: "T1.1", Action: "finish", Requires: StatusWrapper{TaskStatus: "review_ended"}, Produces: StatusWrapper{TaskStatus: "completed"}, Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 5, StepID: "S1_T1.2_implement", TaskID: "T1.2", Action: "implement", Requires: StatusWrapper{TaskStatus: "available"}, Produces: StatusWrapper{TaskStatus: "taken"}, Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 6, StepID: "S1_T1.2_review", TaskID: "T1.2", Action: "review", Requires: StatusWrapper{TaskStatus: "taken"}, Produces: StatusWrapper{TaskStatus: "review_taken"}, Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
	})

	statuses := []struct {
		task   string
		status Status
	}{
		{"T1.1", StatusAvailable}, {"T1.1", StatusAvailable}, {"T1.1", StatusTaken},
		{"T1.1", StatusTaken}, {"T1.1", StatusTaken}, {"T1.1", StatusReviewTaken},
		{"T1.1", StatusReviewTaken}, {"T1.1", StatusReviewTaken}, {"T1.1", StatusReviewEnded},
		{"T1.1", StatusReviewEnded}, {"T1.1", StatusReviewEnded}, {"T1.1", StatusCompleted},
		{"T1.2", StatusAvailable}, {"T1.2", StatusAvailable}, {"T1.2", StatusTaken},
	}
	var reads int
	report, err := Run(RunOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		Until:        "seq:5",
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			if reads >= len(statuses) {
				return snapshotWithTaskStatus("T1.2", StatusTaken), nil
			}
			cur := statuses[reads]
			reads++
			return snapshotWithTaskStatus(cur.task, cur.status), nil
		},
		InvokeFn: invokeFnWithReceipt(t),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Steps) != 5 {
		t.Fatalf("steps len = %d, want 5", len(report.Steps))
	}
	if report.Stopped != "until" || !report.ReachedUntil {
		t.Fatalf("stopped=%q reached=%v, want until/true", report.Stopped, report.ReachedUntil)
	}
	if report.Steps[4].Seq != 5 {
		t.Fatalf("last seq = %d, want 5", report.Steps[4].Seq)
	}
}

func TestRun_UntilStep(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")

	var reads int
	report, err := Run(RunOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		Until:        "step:S1_T1.1_finish",
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			switch {
			case reads <= 2:
				return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
			case reads <= 5:
				return snapshotWithTaskStatus("T1.1", StatusTaken), nil
			case reads <= 8:
				return snapshotWithTaskStatus("T1.1", StatusReviewTaken), nil
			case reads <= 11:
				return snapshotWithTaskStatus("T1.1", StatusReviewEnded), nil
			default:
				return snapshotWithTaskStatus("T1.1", StatusCompleted), nil
			}
		},
		InvokeFn: invokeFnWithReceipt(t),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Steps) != 4 {
		t.Fatalf("steps len = %d, want 4", len(report.Steps))
	}
	if report.Steps[3].StepID != "S1_T1.1_finish" {
		t.Fatalf("last step = %q, want finish", report.Steps[3].StepID)
	}
}

func TestRun_StopOnLockBusy(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")
	lockPath := filepath.Join(dir, "custom.lock")

	writeFourStepSchedule(t, schedulePath, "T1.1")
	writeStateSnapshot(t, statePath, StatusAvailable)
	lock, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	report, err := Run(RunOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		LockPath:     lockPath,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(report.Steps))
	}
	if report.Stopped != "lock_busy" {
		t.Fatalf("stopped = %q, want lock_busy", report.Stopped)
	}
}

func TestRun_DryRunEmitsSequence(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")
	writeStateSnapshot(t, statePath, StatusAvailable)

	var reads int
	report, err := Run(RunOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		Until:        "finish",
		DryRun:       true,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			switch reads {
			case 1:
				return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
			case 2:
				return snapshotWithTaskStatus("T1.1", StatusTaken), nil
			case 3:
				return snapshotWithTaskStatus("T1.1", StatusReviewTaken), nil
			default:
				return snapshotWithTaskStatus("T1.1", StatusReviewEnded), nil
			}
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.DryRun {
		t.Fatal("expected dry_run=true")
	}
	if len(report.Steps) != 4 {
		t.Fatalf("steps len = %d, want 4", len(report.Steps))
	}
	if _, err := os.Stat(journalPath); err == nil {
		t.Fatal("dry-run should not write journal")
	}
}

func TestRun_DryRunHonorsBlocked(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")

	report, err := Run(RunOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		DryRun:       true,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			return snapshotWithTaskStatus("T1.1", StatusTaken), nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(report.Steps))
	}
	if report.Steps[0].Status != "would_block" {
		t.Fatalf("status = %q, want would_block", report.Steps[0].Status)
	}
}

func TestRun_MaxStepsCap(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")

	var reads int
	report, err := Run(RunOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		MaxSteps:     2,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			switch reads {
			case 1, 2:
				return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
			case 3:
				return snapshotWithTaskStatus("T1.1", StatusTaken), nil
			case 4, 5:
				return snapshotWithTaskStatus("T1.1", StatusTaken), nil
			case 6:
				return snapshotWithTaskStatus("T1.1", StatusReviewTaken), nil
			default:
				return snapshotWithTaskStatus("T1.1", StatusReviewTaken), nil
			}
		},
		InvokeFn: invokeFnWithReceipt(t),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(report.Steps))
	}
	if report.Stopped != "max_steps" {
		t.Fatalf("stopped = %q, want max_steps", report.Stopped)
	}
}

func TestRun_InvalidUntil(t *testing.T) {
	_, err := Run(RunOptions{Until: "foo:bar"})
	if err == nil {
		t.Fatal("expected invalid until error")
	}
}

func TestRun_UntilFixWithRoundSuffix(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{Seq: 1, StepID: "S1_T1.1_implement", TaskID: "T1.1", Action: "implement",
			Requires: StatusWrapper{TaskStatus: "available"}, Produces: StatusWrapper{TaskStatus: "taken"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 2, StepID: "S2_T1.1_review", TaskID: "T1.1", Action: "review",
			Requires: StatusWrapper{TaskStatus: "taken"}, Produces: StatusWrapper{TaskStatus: "review_taken"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 3, StepID: "S3_T1.1_fix_r1", TaskID: "T1.1", Action: "fix",
			Requires: StatusWrapper{TaskStatus: "review_taken"}, Produces: StatusWrapper{TaskStatus: "taken"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
	})

	var reads int
	report, err := Run(RunOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		Until:        "fix",
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			switch {
			case reads <= 2:
				return snapshotWithTaskStatus("T1.1", StatusAvailable), nil
			case reads <= 5:
				return snapshotWithTaskStatus("T1.1", StatusTaken), nil
			case reads <= 8:
				return snapshotWithTaskStatus("T1.1", StatusReviewTaken), nil
			default:
				return snapshotWithTaskStatus("T1.1", StatusTaken), nil
			}
		},
		InvokeFn: invokeFnWithReceipt(t),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Stopped != "until" {
		t.Fatalf("stopped = %q, want until", report.Stopped)
	}
	if !report.ReachedUntil {
		t.Fatal("expected ReachedUntil=true")
	}
	if len(report.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(report.Steps))
	}
	last := report.Steps[len(report.Steps)-1]
	if last.StepID != "S3_T1.1_fix_r1" {
		t.Fatalf("last step_id = %q, want S3_T1.1_fix_r1", last.StepID)
	}
}

func writeFourStepSchedule(t *testing.T, path, taskID string) {
	t.Helper()
	writeSchedule(t, path, []ScheduleRow{
		{Seq: 1, StepID: "S1_" + taskID + "_implement", TaskID: taskID, Action: "implement", Requires: StatusWrapper{TaskStatus: "available"}, Produces: StatusWrapper{TaskStatus: "taken"}, Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 2, StepID: "S1_" + taskID + "_review", TaskID: taskID, Action: "review", Requires: StatusWrapper{TaskStatus: "taken"}, Produces: StatusWrapper{TaskStatus: "review_taken"}, Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 3, StepID: "S1_" + taskID + "_end_review", TaskID: taskID, Action: "end_review", Requires: StatusWrapper{TaskStatus: "review_taken"}, Produces: StatusWrapper{TaskStatus: "review_ended"}, Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 4, StepID: "S1_" + taskID + "_finish", TaskID: taskID, Action: "finish", Requires: StatusWrapper{TaskStatus: "review_ended"}, Produces: StatusWrapper{TaskStatus: "completed"}, Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
	})
}
