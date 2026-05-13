package txtstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validSwimDoc() SwimPlanDoc {
	return SwimPlanDoc{
		Title: "Swim Plan Source",
		Meta: SwimMeta{
			SchemaVersion:  1,
			GeneratedOn:    "2026-05-13",
			PlanVersion:    1,
			PlanGeneration: "2026-05-13T00:00:00Z",
		},
		Plan: SwimPlan{
			PlanID:      "txtstore-swim-writer",
			PlanTitle:   "txtstore SWIM Writer",
			PlanDocPath: "docs/superpowers/plans/2026-05-13-txtstore-swim-markdown-writer.md",
			SpecDocPath: "docs/specs/swim-markdown-plan-format-v1.md",
		},
		DocIndex: []SwimDocRef{
			{Ref: "spec", Path: "docs/specs/swim-markdown-plan-format-v1.md", Line: 1, Kind: "spec"},
			{Ref: "plan", Path: "docs/superpowers/plans/2026-05-13-txtstore-swim-markdown-writer.md", Line: 1, Kind: "plan"},
		},
		FPIndex: []SwimFPRef{
			{FPRef: "FP-example", FPID: "backend-id-1"},
		},
		Tasks: []SwimTask{
			{TaskID: "T2", Title: "Second task", PlanLine: 20, DocRefs: []string{"plan"}, Files: []string{"cmd/txtstore/main.go"}},
			{TaskID: "T1", Title: "First task", PlanLine: 10, DocRefs: []string{"spec"}, Files: nil},
		},
		Units: []SwimUnit{
			{UnitID: "T2.1", TaskID: "T2", Title: "Second unit", Kind: "impl", Wave: 2, PlanLine: 21, DependsOn: []string{"T1.1"}, FPRefs: []string{"FP-example"}, DocRefs: []string{"plan"}},
			{UnitID: "T1.1", TaskID: "T1", Title: "First unit", Kind: "doc", Wave: 1, PlanLine: 11, DependsOn: nil, FPRefs: []string{"FP-example"}, DocRefs: []string{"spec"}},
		},
	}
}

func TestValidateSwimPlanValid(t *testing.T) {
	if err := ValidateSwimPlan(validSwimDoc()); err != nil {
		t.Fatalf("ValidateSwimPlan() error = %v", err)
	}
}

func TestValidateSwimPlanRejectsMissingDocRef(t *testing.T) {
	doc := validSwimDoc()
	doc.Tasks[0].DocRefs = []string{"missing"}
	if err := ValidateSwimPlan(doc); err == nil {
		t.Fatal("expected missing doc ref error")
	}
}

func TestValidateSwimPlanRejectsCycle(t *testing.T) {
	doc := validSwimDoc()
	doc.Units[1].DependsOn = []string{"T2.1"}
	if err := ValidateSwimPlan(doc); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestRenderSwimPlanSortsAndNormalizes(t *testing.T) {
	out, err := RenderSwimPlan(validSwimDoc())
	if err != nil {
		t.Fatalf("RenderSwimPlan() error = %v", err)
	}
	if !strings.Contains(out, "## Meta") || !strings.Contains(out, "## Units") {
		t.Fatal("missing required sections")
	}
	if !strings.Contains(out, "| T1 | First task |") {
		t.Fatal("expected tasks sorted by numeric task id")
	}
	if !strings.Contains(out, "| T1.1 | T1 | First unit |") {
		t.Fatal("expected units sorted by numeric unit id")
	}
	if !strings.Contains(out, "| T1 | First task | 10 | spec | - |") {
		t.Fatal("expected empty file list to render as '-'")
	}
}

func TestFileStoreWriteSwimPlanOverwritesCanonicalDocument(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "plan.md")
	if err := os.WriteFile(path, []byte("stale\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	fs := New(path)
	if err := fs.WriteSwimPlan(validSwimDoc()); err != nil {
		t.Fatalf("WriteSwimPlan() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(data)
	if strings.Contains(got, "stale") {
		t.Fatal("expected stale content to be replaced")
	}
	if !strings.HasPrefix(got, "# Swim Plan Source\n") {
		t.Fatal("expected top-level swim plan heading")
	}
}
