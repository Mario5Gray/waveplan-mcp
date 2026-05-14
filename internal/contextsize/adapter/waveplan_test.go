package adapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/contextsize"
)

// fixturePlan is a minimal execution-waves JSON for testing the adapter.
var fixturePlan = `{
  "schema_version": 1,
  "generated_on": "2026-05-13",
  "plan": {
    "id": "test-plan",
    "title": "Test Plan"
  },
  "tasks": {
    "T1": {
      "title": "Do something",
      "plan_line": 10,
      "doc_refs": [
        "plan",
        "spec"
      ],
      "files": [
        "internal/foo/bar.go",
        "internal/foo/baz.go"
      ]
    },
    "T2": {
      "title": "Do another thing",
      "plan_line": 20,
      "doc_refs": [
        "plan"
      ],
      "files": [
        "internal/bar/thing.go"
      ]
    }
  },
  "units": {
    "T1.1": {
      "task": "T1",
      "title": "Unit one",
      "kind": "impl",
      "wave": 1,
      "plan_line": 30,
      "depends_on": [],
      "doc_refs": [
        "plan",
        "spec"
      ]
    },
    "T1.2": {
      "task": "T1",
      "title": "Unit two",
      "kind": "impl",
      "wave": 2,
      "plan_line": 40,
      "depends_on": [
        "T1.1"
      ],
      "doc_refs": [
        "spec"
      ]
    },
    "T2.1": {
      "task": "T2",
      "title": "Unit three",
      "kind": "test",
      "wave": 3,
      "plan_line": 50,
      "depends_on": [
        "T1.2"
      ],
      "doc_refs": [
        "plan"
      ]
    }
  },
  "doc_index": {
    "plan": {
      "path": "docs/superpowers/plans/test-plan.md",
      "line": 1,
      "kind": "plan"
    },
    "spec": {
      "path": "docs/superpowers/specs/test-spec.md",
      "line": 1,
      "kind": "spec"
    }
  }
}`

func makeFixturePlan(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test-plan-execution-waves.json")
	if err := os.WriteFile(p, []byte(fixturePlan), 0644); err != nil {
		t.Fatalf("write fixture plan: %v", err)
	}
	return p
}

func TestFromWaveplanTask_ResolvesFilesAndDocRefs(t *testing.T) {
	planPath := makeFixturePlan(t)

	candidate, err := FromWaveplanTask(planPath, "T1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if candidate.ID != "T1" {
		t.Errorf("expected id 'T1', got '%s'", candidate.ID)
	}
	if candidate.Title != "Do something" {
		t.Errorf("expected title 'Do something', got '%s'", candidate.Title)
	}
	if candidate.Source != "waveplan" {
		t.Errorf("expected source 'waveplan', got '%s'", candidate.Source)
	}
	if candidate.Description != "" {
		t.Errorf("expected empty description, got '%s'", candidate.Description)
	}

	// Should union task files + resolved doc_refs, deduplicated and sorted.
	expectedFiles := []string{
		"docs/superpowers/plans/test-plan.md",
		"docs/superpowers/specs/test-spec.md",
		"internal/foo/bar.go",
		"internal/foo/baz.go",
	}
	if len(candidate.ReferencedFiles) != len(expectedFiles) {
		t.Fatalf("expected %d referenced_files, got %d: %v", len(expectedFiles), len(candidate.ReferencedFiles), candidate.ReferencedFiles)
	}
	for i, f := range candidate.ReferencedFiles {
		if f != expectedFiles[i] {
			t.Errorf("expected referenced_files[%d] = '%s', got '%s'", i, expectedFiles[i], f)
		}
	}
}

func TestFromWaveplanUnit_ResolvesDocRefs(t *testing.T) {
	planPath := makeFixturePlan(t)

	candidate, err := FromWaveplanUnit(planPath, "T1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if candidate.ID != "T1.1" {
		t.Errorf("expected id 'T1.1', got '%s'", candidate.ID)
	}
	if candidate.Title != "Unit one" {
		t.Errorf("expected title 'Unit one', got '%s'", candidate.Title)
	}
	if candidate.Source != "waveplan" {
		t.Errorf("expected source 'waveplan', got '%s'", candidate.Source)
	}

	// Unit doc_refs: ["plan", "spec"] resolved through doc_index.
	expectedFiles := []string{
		"docs/superpowers/plans/test-plan.md",
		"docs/superpowers/specs/test-spec.md",
	}
	if len(candidate.ReferencedFiles) != len(expectedFiles) {
		t.Fatalf("expected %d referenced_files, got %d: %v", len(expectedFiles), len(candidate.ReferencedFiles), candidate.ReferencedFiles)
	}
	for i, f := range candidate.ReferencedFiles {
		if f != expectedFiles[i] {
			t.Errorf("expected referenced_files[%d] = '%s', got '%s'", i, expectedFiles[i], f)
		}
	}
}

func TestFromWaveplanTask_UnknownTaskID_ReturnsError(t *testing.T) {
	planPath := makeFixturePlan(t)

	_, err := FromWaveplanTask(planPath, "T99")
	if err == nil {
		t.Fatal("expected error for unknown task ID, got nil")
	}
}

func TestFromWaveplanUnit_UnknownUnitID_ReturnsError(t *testing.T) {
	planPath := makeFixturePlan(t)

	_, err := FromWaveplanUnit(planPath, "T99.1")
	if err == nil {
		t.Fatal("expected error for unknown unit ID, got nil")
	}
}

func TestFromWaveplanTask_DedupedSortedReferencedFiles(t *testing.T) {
	// Plan where doc_refs overlap with task files.
	overlappingPlan := `{
  "schema_version": 1,
  "generated_on": "2026-05-13",
  "plan": {"id": "overlap", "title": "Overlap Plan"},
  "tasks": {
    "T1": {
      "title": "Overlapping",
      "plan_line": 10,
      "doc_refs": ["plan", "shared_ref"],
      "files": ["docs/superpowers/plans/test-plan.md", "internal/unique.go"]
    }
  },
  "units": {},
  "doc_index": {
    "plan": {"path": "docs/superpowers/plans/test-plan.md", "line": 1, "kind": "plan"},
    "shared_ref": {"path": "internal/unique.go", "line": 5, "kind": "code"}
  }
}`
	dir := t.TempDir()
	p := filepath.Join(dir, "overlap.json")
	os.WriteFile(p, []byte(overlappingPlan), 0644)

	candidate, err := FromWaveplanTask(p, "T1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be deduplicated: "docs/superpowers/plans/test-plan.md" and "internal/unique.go"
	expectedFiles := []string{"docs/superpowers/plans/test-plan.md", "internal/unique.go"}
	if len(candidate.ReferencedFiles) != len(expectedFiles) {
		t.Fatalf("expected %d deduplicated files, got %d: %v", len(expectedFiles), len(candidate.ReferencedFiles), candidate.ReferencedFiles)
	}
	for i, f := range expectedFiles {
		if candidate.ReferencedFiles[i] != f {
			t.Errorf("expected [%d] = '%s', got '%s'", i, f, candidate.ReferencedFiles[i])
		}
	}
}

func TestFromWaveplanTask_TaskDependsOnCollapsesFromUnitEdges(t *testing.T) {
	// T2.1 depends on T1.2, T1.2 depends on T1.1.
	// T2's depends_on should collapse to ["T1.1", "T1.2"] (all external unit deps transitively).
	planPath := makeFixturePlan(t)

	candidate, err := FromWaveplanTask(planPath, "T2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// T2.1 depends on ["T1.2"], T1.2 is a unit in a different task (T1), so it's external.
	// T1.2 depends on ["T1.1"], T1.1 is in same task T1, so it's internal.
	// Collapsed external deps for T2: ["T1.2"]
	if len(candidate.DependsOn) != 1 {
		t.Fatalf("expected 1 depends_on, got %d: %v", len(candidate.DependsOn), candidate.DependsOn)
	}
	if len(candidate.DependsOn) > 0 && candidate.DependsOn[0] != "T1.2" {
		t.Errorf("expected depends_on ['T1.2'], got %v", candidate.DependsOn)
	}
}

func TestFromWaveplanTask_NoExternalDeps_EmptyDependsOn(t *testing.T) {
	// T1's units (T1.1, T1.2) only depend on each other, no external deps.
	planPath := makeFixturePlan(t)

	candidate, err := FromWaveplanTask(planPath, "T1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(candidate.DependsOn) != 0 {
		t.Errorf("expected empty depends_on for T1, got %v", candidate.DependsOn)
	}
}

func TestFromWaveplanUnit_DependsOnFromUnit(t *testing.T) {
	planPath := makeFixturePlan(t)

	// T1.2 depends on ["T1.1"]
	candidate, err := FromWaveplanUnit(planPath, "T1.2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(candidate.DependsOn) != 1 {
		t.Fatalf("expected 1 depends_on, got %d: %v", len(candidate.DependsOn), candidate.DependsOn)
	}
	if candidate.DependsOn[0] != "T1.1" {
		t.Errorf("expected depends_on ['T1.1'], got %v", candidate.DependsOn)
	}
}

func TestFromWaveplanTask_EmptyPlanPath_ReturnsError(t *testing.T) {
	_, err := FromWaveplanTask("", "T1")
	if err == nil {
		t.Fatal("expected error for empty plan path, got nil")
	}
}

func TestFromWaveplanUnit_EmptyPlanPath_ReturnsError(t *testing.T) {
	_, err := FromWaveplanUnit("", "T1.1")
	if err == nil {
		t.Fatal("expected error for empty plan path, got nil")
	}
}

func TestFromWaveplanTask_MissingDocRef_ReturnsError(t *testing.T) {
	// Plan with a doc_ref that doesn't exist in doc_index.
	brokenPlan := `{
  "schema_version": 1,
  "generated_on": "2026-05-13",
  "plan": {"id": "broken", "title": "Broken Plan"},
  "tasks": {
    "T1": {
      "title": "Broken",
      "plan_line": 10,
      "doc_refs": ["plan", "nonexistent_ref"],
      "files": []
    }
  },
  "units": {},
  "doc_index": {
    "plan": {"path": "docs/plan.md", "line": 1, "kind": "plan"}
  }
}`
	dir := t.TempDir()
	p := filepath.Join(dir, "broken.json")
	os.WriteFile(p, []byte(brokenPlan), 0644)

	_, err := FromWaveplanTask(p, "T1")
	if err == nil {
		t.Fatal("expected error for missing doc_ref, got nil")
	}
}

func TestCandidate_SourceIsWaveplan(t *testing.T) {
	planPath := makeFixturePlan(t)

	taskCandidate, err := FromWaveplanTask(planPath, "T1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taskCandidate.Source != "waveplan" {
		t.Errorf("expected task candidate source 'waveplan', got '%s'", taskCandidate.Source)
	}

	unitCandidate, err := FromWaveplanUnit(planPath, "T1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unitCandidate.Source != "waveplan" {
		t.Errorf("expected unit candidate source 'waveplan', got '%s'", unitCandidate.Source)
	}
}

func TestCandidate_StructurallyValidForEstimator(t *testing.T) {
	// Verify the candidate is structurally valid for contextsize.ContextCandidate.
	planPath := makeFixturePlan(t)

	candidate, err := FromWaveplanTask(planPath, "T1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be a valid ContextCandidate that can be used with the estimator.
	// This test verifies no nil slices or invalid fields.
	var _ contextsize.ContextCandidate = candidate
	if candidate.ID == "" {
		t.Error("expected non-empty ID")
	}
	if candidate.Title == "" {
		t.Error("expected non-empty title")
	}
}