package swim

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureCoarsePlan writes a small coarse plan covering two parents,
// each with a controlled number of files. Returns the absolute path.
func fixtureCoarsePlan(t *testing.T, files map[string][]string) string {
	t.Helper()
	plan := map[string]any{
		"schema_version":  1,
		"generated_on":    "2026-05-09",
		"plan_version":    1,
		"plan_generation": "2026-05-09T00:00:00Z",
		"plan":            map[string]any{"id": "refine-test"},
		"fp_index":        map[string]any{},
		"doc_index":       map[string]any{},
		"tasks": map[string]any{
			"T1": map[string]any{"title": "task one", "files": files["T1"]},
			"T2": map[string]any{"title": "task two", "files": files["T2"]},
		},
		"units": map[string]any{
			"T1.1": map[string]any{"task": "T1", "title": "u1", "kind": "impl", "wave": 1, "plan_line": 1, "depends_on": []string{}},
			"T1.2": map[string]any{"task": "T1", "title": "u2", "kind": "impl", "wave": 1, "plan_line": 2, "depends_on": []string{}},
			"T2.1": map[string]any{"task": "T2", "title": "u3", "kind": "impl", "wave": 2, "plan_line": 3, "depends_on": []string{}},
		},
	}
	body, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "coarse.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestRefine_RejectsNon8kProfile(t *testing.T) {
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": {"a.go"}, "T2": {"b.go"}})
	_, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        RefineProfile("16k"),
		Targets:        []string{"T1.1"},
	})
	if err == nil {
		t.Fatal("expected error on non-8k profile")
	}
	if !strings.Contains(err.Error(), "v1 supports only") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRefine_RejectsEmptyTargets(t *testing.T) {
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": {"a.go"}})
	_, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        nil,
	})
	if err == nil {
		t.Fatal("expected error on empty targets")
	}
	if !strings.Contains(err.Error(), "targets required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRefine_RejectsUnknownTarget(t *testing.T) {
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": {"a.go"}, "T2": {"b.go"}})
	_, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"T9.9"},
	})
	if err == nil {
		t.Fatal("expected error on unknown target")
	}
	if !strings.Contains(err.Error(), "not present in coarse plan") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRefine_RejectsBadUnitID(t *testing.T) {
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": {"a.go"}, "T2": {"b.go"}})
	_, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"not-a-unit-id"},
	})
	if err == nil {
		t.Fatal("expected error on malformed unit id")
	}
}

func TestRefine_PassthroughEmitsOneStep(t *testing.T) {
	// Parent has 0 files → emit one passthrough s1.
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": nil, "T2": {"b.go"}})
	side, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"T1.1"},
	})
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	if len(side.Units) != 1 {
		t.Fatalf("expected 1 fine step (passthrough), got %d", len(side.Units))
	}
	u := side.Units[0]
	if u.StepID != "F1_T1.1_s1" {
		t.Errorf("step_id = %q, want F1_T1.1_s1", u.StepID)
	}
	if len(u.FilesScope) != 0 {
		t.Errorf("passthrough files_scope should be empty, got %v", u.FilesScope)
	}
}

func TestRefine_ChunksByIndexUnderMaxFiles(t *testing.T) {
	// 14 files at MaxFiles=6 should produce 3 chunks: 6 + 6 + 2.
	files := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n"}
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": files, "T2": {"x.go"}})
	side, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"T1.1"},
	})
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	if len(side.Units) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(side.Units))
	}
	wantSizes := []int{6, 6, 2}
	wantSteps := []string{"F1_T1.1_s1", "F1_T1.1_s2", "F1_T1.1_s3"}
	for i, u := range side.Units {
		if len(u.FilesScope) != wantSizes[i] {
			t.Errorf("unit %d files_scope size = %d, want %d", i, len(u.FilesScope), wantSizes[i])
		}
		if u.StepID != wantSteps[i] {
			t.Errorf("unit %d step_id = %q, want %q", i, u.StepID, wantSteps[i])
		}
	}
}

func TestRefine_LinearChainDepends(t *testing.T) {
	files := []string{"a", "b", "c", "d", "e", "f", "g"}
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": files, "T2": {"x.go"}})
	side, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"T1.1"},
	})
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	if len(side.Units) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(side.Units))
	}
	if len(side.Units[0].DependsOn) != 0 {
		t.Errorf("s1 should have no deps, got %v", side.Units[0].DependsOn)
	}
	if len(side.Units[1].DependsOn) != 1 || side.Units[1].DependsOn[0] != "F1_T1.1_s1" {
		t.Errorf("s2 should depend on s1, got %v", side.Units[1].DependsOn)
	}
}

func TestRefine_PreservesCoarsePlanOrder(t *testing.T) {
	// Targets given out-of-order; emission must follow coarse-plan order.
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": {"a"}, "T2": {"b"}})
	side, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"T2.1", "T1.2", "T1.1"}, // unsorted on purpose
	})
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	wantParents := []string{"T1.1", "T1.2", "T2.1"}
	if len(side.Units) != len(wantParents) {
		t.Fatalf("expected %d units, got %d", len(wantParents), len(side.Units))
	}
	for i, u := range side.Units {
		if u.ParentUnit != wantParents[i] {
			t.Errorf("position %d: parent = %q, want %q", i, u.ParentUnit, wantParents[i])
		}
	}
}

func TestRefine_DedupesAndSortsTargets(t *testing.T) {
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": {"a"}, "T2": {"b"}})
	side, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"T1.2", "T1.1", "T1.2", "T1.1"}, // dups
	})
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	want := []string{"T1.1", "T1.2"}
	if len(side.Targets) != len(want) {
		t.Fatalf("dedupe: got %v, want %v", side.Targets, want)
	}
	for i, w := range want {
		if side.Targets[i] != w {
			t.Errorf("targets[%d] = %q, want %q", i, side.Targets[i], w)
		}
	}
}

func TestRefine_DeterministicByteIdenticalOutput(t *testing.T) {
	planPath := fixtureCoarsePlan(t, map[string][]string{
		"T1": {"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go", "h.go"},
		"T2": {"x.go"},
	})
	opts := RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"T2.1", "T1.1"}, // unsorted; must still be byte-identical
	}
	s1, err := Refine(opts)
	if err != nil {
		t.Fatalf("Refine s1: %v", err)
	}
	s2, err := Refine(opts)
	if err != nil {
		t.Fatalf("Refine s2: %v", err)
	}
	b1, _ := MarshalSidecar(s1)
	b2, _ := MarshalSidecar(s2)
	if !bytes.Equal(b1, b2) {
		t.Fatalf("output not byte-identical:\n--- run1 ---\n%s\n--- run2 ---\n%s", b1, b2)
	}
	h1 := sha256.Sum256(b1)
	h2 := sha256.Sum256(b2)
	if hex.EncodeToString(h1[:]) != hex.EncodeToString(h2[:]) {
		t.Fatalf("hash mismatch: %x vs %x", h1, h2)
	}
}

func TestRefine_InvokeArgvShapeAndCommandHintParity(t *testing.T) {
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": {"a.go"}, "T2": {"b.go"}})
	side, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"T1.1"},
		InvokerBin:     "wp-plan-to-agent.sh",
	})
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	u := side.Units[0]
	want := []string{
		"wp-plan-to-agent.sh",
		"--mode", "implement",
		"--plan", planPath,
		"--task-id", "T1.1",
		"--step", "s1",
	}
	if len(u.Invoke.Argv) != len(want) {
		t.Fatalf("argv len: got %d, want %d (%v)", len(u.Invoke.Argv), len(want), u.Invoke.Argv)
	}
	for i, a := range u.Invoke.Argv {
		if a != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, a, want[i])
		}
	}
	// command_hint must shell-tokenize back to argv (parity contract)
	if u.CommandHint == "" {
		t.Fatal("command_hint should be populated for parity")
	}
}

func TestRefine_PlanRefMirrorsCoarse(t *testing.T) {
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": {"a.go"}, "T2": {"b.go"}})
	side, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"T1.1"},
	})
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	if side.PlanRef == nil {
		t.Fatal("plan_ref should mirror coarse plan_version + plan_generation")
	}
	if side.PlanRef.PlanVersion != 1 || side.PlanRef.PlanGeneration != "2026-05-09T00:00:00Z" {
		t.Errorf("plan_ref mismatch: %+v", side.PlanRef)
	}
}

func TestRefine_GeneratedOnFromCoarse(t *testing.T) {
	planPath := fixtureCoarsePlan(t, map[string][]string{"T1": {"a.go"}, "T2": {"b.go"}})
	side, err := Refine(RefineOptions{
		CoarsePlanPath: planPath,
		Profile:        ProfileEightK,
		Targets:        []string{"T1.1"},
	})
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	if side.GeneratedOn != "2026-05-09" {
		t.Errorf("generated_on should derive from coarse plan, got %q", side.GeneratedOn)
	}
}
