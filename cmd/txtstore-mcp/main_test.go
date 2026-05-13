package main

import "testing"

func TestBuildSwimPlanDocRequiresTaskAndUnitPayloads(t *testing.T) {
	args := map[string]any{
		"filepath": "tmp/plan.md",
		"title":    "Swim Plan Source",
		"meta": map[string]any{
			"schema_version":  1,
			"generated_on":    "2026-05-13",
			"plan_version":    1,
			"plan_generation": "2026-05-13T00:00:00Z",
		},
		"plan": map[string]any{
			"plan_id":       "p",
			"plan_title":    "Plan",
			"plan_doc_path": "plan.md",
			"spec_doc_path": "spec.md",
		},
		"doc_index": []any{},
		"fp_index":  []any{},
		"tasks":     []any{},
		"units":     []any{},
	}

	doc, err := buildSwimPlanDoc(args)
	if err != nil {
		t.Fatalf("buildSwimPlanDoc() error = %v", err)
	}
	if doc.Title != "Swim Plan Source" {
		t.Fatalf("doc.Title = %q, want %q", doc.Title, "Swim Plan Source")
	}
}
