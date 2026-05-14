package contextsize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEstimate_NoSignal(t *testing.T) {
	tmpDir := t.TempDir()
	c := ContextCandidate{
		ID:          "T1.1",
		Title:       "Empty candidate",
		Description: "",
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.EstimatedTokens != 0 {
		t.Errorf("expected 0 tokens, got %d", est.EstimatedTokens)
	}
	if est.Fit != "under" {
		t.Errorf("expected fit 'under', got '%s'", est.Fit)
	}
	if est.Recommendation != "merge_candidate" {
		t.Errorf("expected recommendation 'merge_candidate', got '%s'", est.Recommendation)
	}
	if est.Confidence != "low" {
		t.Errorf("expected confidence 'low', got '%s'", est.Confidence)
	}
}

func TestEstimate_SmallGoFile(t *testing.T) {
	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "test.go")
	os.WriteFile(goFile, []byte("package test\n"), 0644) // 14 bytes

	c := ContextCandidate{
		ID:              "T1.1",
		Title:           "Small Go file",
		ReferencedFiles: []string{"test.go"},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 14 bytes / 3 = 4 tokens (integer division)
	if est.EstimatedTokens != 4 {
		t.Errorf("expected 4 tokens, got %d", est.EstimatedTokens)
	}
	if est.Fit != "under" {
		t.Errorf("expected fit 'under', got '%s'", est.Fit)
	}
	if est.Confidence != "high" {
		t.Errorf("expected confidence 'high', got '%s'", est.Confidence)
	}
}

func TestEstimate_LargeGoFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a 100k byte Go file
	data := make([]byte, 100000)
	for i := range data {
		data[i] = 'x'
	}
	goFile := filepath.Join(tmpDir, "large.go")
	os.WriteFile(goFile, data, 0644)

	c := ContextCandidate{
		ID:              "T1.1",
		Title:           "Large Go file",
		ReferencedFiles: []string{"large.go"},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 100000 / 3 = 33333 tokens
	if est.EstimatedTokens != 33333 {
		t.Errorf("expected 33333 tokens, got %d", est.EstimatedTokens)
	}
	if est.Fit != "under" {
		t.Errorf("expected fit 'under', got '%s'", est.Fit)
	}
	if est.Confidence != "medium" {
		t.Errorf("expected confidence 'medium' (downgraded from high), got '%s'", est.Confidence)
	}
}

func TestEstimate_ExceedsBudget(t *testing.T) {
	tmpDir := t.TempDir()
	// Create files totaling ~600k bytes = 200k tokens (exceeds 192k max)
	for _, name := range []string{"a.go", "b.go"} {
		data := make([]byte, 300000)
		os.WriteFile(filepath.Join(tmpDir, name), data, 0644)
	}

	c := ContextCandidate{
		ID:              "T1.1",
		Title:           "Exceeds budget",
		ReferencedFiles: []string{"a.go", "b.go"},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 600000 / 3 = 200000 tokens
	if est.EstimatedTokens != 200000 {
		t.Errorf("expected 200000 tokens, got %d", est.EstimatedTokens)
	}
	if est.Fit != "over" {
		t.Errorf("expected fit 'over', got '%s'", est.Fit)
	}
	if est.Recommendation != "split" {
		t.Errorf("expected recommendation 'split', got '%s'", est.Recommendation)
	}
}

func TestEstimate_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	c := ContextCandidate{
		ID:              "T1.1",
		Title:           "Missing file",
		ReferencedFiles: []string{"nonexistent.go"},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.EstimatedTokens != 0 {
		t.Errorf("expected 0 tokens (file missing), got %d", est.EstimatedTokens)
	}
	if est.Confidence != "low" {
		t.Errorf("expected confidence 'low', got '%s'", est.Confidence)
	}
	if len(est.MissingFiles) != 1 || est.MissingFiles[0] != "nonexistent.go" {
		t.Errorf("expected missing_files ['nonexistent.go'], got %v", est.MissingFiles)
	}
}

func TestEstimate_MixedExtensions(t *testing.T) {
	tmpDir := t.TempDir()
	// Go file: 10k bytes
	goData := make([]byte, 10000)
	os.WriteFile(filepath.Join(tmpDir, "code.go"), goData, 0644)
	// Markdown file: 10k bytes
	mdData := make([]byte, 10000)
	os.WriteFile(filepath.Join(tmpDir, "doc.md"), mdData, 0644)
	// Unknown extension: 5k bytes
	unkData := make([]byte, 5000)
	os.WriteFile(filepath.Join(tmpDir, "weird.xyz"), unkData, 0644)

	c := ContextCandidate{
		ID:              "T1.1",
		Title:           "Mixed extensions",
		ReferencedFiles: []string{"code.go", "doc.md", "weird.xyz"},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Go: 10000/3 = 3333, Markdown: 10000/4 = 2500, Unknown: 5000/3 = 1666
	// Total: 3333 + 2500 + 1666 = 7499
	if est.EstimatedTokens != 7499 {
		t.Errorf("expected 7499 tokens, got %d", est.EstimatedTokens)
	}
	if est.Confidence != "medium" {
		t.Errorf("expected confidence 'medium' (unknown ext downgrade), got '%s'", est.Confidence)
	}
	if len(est.UnknownFiles) != 1 || est.UnknownFiles[0] != "weird.xyz" {
		t.Errorf("expected unknown_files ['weird.xyz'], got %v", est.UnknownFiles)
	}
}

func TestEstimate_HeadingNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	// File with content but no "Foo" heading
	mdData := []byte("# Bar\nSome content\n")
	os.WriteFile(filepath.Join(tmpDir, "doc.md"), mdData, 0644)

	c := ContextCandidate{
		ID:                 "T1.1",
		Title:              "Heading not found",
		ReferencedSections: []SectionRef{{Path: "doc.md", Heading: "Foo"}},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.EstimatedTokens != 0 {
		t.Errorf("expected 0 tokens (heading not found), got %d", est.EstimatedTokens)
	}
	if est.Confidence != "medium" {
		t.Errorf("expected confidence 'medium' (heading not found downgrade), got '%s'", est.Confidence)
	}
	if len(est.MissingSections) != 1 {
		t.Errorf("expected 1 missing section, got %d", len(est.MissingSections))
	}
	if est.MissingSections[0].Heading != "Foo" {
		t.Errorf("expected missing section heading 'Foo', got '%s'", est.MissingSections[0].Heading)
	}
}

func TestEstimate_MissingSectionFileLowersConfidence(t *testing.T) {
	tmpDir := t.TempDir()

	c := ContextCandidate{
		ID:                 "T1.1",
		Title:              "Missing section file",
		ReferencedSections: []SectionRef{{Path: "missing.md", Heading: "Architecture"}},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.Confidence != "low" {
		t.Errorf("expected confidence 'low', got %q", est.Confidence)
	}
	if len(est.MissingFiles) != 1 || est.MissingFiles[0] != "missing.md" {
		t.Errorf("expected missing_files ['missing.md'], got %v", est.MissingFiles)
	}
	if len(est.MissingSections) != 0 {
		t.Errorf("expected no missing_sections entry for a missing file, got %v", est.MissingSections)
	}
}

func TestEstimate_SectionTokensIncludeHeading(t *testing.T) {
	tmpDir := t.TempDir()
	mdData := []byte("## Architecture\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "doc.md"), mdData, 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	c := ContextCandidate{
		ID:                 "T1.1",
		Title:              "Heading only section",
		ReferencedSections: []SectionRef{{Path: "doc.md", Heading: "Architecture"}},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.EstimatedTokens != len(mdData)/4 {
		t.Errorf("expected %d tokens, got %d", len(mdData)/4, est.EstimatedTokens)
	}
}

func TestEstimate_UnderBudgetThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	// 5k bytes of Go code = ~1666 tokens
	data := make([]byte, 5000)
	os.WriteFile(filepath.Join(tmpDir, "small.go"), data, 0644)

	c := ContextCandidate{
		ID:              "T1.1",
		Title:           "Under threshold",
		ReferencedFiles: []string{"small.go"},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.EstimatedTokens != 1666 {
		t.Errorf("expected 1666 tokens, got %d", est.EstimatedTokens)
	}
	if est.Recommendation != "merge_candidate" {
		t.Errorf("expected 'merge_candidate' (1666 < 64000*0.3=19200), got '%s'", est.Recommendation)
	}
}

func TestEstimate_WithinBudget(t *testing.T) {
	tmpDir := t.TempDir()
	// 300k bytes of Go code = 100000 tokens
	data := make([]byte, 300000)
	os.WriteFile(filepath.Join(tmpDir, "medium.go"), data, 0644)

	c := ContextCandidate{
		ID:              "T1.1",
		Title:           "Within budget",
		ReferencedFiles: []string{"medium.go"},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.EstimatedTokens != 100000 {
		t.Errorf("expected 100000 tokens, got %d", est.EstimatedTokens)
	}
	if est.Fit != "within" {
		t.Errorf("expected fit 'within', got '%s'", est.Fit)
	}
	if est.Recommendation != "keep" {
		t.Errorf("expected 'keep', got '%s'", est.Recommendation)
	}
}

func TestEstimate_DescriptionTokens(t *testing.T) {
	tmpDir := t.TempDir()

	c := ContextCandidate{
		ID:          "T1.1",
		Title:       "Description only",
		Description: "0123456789012345678901234567890123456789", // exactly 40 chars
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 40 runes / 4 = 10 tokens
	if est.EstimatedTokens != 10 {
		t.Errorf("expected 10 tokens, got %d", est.EstimatedTokens)
	}
}

func TestEstimate_CountsUniqueLocalImportsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	goMod := []byte("module example.com/contextsize-test\n\ngo 1.26\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), goMod, 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	goFile := []byte(`package test

import (
	"fmt"
	alias "example.com/contextsize-test/internal/foo"
	"example.com/contextsize-test/internal/bar"
	"example.com/contextsize-test/internal/foo"
)

func _() {
	fmt.Println(alias.Name)
}
`)
	if err := os.WriteFile(filepath.Join(tmpDir, "test.go"), goFile, 0644); err != nil {
		t.Fatalf("write test.go: %v", err)
	}

	c := ContextCandidate{
		ID:              "T1.1",
		Title:           "Local imports only",
		ReferencedFiles: []string{"test.go"},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, driver := range est.Drivers {
		if strings.Contains(driver, "2 imported local packages") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected driver reporting 2 imported local packages, got %v", est.Drivers)
	}
}
