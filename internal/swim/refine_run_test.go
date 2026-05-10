package swim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// helper: minimal coarse plan with two parents — T1.1 (wave 1) and T2.1 (wave 2).
func writeCoarsePlanForRefineRun(t *testing.T, dir string) string {
	t.Helper()
	plan := map[string]any{
		"schema_version":  1,
		"generated_on":    "2026-05-09",
		"plan_version":    1,
		"plan_generation": "2026-05-09T00:00:00Z",
		"plan":            map[string]any{"id": "refine-run-test"},
		"fp_index":        map[string]any{},
		"doc_index":       map[string]any{},
		"tasks": map[string]any{
			"T1": map[string]any{"title": "task one", "files": []string{"a.go", "b.go"}},
			"T2": map[string]any{"title": "task two", "files": []string{"c.go"}},
		},
		"units": map[string]any{
			"T1.1": map[string]any{"task": "T1", "title": "u1", "kind": "impl", "wave": 1, "plan_line": 1, "depends_on": []string{}},
			"T2.1": map[string]any{"task": "T2", "title": "u2", "kind": "impl", "wave": 2, "plan_line": 2, "depends_on": []string{}},
		},
	}
	body, _ := json.MarshalIndent(plan, "", "  ")
	path := filepath.Join(dir, "coarse.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write coarse: %v", err)
	}
	return path
}

// helper: write a refinement sidecar for the given parent.
func writeRefineSidecar(t *testing.T, dir, coarsePath, parentID string, wave int, fineSteps int) string {
	t.Helper()
	units := []map[string]any{}
	for i := 1; i <= fineSteps; i++ {
		stepID := stepIDFor(wave, parentID, i)
		deps := []string{}
		if i > 1 {
			deps = append(deps, stepIDFor(wave, parentID, i-1))
		}
		units = append(units, map[string]any{
			"parent_unit":    parentID,
			"step_id":        stepID,
			"seq":            i,
			"context_budget": "8k",
			"depends_on":     deps,
			"requires":       map[string]any{"task_status": "taken"},
			"produces":       map[string]any{"task_status": "review_taken"},
			"invoke":         map[string]any{"argv": []string{"echo", parentID, "step", stepIDFor(wave, parentID, i)}},
		})
	}
	side := map[string]any{
		"schema_version": 1,
		"coarse_plan":    coarsePath,
		"profile":        "8k",
		"generated_on":   "2026-05-09",
		"targets":        []string{parentID},
		"units":          units,
	}
	body, _ := json.MarshalIndent(side, "", "  ")
	path := filepath.Join(dir, "refine.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	return path
}

func stepIDFor(wave int, parentID string, n int) string {
	return "F" + intToString(wave) + "_" + parentID + "_s" + intToString(n)
}

func intToString(n int) string {
	return string('0' + byte(n))
}

// helper: write a state file with explicit task-status entries.
func writeStateFor(t *testing.T, path string, taken map[string]bool, completed map[string]bool) {
	t.Helper()
	tk := map[string]any{}
	for id := range taken {
		tk[id] = map[string]any{"taken_by": "phi", "started_at": "2026-05-09 10:00"}
	}
	cp := map[string]any{}
	for id := range completed {
		cp[id] = map[string]any{
			"taken_by":    "phi",
			"started_at":  "2026-05-09 09:00",
			"finished_at": "2026-05-09 09:30",
		}
	}
	state := map[string]any{
		"plan":      "demo",
		"taken":     tk,
		"completed": cp,
	}
	body, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func TestRefineRun_HappyLinearChain(t *testing.T) {
	dir := t.TempDir()
	coarse := writeCoarsePlanForRefineRun(t, dir)
	refine := writeRefineSidecar(t, dir, coarse, "T1.1", 1, 3)
	statePath := filepath.Join(dir, "state.json")

	// T1.1 is taken; refine fine steps run while taken; rollup expects
	// task_status=review_taken so we pre-stage it as review_taken in state for
	// the rollup gate to pass (in real usage the agent transitions state via
	// wp-plan-to-agent.sh; we simulate by pre-mutating).
	writeStateFor(t, statePath, map[string]bool{}, map[string]bool{})
	// Stage T1.1 in state as review_taken via direct write.
	stageTaskStatus(t, statePath, "T1.1", "review_taken")

	coarseJournal := filepath.Join(dir, "coarse.journal.json")

	report, err := RefineRun(RefineRunOptions{
		RefinePath:        refine,
		CoarseJournalPath: coarseJournal,
		StatePath:         statePath,
		InvokeFn:          func(_ []string, _ string) error { return nil },
	})
	if err != nil {
		t.Fatalf("RefineRun: %v", err)
	}
	if report.Stopped != "done" {
		t.Errorf("Stopped = %q, want done", report.Stopped)
	}
	if len(report.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(report.Steps))
	}
	for i, s := range report.Steps {
		if s.Status != "applied" {
			t.Errorf("step %d status = %q, want applied", i, s.Status)
		}
	}
	if len(report.ParentsCompleted) != 1 || report.ParentsCompleted[0] != "T1.1" {
		t.Errorf("ParentsCompleted = %v, want [T1.1]", report.ParentsCompleted)
	}
}

func TestRefineRun_IdempotentRerun(t *testing.T) {
	dir := t.TempDir()
	coarse := writeCoarsePlanForRefineRun(t, dir)
	refine := writeRefineSidecar(t, dir, coarse, "T1.1", 1, 2)
	statePath := filepath.Join(dir, "state.json")
	writeStateFor(t, statePath, map[string]bool{}, map[string]bool{})
	stageTaskStatus(t, statePath, "T1.1", "review_taken")
	coarseJournal := filepath.Join(dir, "coarse.journal.json")

	opts := RefineRunOptions{
		RefinePath:        refine,
		CoarseJournalPath: coarseJournal,
		StatePath:         statePath,
		InvokeFn:          func(_ []string, _ string) error { return nil },
	}
	r1, err := RefineRun(opts)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if len(r1.Steps) != 2 || len(r1.ParentsCompleted) != 1 {
		t.Fatalf("first run: %+v", r1)
	}

	r2, err := RefineRun(opts)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(r2.Steps) != 0 {
		t.Errorf("rerun should be no-op, got %d steps", len(r2.Steps))
	}
	if len(r2.ParentsCompleted) != 0 {
		t.Errorf("rerun must not roll up again, got %v", r2.ParentsCompleted)
	}
	if r2.ProtocolNote != "idempotent_noop" {
		t.Errorf("ProtocolNote = %q, want idempotent_noop", r2.ProtocolNote)
	}
}

func TestRefineRun_PartialFailureBlocksRollup(t *testing.T) {
	dir := t.TempDir()
	coarse := writeCoarsePlanForRefineRun(t, dir)
	refine := writeRefineSidecar(t, dir, coarse, "T1.1", 1, 2)
	statePath := filepath.Join(dir, "state.json")
	writeStateFor(t, statePath, map[string]bool{}, map[string]bool{})
	stageTaskStatus(t, statePath, "T1.1", "review_taken")
	coarseJournal := filepath.Join(dir, "coarse.journal.json")

	calls := 0
	r, err := RefineRun(RefineRunOptions{
		RefinePath:        refine,
		CoarseJournalPath: coarseJournal,
		StatePath:         statePath,
		InvokeFn: func(_ []string, _ string) error {
			calls++
			if calls == 2 {
				return &fakeExitErr{code: 7}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RefineRun: %v", err)
	}
	if len(r.ParentsCompleted) != 0 {
		t.Errorf("partial failure must block rollup, got %v", r.ParentsCompleted)
	}
	if r.Stopped != "non_applied" {
		t.Errorf("Stopped = %q, want non_applied", r.Stopped)
	}
}

func TestRefineRun_RollupPostconditionMismatch(t *testing.T) {
	dir := t.TempDir()
	coarse := writeCoarsePlanForRefineRun(t, dir)
	refine := writeRefineSidecar(t, dir, coarse, "T1.1", 1, 1)
	statePath := filepath.Join(dir, "state.json")
	// State does NOT show T1.1 in review_taken; it's still available.
	writeStateFor(t, statePath, map[string]bool{}, map[string]bool{})
	coarseJournal := filepath.Join(dir, "coarse.journal.json")

	r, err := RefineRun(RefineRunOptions{
		RefinePath:        refine,
		CoarseJournalPath: coarseJournal,
		StatePath:         statePath,
		InvokeFn:          func(_ []string, _ string) error { return nil },
	})
	if err != nil {
		t.Fatalf("RefineRun: %v", err)
	}
	if len(r.ParentsCompleted) != 0 {
		t.Errorf("rollup should be blocked when state mismatch, got %v", r.ParentsCompleted)
	}
	// Last step in report should be a rollup_postcondition_mismatch entry.
	last := r.Steps[len(r.Steps)-1]
	if last.Status != "blocked" {
		t.Errorf("last step status = %q, want blocked", last.Status)
	}
	if last.StepID != parentRollupID("T1.1") {
		t.Errorf("last step_id = %q, want %s", last.StepID, parentRollupID("T1.1"))
	}
}

func TestRefineRun_CrossParentGate(t *testing.T) {
	dir := t.TempDir()
	coarse := writeCoarsePlanForRefineRun(t, dir)
	// Refine T2.1 (wave 2) but T1.1 (wave 1) is NOT complete.
	refine := writeRefineSidecar(t, dir, coarse, "T2.1", 2, 1)
	statePath := filepath.Join(dir, "state.json")
	writeStateFor(t, statePath, map[string]bool{}, map[string]bool{})

	r, err := RefineRun(RefineRunOptions{
		RefinePath: refine,
		StatePath:  statePath,
		InvokeFn:   func(_ []string, _ string) error { return nil },
	})
	if err != nil {
		t.Fatalf("RefineRun: %v", err)
	}
	if r.Stopped != "cross_parent_gate" {
		t.Errorf("Stopped = %q, want cross_parent_gate", r.Stopped)
	}
	if len(r.Steps) != 1 || r.Steps[0].Status != "blocked" {
		t.Errorf("expected one blocked step, got %+v", r.Steps)
	}
}

func TestRefineRun_DryRunNoMutation(t *testing.T) {
	dir := t.TempDir()
	coarse := writeCoarsePlanForRefineRun(t, dir)
	refine := writeRefineSidecar(t, dir, coarse, "T1.1", 1, 2)
	statePath := filepath.Join(dir, "state.json")
	writeStateFor(t, statePath, map[string]bool{}, map[string]bool{})
	stageTaskStatus(t, statePath, "T1.1", "review_taken")

	r, err := RefineRun(RefineRunOptions{
		RefinePath: refine,
		StatePath:  statePath,
		DryRun:     true,
		InvokeFn:   func(_ []string, _ string) error { t.Fatal("InvokeFn must not run in dry-run"); return nil },
	})
	if err != nil {
		t.Fatalf("RefineRun: %v", err)
	}
	if !r.DryRun {
		t.Error("DryRun report flag should be true")
	}
	for _, s := range r.Steps {
		if !s.WouldApply {
			t.Errorf("dry-run step should be would_apply, got %+v", s)
		}
	}
	// No fine journal file should exist.
	fineJournal := filepath.Join(dir, "refine.journal.json")
	if _, err := os.Stat(fineJournal); !os.IsNotExist(err) {
		t.Errorf("dry-run must not write fine journal; %v", err)
	}
}

func TestRefineRun_LockBusyReturnsLockBusyStatus(t *testing.T) {
	dir := t.TempDir()
	coarse := writeCoarsePlanForRefineRun(t, dir)
	refine := writeRefineSidecar(t, dir, coarse, "T1.1", 1, 1)
	statePath := filepath.Join(dir, "state.json")
	writeStateFor(t, statePath, map[string]bool{}, map[string]bool{})
	stageTaskStatus(t, statePath, "T1.1", "review_taken")

	// Hold the lock from a separate path-equivalent goroutine.
	lockPath := deriveRefineLockPath(refine)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	held, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("acquire holder lock: %v", err)
	}
	defer held.Release()

	r, err := RefineRun(RefineRunOptions{
		RefinePath: refine,
		StatePath:  statePath,
		InvokeFn:   func(_ []string, _ string) error { return nil },
	})
	if err != nil {
		t.Fatalf("RefineRun: %v", err)
	}
	if r.Stopped != "lock_busy" {
		t.Errorf("Stopped = %q, want lock_busy", r.Stopped)
	}
	if len(r.Steps) != 1 || r.Steps[0].Status != "lock_busy" {
		t.Errorf("expected one lock_busy step, got %+v", r.Steps)
	}
}

// stageTaskStatus rewrites the state file so the named taskID has the given
// canonical status. Used to simulate side-effects of fine steps that the test
// does not actually invoke.
func stageTaskStatus(t *testing.T, statePath, taskID, status string) {
	t.Helper()
	body, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("parse state: %v", err)
	}
	taken, _ := raw["taken"].(map[string]any)
	if taken == nil {
		taken = map[string]any{}
	}
	completed, _ := raw["completed"].(map[string]any)
	if completed == nil {
		completed = map[string]any{}
	}
	delete(taken, taskID)
	delete(completed, taskID)
	switch status {
	case "available":
		// nothing
	case "taken":
		taken[taskID] = map[string]any{"taken_by": "phi", "started_at": "2026-05-09 10:00"}
	case "review_taken":
		taken[taskID] = map[string]any{
			"taken_by":          "phi",
			"started_at":        "2026-05-09 10:00",
			"review_entered_at": "2026-05-09 10:30",
		}
	case "review_ended":
		taken[taskID] = map[string]any{
			"taken_by":          "phi",
			"started_at":        "2026-05-09 10:00",
			"review_entered_at": "2026-05-09 10:30",
			"review_ended_at":   "2026-05-09 10:45",
		}
	case "completed":
		completed[taskID] = map[string]any{
			"taken_by":    "phi",
			"started_at":  "2026-05-09 09:00",
			"finished_at": "2026-05-09 09:30",
		}
	default:
		t.Fatalf("stageTaskStatus: unsupported status %q", status)
	}
	raw["taken"] = taken
	raw["completed"] = completed
	body, _ = json.MarshalIndent(raw, "", "  ")
	if err := os.WriteFile(statePath, body, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

// fakeExitErr satisfies the runner's exit-code recovery contract via
// errors.As(*exec.ExitError). For our purposes we just use a simple type
// implementing error; exitCodeFromErr falls back to 1 when the assertion
// fails, which is sufficient to mark the run as failed.
type fakeExitErr struct{ code int }

func (f *fakeExitErr) Error() string { return "fake_exit" }
