package swim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSchedule_MergesReviewSidecarIntoRuntimeOrder(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	reviewSchedulePath := filepath.Join(dir, "review-sidecar.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")
	writeReviewSidecar(t, reviewSchedulePath, schedulePath, []ReviewScheduleInsertion{
		{
			ID:            "X1",
			AfterStepID:   "S1_T1.1_review",
			StepID:        "S1_T1.1_fix_r2",
			SeqHint:       2,
			TaskID:        "T1.1",
			Action:        "fix",
			Requires:      StatusWrapper{TaskStatus: "review_taken"},
			Produces:      StatusWrapper{TaskStatus: "taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "second review loop",
			SourceEventID: "E0002",
		},
		{
			ID:            "X2",
			AfterStepID:   "S1_T1.1_review",
			StepID:        "S1_T1.1_fix_r1",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "fix",
			Requires:      StatusWrapper{TaskStatus: "review_taken"},
			Produces:      StatusWrapper{TaskStatus: "taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "first review loop",
			SourceEventID: "E0001",
		},
		{
			ID:            "X3",
			AfterStepID:   "X2",
			StepID:        "S1_T1.1_review_r2",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "review",
			Requires:      StatusWrapper{TaskStatus: "taken"},
			Produces:      StatusWrapper{TaskStatus: "review_taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "review after fix round 1",
			SourceEventID: "E0003",
		},
		{
			ID:            "X4",
			AfterStepID:   "X1",
			StepID:        "S1_T1.1_review_r3",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "review",
			Requires:      StatusWrapper{TaskStatus: "taken"},
			Produces:      StatusWrapper{TaskStatus: "review_taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "review after fix round 2",
			SourceEventID: "E0004",
		},
	})

	schedule, err := loadSchedule(schedulePath, reviewSchedulePath)
	if err != nil {
		t.Fatalf("loadSchedule with review sidecar: %v", err)
	}

	wantOrder := []string{
		"S1_T1.1_implement",
		"S1_T1.1_review",
		"S1_T1.1_fix_r1",
		"S1_T1.1_review_r2",
		"S1_T1.1_fix_r2",
		"S1_T1.1_review_r3",
		"S1_T1.1_end_review",
		"S1_T1.1_finish",
	}
	if len(schedule.Execution) != len(wantOrder) {
		t.Fatalf("execution len = %d, want %d", len(schedule.Execution), len(wantOrder))
	}
	for i, row := range schedule.Execution {
		if row.StepID != wantOrder[i] {
			t.Fatalf("execution[%d].step_id = %q, want %q", i, row.StepID, wantOrder[i])
		}
		if row.Seq != i+1 {
			t.Fatalf("execution[%d].seq = %d, want %d", i, row.Seq, i+1)
		}
	}
}

func TestRun_DryRun_UsesMergedReviewScheduleRuntimeFrame(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	reviewSchedulePath := filepath.Join(dir, "review-sidecar.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")
	writeReviewSidecar(t, reviewSchedulePath, schedulePath, []ReviewScheduleInsertion{
		{
			ID:            "X1",
			AfterStepID:   "S1_T1.1_review",
			StepID:        "S1_T1.1_fix_r1",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "fix",
			Requires:      StatusWrapper{TaskStatus: "review_taken"},
			Produces:      StatusWrapper{TaskStatus: "taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "review findings require rework",
			SourceEventID: "E0001",
		},
		{
			ID:            "X2",
			AfterStepID:   "X1",
			StepID:        "S1_T1.1_review_r2",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "review",
			Requires:      StatusWrapper{TaskStatus: "taken"},
			Produces:      StatusWrapper{TaskStatus: "review_taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "follow-up review",
			SourceEventID: "E0002",
		},
	})
	writeStateSnapshot(t, statePath, StatusAvailable)

	report, err := Run(RunOptions{
		SchedulePath:       schedulePath,
		ReviewSchedulePath: reviewSchedulePath,
		JournalPath:        journalPath,
		StatePath:          statePath,
		DryRun:             true,
		Until:              "step:S1_T1.1_fix_r1",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.ReachedUntil || report.Stopped != "until" {
		t.Fatalf("stopped=%q reached_until=%v, want until/true", report.Stopped, report.ReachedUntil)
	}
	if len(report.Steps) != 3 {
		t.Fatalf("steps len = %d, want 3", len(report.Steps))
	}
	if got := report.Steps[2].StepID; got != "S1_T1.1_fix_r1" {
		t.Fatalf("step[2].step_id = %q, want S1_T1.1_fix_r1", got)
	}
}

func TestRun_WetRun_UsesMergedReviewScheduleRuntimeFrame(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	reviewSchedulePath := filepath.Join(dir, "review-sidecar.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")
	writeReviewSidecar(t, reviewSchedulePath, schedulePath, []ReviewScheduleInsertion{
		{
			ID:            "X1",
			AfterStepID:   "S1_T1.1_review",
			StepID:        "S1_T1.1_fix_r1",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "fix",
			Requires:      StatusWrapper{TaskStatus: "review_taken"},
			Produces:      StatusWrapper{TaskStatus: "taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "review findings require rework",
			SourceEventID: "E0001",
		},
		{
			ID:            "X2",
			AfterStepID:   "X1",
			StepID:        "S1_T1.1_review_r2",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "review",
			Requires:      StatusWrapper{TaskStatus: "taken"},
			Produces:      StatusWrapper{TaskStatus: "review_taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "follow-up review",
			SourceEventID: "E0002",
		},
	})
	writeStateSnapshot(t, statePath, StatusAvailable)

	report, err := Run(RunOptions{
		SchedulePath:       schedulePath,
		ReviewSchedulePath: reviewSchedulePath,
		JournalPath:        journalPath,
		StatePath:          statePath,
		InvokeFn:           invokeFnWithReceipt(t),
		Until:              "step:S1_T1.1_review_r2",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.ReachedUntil || report.Stopped != "until" {
		t.Fatalf("stopped=%q reached_until=%v, want until/true", report.Stopped, report.ReachedUntil)
	}
	if len(report.Steps) != 4 {
		t.Fatalf("steps len = %d, want 4", len(report.Steps))
	}
	wantOrder := []string{
		"S1_T1.1_implement",
		"S1_T1.1_review",
		"S1_T1.1_fix_r1",
		"S1_T1.1_review_r2",
	}
	for i, want := range wantOrder {
		if report.Steps[i].Status != "applied" {
			t.Fatalf("step[%d].status = %q, want applied", i, report.Steps[i].Status)
		}
		if report.Steps[i].StepID != want {
			t.Fatalf("step[%d].step_id = %q, want %q", i, report.Steps[i].StepID, want)
		}
	}
}

func TestRun_DryRun_BlocksEndReviewWhilePendingSidecarRowsExist(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	reviewSchedulePath := filepath.Join(dir, "review-sidecar.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")
	writeReviewSidecar(t, reviewSchedulePath, schedulePath, []ReviewScheduleInsertion{
		{
			ID:            "X1",
			AfterStepID:   "S1_T1.1_end_review",
			StepID:        "S1_T1.1_fix_r1",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "fix",
			Requires:      StatusWrapper{TaskStatus: "review_taken"},
			Produces:      StatusWrapper{TaskStatus: "taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "pending review-sidecar fix",
			SourceEventID: "E0003",
		},
	})
	writeStateSnapshot(t, statePath, StatusAvailable)

	report, err := Run(RunOptions{
		SchedulePath:       schedulePath,
		ReviewSchedulePath: reviewSchedulePath,
		JournalPath:        journalPath,
		StatePath:          statePath,
		DryRun:             true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Stopped != "blocked" || report.Boundary != "blocked" {
		t.Fatalf("stopped=%q boundary=%q, want blocked/blocked", report.Stopped, report.Boundary)
	}
	if len(report.Steps) != 3 {
		t.Fatalf("steps len = %d, want 3", len(report.Steps))
	}
	last := report.Steps[2]
	if last.Status != "would_block" {
		t.Fatalf("last status = %q, want would_block", last.Status)
	}
	if last.StepID != "S1_T1.1_end_review" {
		t.Fatalf("last step_id = %q, want S1_T1.1_end_review", last.StepID)
	}
	if last.Reason == "" {
		t.Fatal("expected non-empty blocked reason")
	}
}

func TestRun_DryRun_BlocksFinishWhilePendingSidecarRowsExist(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	reviewSchedulePath := filepath.Join(dir, "review-sidecar.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")
	writeReviewSidecar(t, reviewSchedulePath, schedulePath, []ReviewScheduleInsertion{
		{
			ID:            "X1",
			AfterStepID:   "S1_T1.1_finish",
			StepID:        "S1_T1.1_fix_r1",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "fix",
			Requires:      StatusWrapper{TaskStatus: "review_taken"},
			Produces:      StatusWrapper{TaskStatus: "taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "pending review-sidecar fix",
			SourceEventID: "E0004",
		},
	})
	writeStateSnapshot(t, statePath, StatusReviewEnded)
	writeJournal(t, journalPath, Journal{
		SchemaVersion: 1,
		SchedulePath:  schedulePath,
		Cursor:        3,
		Events:        []JournalEvent{},
	})

	report, err := Run(RunOptions{
		SchedulePath:       schedulePath,
		ReviewSchedulePath: reviewSchedulePath,
		JournalPath:        journalPath,
		StatePath:          statePath,
		DryRun:             true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Stopped != "blocked" || report.Boundary != "blocked" {
		t.Fatalf("stopped=%q boundary=%q, want blocked/blocked", report.Stopped, report.Boundary)
	}
	if len(report.Steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(report.Steps))
	}
	last := report.Steps[0]
	if last.Status != "would_block" {
		t.Fatalf("last status = %q, want would_block", last.Status)
	}
	if last.StepID != "S1_T1.1_finish" {
		t.Fatalf("last step_id = %q, want S1_T1.1_finish", last.StepID)
	}
	if last.Reason == "" {
		t.Fatal("expected non-empty blocked reason")
	}
}

func TestRun_WetRun_CursorAdvancesThroughSidecarRows(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	reviewSchedulePath := filepath.Join(dir, "review-sidecar.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeFourStepSchedule(t, schedulePath, "T1.1")
	writeReviewSidecar(t, reviewSchedulePath, schedulePath, []ReviewScheduleInsertion{
		{
			ID:            "X1",
			AfterStepID:   "S1_T1.1_review",
			StepID:        "S1_T1.1_fix_r1",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "fix",
			Requires:      StatusWrapper{TaskStatus: "review_taken"},
			Produces:      StatusWrapper{TaskStatus: "taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "review findings require rework",
			SourceEventID: "E0001",
		},
		{
			ID:            "X2",
			AfterStepID:   "X1",
			StepID:        "S1_T1.1_review_r2",
			SeqHint:       1,
			TaskID:        "T1.1",
			Action:        "review",
			Requires:      StatusWrapper{TaskStatus: "taken"},
			Produces:      StatusWrapper{TaskStatus: "review_taken"},
			Invoke:        InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
			Reason:        "follow-up review",
			SourceEventID: "E0002",
		},
	})
	writeStateSnapshot(t, statePath, StatusReviewTaken)

	writeJournal(t, journalPath, Journal{
		SchemaVersion: 1,
		SchedulePath:  schedulePath,
		Cursor:        2,
		Events:        []JournalEvent{},
	})

	report, err := Run(RunOptions{
		SchedulePath:       schedulePath,
		ReviewSchedulePath: reviewSchedulePath,
		JournalPath:        journalPath,
		StatePath:          statePath,
		InvokeFn:           invokeFnWithReceipt(t),
		MaxSteps:           1,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(report.Steps))
	}
	if report.Steps[0].StepID != "S1_T1.1_fix_r1" {
		t.Fatalf("step[0].step_id = %q, want S1_T1.1_fix_r1", report.Steps[0].StepID)
	}
	if report.Steps[0].Status != "applied" {
		t.Fatalf("step[0].status = %q, want applied", report.Steps[0].Status)
	}

	journalAfterFirstStep := readJournal(t, journalPath)
	if journalAfterFirstStep.Cursor != 3 {
		t.Fatalf("cursor after step 1 = %d, want 3 (pointing to merged index of X2)", journalAfterFirstStep.Cursor)
	}

	report2, err := Run(RunOptions{
		SchedulePath:       schedulePath,
		ReviewSchedulePath: reviewSchedulePath,
		JournalPath:        journalPath,
		StatePath:          statePath,
		InvokeFn:           invokeFnWithReceipt(t),
		MaxSteps:           1,
	})
	if err != nil {
		t.Fatalf("Run step 2: %v", err)
	}
	if len(report2.Steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(report2.Steps))
	}
	if report2.Steps[0].StepID != "S1_T1.1_review_r2" {
		t.Fatalf("step[0].step_id = %q, want S1_T1.1_review_r2", report2.Steps[0].StepID)
	}
	if report2.Steps[0].Status != "applied" {
		t.Fatalf("step[0].status = %q, want applied", report2.Steps[0].Status)
	}

	journalAfterSecondStep := readJournal(t, journalPath)
	if journalAfterSecondStep.Cursor != 4 {
		t.Fatalf("cursor after step 2 = %d, want 4 (pointing to merged index of end_review)", journalAfterSecondStep.Cursor)
	}
}

func writeReviewSidecar(t *testing.T, path, baseSchedulePath string, insertions []ReviewScheduleInsertion) {
	t.Helper()
	body, err := json.MarshalIndent(ReviewScheduleSidecar{
		SchemaVersion:    1,
		BaseSchedulePath: baseSchedulePath,
		Insertions:       insertions,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal review sidecar: %v", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write review sidecar: %v", err)
	}
}
