# Context Sizer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a deterministic context estimation tool that reads issue candidates and returns token estimates with fit classification, confidence scoring, and split/merge recommendations.

**Architecture:** Go package `internal/contextsize` with core estimation logic, JSON handling, and a CLI binary `cmd/contextsize`. Python wrapper in `waveplan-cli` shells to the Go binary. No external dependencies beyond the Go standard library.

**Tech Stack:** Go 1.26, standard library only (encoding/json, os, path/filepath, strings, sort, regexp)

---

### Task 1: Create types.go

**Files:**
- Create: `internal/contextsize/types.go`

- [ ] **Step 1: Write the types**

Create `internal/contextsize/types.go`:

```go
package contextsize

// SectionRef references a heading within a file.
type SectionRef struct {
	Path    string `json:"path"`
	Heading string `json:"heading"`
}

// ContextCandidate is a source-agnostic issue candidate for context estimation.
type ContextCandidate struct {
	ID                 string        `json:"id"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	ReferencedFiles    []string      `json:"referenced_files"`
	ReferencedSections []SectionRef  `json:"referenced_sections"`
	DependsOn          []string      `json:"depends_on"`
	Source             string        `json:"source"`
}

// Budget defines the acceptable token range.
type Budget struct {
	MinTokens int
	MaxTokens int
}

// ContextEstimate is the output of context estimation.
type ContextEstimate struct {
	EstimatedTokens   int          `json:"estimated_tokens"`
	BudgetMin         int          `json:"budget_min"`
	BudgetMax         int          `json:"budget_max"`
	Fit               string       `json:"fit"`
	Confidence        string       `json:"confidence"`
	Drivers           []string     `json:"drivers"`
	Recommendation    string       `json:"recommendation"`
	MissingFiles      []string     `json:"missing_files"`
	MissingSections   []SectionRef `json:"missing_sections"`
	UnknownFiles      []string     `json:"unknown_files"`
	SplitCandidates   []string     `json:"split_candidates"`
	MergeCandidates   []string     `json:"merge_candidates"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/contextsize/`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add internal/contextsize/types.go
git commit -m "feat: add contextsize types (ContextCandidate, Budget, ContextEstimate)"
```

---

### Task 2: Create estimate.go with token estimation logic

**Files:**
- Create: `internal/contextsize/estimate.go`
- Create: `internal/contextsize/estimate_test.go`

- [ ] **Step 1: Write the test file first**

Create `internal/contextsize/estimate_test.go`:

```go
package contextsize

import (
	"os"
	"path/filepath"
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
		ID:            "T1.1",
		Title:         "Small Go file",
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
		ID:            "T1.1",
		Title:         "Large Go file",
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
	// Create two 100k byte files = 200k bytes total
	for _, name := range []string{"a.go", "b.go"} {
		data := make([]byte, 100000)
		os.WriteFile(filepath.Join(tmpDir, name), data, 0644)
	}

	c := ContextCandidate{
		ID:            "T1.1",
		Title:         "Exceeds budget",
		ReferencedFiles: []string{"a.go", "b.go"},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 200000 / 3 = 66666 tokens
	if est.EstimatedTokens != 66666 {
		t.Errorf("expected 66666 tokens, got %d", est.EstimatedTokens)
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
		ID:            "T1.1",
		Title:         "Missing file",
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
		ID:            "T1.1",
		Title:         "Mixed extensions",
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

func TestEstimate_UnderBudgetThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	// 5k bytes of Go code = ~1666 tokens
	data := make([]byte, 5000)
	os.WriteFile(filepath.Join(tmpDir, "small.go"), data, 0644)

	c := ContextCandidate{
		ID:            "T1.1",
		Title:         "Under threshold",
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
	// 100k bytes of Go code = ~33333 tokens
	data := make([]byte, 100000)
	os.WriteFile(filepath.Join(tmpDir, "medium.go"), data, 0644)

	c := ContextCandidate{
		ID:            "T1.1",
		Title:         "Within budget",
		ReferencedFiles: []string{"medium.go"},
	}
	budget := Budget{MinTokens: 64000, MaxTokens: 192000}
	e := &Estimator{BaseDir: tmpDir}

	est, err := e.Estimate(c, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.EstimatedTokens != 33333 {
		t.Errorf("expected 33333 tokens, got %d", est.EstimatedTokens)
	}
	if est.Fit != "under" {
		t.Errorf("expected fit 'under', got '%s'", est.Fit)
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
		Description: "This is a test description with exactly 40 characters!",
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/contextsize/ -run TestEstimate_NoSignal -v`
Expected: FAIL with "undefined: Estimator" or "undefined: Estimate"

- [ ] **Step 3: Write the Estimator implementation**

Create `internal/contextsize/estimate.go`:

```go
package contextsize

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// codeExtensions are file extensions treated as code (higher density).
var codeExtensions = map[string]bool{
	".go":    true,
	".json":  true,
	".yaml":  true,
	".yml":   true,
	".toml":  true,
	".proto": true,
	".html":  true,
	".css":   true,
	".js":    true,
	".ts":    true,
	".tsx":   true,
	".py":    true,
	".sh":    true,
	".sql":   true,
}

// proseExtensions are file extensions treated as prose (lower density).
var proseExtensions = map[string]bool{
	".md":  true,
	".txt": true,
	".rst": true,
}

// headingRe matches markdown headings (## Title or ### Title, etc.).
var headingRe = regexp.MustCompile(`^#{1,6}\s+(.+)$`)

// Estimator performs deterministic context estimation.
type Estimator struct {
	BaseDir string // root for resolving relative file paths; empty = cwd
}

// Estimate returns a ContextEstimate for the given candidate and budget.
// Returns error only for invalid inputs (zero/negative budget) or IO conditions
// that prevent deterministic behavior (unreadable directory).
// Sparse candidates (no signal) return a valid estimate with low confidence.
func (e *Estimator) Estimate(c ContextCandidate, budget Budget) (ContextEstimate, error) {
	if budget.MinTokens <= 0 || budget.MaxTokens <= 0 {
		return ContextEstimate{}, fmt.Errorf("budget must have positive min and max tokens")
	}
	if budget.MinTokens > budget.MaxTokens {
		return ContextEstimate{}, fmt.Errorf("budget min (%d) exceeds max (%d)", budget.MinTokens, budget.MaxTokens)
	}

	est := ContextEstimate{
		BudgetMin:     budget.MinTokens,
		BudgetMax:     budget.MaxTokens,
		Drivers:       []string{},
		MissingFiles:  []string{},
		MissingSections: []SectionRef{},
		UnknownFiles:  []string{},
	}

	var totalTokens int
	var fileCount int
	var localImportCount int

	// Estimate tokens from referenced files.
	for _, refFile := range c.ReferencedFiles {
		path := refFile
		if e.BaseDir != "" {
			path = filepath.Join(e.BaseDir, refFile)
		}

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				est.MissingFiles = append(est.MissingFiles, refFile)
				continue
			}
			// Other IO error — treat as missing.
			est.MissingFiles = append(est.MissingFiles, refFile)
			continue
		}

		fileCount++
		bytes := info.Size()
		tokens := estimateFileTokens(path, bytes)
		totalTokens += tokens

		// Track local imports for Go files.
		if strings.HasSuffix(path, ".go") {
			localImportCount += countLocalImports(path)
		}

		// Check for unknown extension.
		ext := strings.ToLower(filepath.Ext(path))
		if !codeExtensions[ext] && !proseExtensions[ext] {
			est.UnknownFiles = append(est.UnknownFiles, refFile)
		}
	}

	// Estimate tokens from description.
	if c.Description != "" {
		totalTokens += len([]rune(c.Description)) / 4
	}

	// Estimate tokens from referenced sections.
	for _, section := range c.ReferencedSections {
		path := section.Path
		if e.BaseDir != "" {
			path = filepath.Join(e.BaseDir, section.Path)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			// File missing — section also missing.
			est.MissingSections = append(est.MissingSections, section)
			continue
		}

		sectionTokens := estimateSectionTokens(content, section.Heading)
		if sectionTokens == -1 {
			// Heading not found.
			est.MissingSections = append(est.MissingSections, section)
		} else {
			totalTokens += sectionTokens
		}
	}

	// Build drivers.
	est.Drivers = append(est.Drivers, fmt.Sprintf("%d referenced files", fileCount))
	est.Drivers = append(est.Drivers, fmt.Sprintf("%d imported local packages", localImportCount))
	if c.Description != "" {
		est.Drivers = append(est.Drivers, fmt.Sprintf("description %d chars", len([]rune(c.Description))))
	}
	for _, missing := range est.MissingFiles {
		est.Drivers = append(est.Drivers, fmt.Sprintf("file not found: %s", missing))
	}
	for _, missing := range est.MissingSections {
		est.Drivers = append(est.Drivers, fmt.Sprintf("section heading not found: %s#%s", missing.Path, missing.Heading))
	}
	for _, unknown := range est.UnknownFiles {
		info, _ := os.Stat(filepath.Join(e.BaseDir, unknown))
		if info != nil {
			est.Drivers = append(est.Drivers, fmt.Sprintf("unknown extension: %s (%d bytes)", unknown, info.Size()))
		}
	}

	// Classify fit.
	est.Fit = classifyFit(totalTokens, budget)

	// Determine recommendation.
	est.Recommendation = classifyRecommendation(totalTokens, budget, est.Fit)

	// Determine confidence.
	est.Confidence = determineConfidence(est, fileCount)

	// Build split/merge candidates.
	est.SplitCandidates = buildSplitCandidates(est, c)
	est.MergeCandidates = buildMergeCandidates(est, budget)

	est.EstimatedTokens = totalTokens

	return est, nil
}

// estimateFileTokens returns token estimate for a file based on extension and size.
func estimateFileTokens(path string, bytes int64) int {
	ext := strings.ToLower(filepath.Ext(path))
	if codeExtensions[ext] {
		return int(bytes) / 3
	}
	if proseExtensions[ext] {
		return int(bytes) / 4
	}
	// Unknown extension: assume code-like density (conservative).
	return int(bytes) / 3
}

// estimateSectionTokens returns token estimate for a markdown section, or -1 if heading not found.
func estimateSectionTokens(content []byte, heading string) int {
	lines := strings.Split(string(content), "\n")
	found := false
	var sectionBytes int

	for i, line := range lines {
		matches := headingRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		if matches[1] == heading {
			found = true
			// Count bytes from this heading to next heading at same/lower level or EOF.
			for j := i + 1; j < len(lines); j++ {
				nextMatch := headingRe.FindStringSubmatch(lines[j])
				if nextMatch != nil {
					// Check heading level (count # characters).
					currentLevel := 0
					for _, c := range line {
						if c == '#' {
							currentLevel++
						}
					}
					nextLevel := 0
					for _, c := range nextMatch[0] {
						if c == '#' {
							nextLevel++
						}
					}
					if nextLevel <= currentLevel {
						// Next heading at same or lower level — stop here.
						break
					}
				}
				sectionBytes += len(lines[j]) + 1 // +1 for newline
			}
			break
		}
	}

	if !found {
		return -1
	}

	// Sections are prose.
	return sectionBytes / 4
}

// countLocalImports counts import paths in a Go file.
func countLocalImports(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	// Simple heuristic: count import paths in import blocks.
	text := string(content)
	importCount := 0
	inImportBlock := false

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") {
			// Single import: import "path"
			if !strings.Contains(trimmed, "(") {
				importCount++
				continue
			}
			inImportBlock = true
			continue
		}
		if inImportBlock {
			if strings.Contains(trimmed, ")") {
				inImportBlock = false
				continue
			}
			// Count this import line.
			importCount++
		}
	}

	return importCount
}

// classifyFit returns "within", "over", or "under".
func classifyFit(tokens int, budget Budget) string {
	if tokens >= budget.MinTokens && tokens <= budget.MaxTokens {
		return "within"
	}
	if tokens > budget.MaxTokens {
		return "over"
	}
	return "under"
}

// classifyRecommendation returns "keep", "split", or "merge_candidate".
func classifyRecommendation(tokens int, budget Budget, fit string) string {
	if fit == "over" {
		return "split"
	}
	if fit == "under" && tokens < budget.MinTokens*3/10 {
		return "merge_candidate"
	}
	return "keep"
}

// determineConfidence returns "high", "medium", or "low".
func determineConfidence(est ContextEstimate, fileCount int) string {
	confidence := "high"

	// Missing files → low.
	if len(est.MissingFiles) > 0 {
		return "low"
	}

	// Missing sections → downgrade one level.
	if len(est.MissingSections) > 0 {
		confidence = downgrade(confidence)
	}

	// Unknown extensions → downgrade one level.
	if len(est.UnknownFiles) > 0 {
		confidence = downgrade(confidence)
	}

	// No signal at all → low.
	if fileCount == 0 && est.EstimatedTokens == 0 {
		return "low"
	}

	return confidence
}

// downgrade downgrades confidence by one level.
func downgrade(confidence string) string {
	switch confidence {
	case "high":
		return "medium"
	case "medium":
		return "low"
	default:
		return "low"
	}
}

// buildSplitCandidates returns human-readable split hints.
func buildSplitCandidates(est ContextEstimate, c ContextCandidate) []string {
	var candidates []string
	if est.Fit != "over" {
		return candidates
	}

	// Simple heuristic: if multiple files, suggest package-based split.
	if len(c.ReferencedFiles) > 2 {
		// Count unique directory prefixes as a proxy for packages.
		packages := make(map[string]bool)
		for _, f := range c.ReferencedFiles {
			dir := filepath.Dir(f)
			packages[dir] = true
		}
		if len(packages) > 1 {
			candidates = append(candidates, fmt.Sprintf("Consider splitting: %d files touch different directories", len(c.ReferencedFiles)))
		}
	}

	return candidates
}

// buildMergeCandidates returns human-readable merge hints.
func buildMergeCandidates(est ContextEstimate, budget Budget) []string {
	var candidates []string
	if est.Fit != "under" || est.Recommendation != "merge_candidate" {
		return candidates
	}

	candidates = append(candidates, fmt.Sprintf("Consider merging: %d tokens, well under %d minimum budget", est.EstimatedTokens, budget.MinTokens))

	return candidates
}
```

- [ ] **Step 4: Run test to verify all tests pass**

Run: `go test ./internal/contextsize/ -v`
Expected: All 10 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/contextsize/estimate.go internal/contextsize/estimate_test.go
git commit -m "feat: add context estimator with token estimation and confidence scoring"
```

---

### Task 3: Create json.go for JSON serialization

**Files:**
- Create: `internal/contextsize/json.go`
- Create: `internal/contextsize/json_test.go`

- [ ] **Step 1: Write the JSON handling code**

Create `internal/contextsize/json.go`:

```go
package contextsize

import (
	"bytes"
	"encoding/json"
)

// DecodeCandidateJSON decodes a ContextCandidate from JSON with strict decoding.
func DecodeCandidateJSON(data []byte) (ContextCandidate, error) {
	var c ContextCandidate
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return ContextCandidate{}, err
	}
	return c, nil
}

// EncodeEstimateJSON encodes a ContextEstimate to pretty-printed JSON.
func EncodeEstimateJSON(e ContextEstimate) ([]byte, error) {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(e); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 2: Write the tests**

Create `internal/contextsize/json_test.go`:

```go
package contextsize

import (
	"encoding/json"
	"testing"
)

func TestDecodeCandidateJSON_RoundTrip(t *testing.T) {
	original := ContextCandidate{
		ID:          "T1.1",
		Title:       "Test task",
		Description: "A test description",
		ReferencedFiles: []string{"main.go", "internal/foo.go"},
		ReferencedSections: []SectionRef{
			{Path: "docs/spec.md", Heading: "Architecture"},
		},
		DependsOn: []string{"T1.0"},
		Source:    "waveplan",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	decoded, err := DecodeCandidateJSON(data)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Title != original.Title {
		t.Errorf("Title mismatch: got %q, want %q", decoded.Title, original.Title)
	}
	if len(decoded.ReferencedFiles) != len(original.ReferencedFiles) {
		t.Errorf("ReferencedFiles length mismatch: got %d, want %d", len(decoded.ReferencedFiles), len(original.ReferencedFiles))
	}
	if decoded.Source != original.Source {
		t.Errorf("Source mismatch: got %q, want %q", decoded.Source, original.Source)
	}
}

func TestDecodeCandidateJSON_RejectsUnknownFields(t *testing.T) {
	input := []byte(`{"id":"T1.1","title":"Test","oops":"field"}`)
	_, err := DecodeCandidateJSON(input)
	if err == nil {
		t.Fatal("expected error for unknown field 'oops'")
	}
}

func TestEncodeEstimateJSON_ProducesValidJSON(t *testing.T) {
	est := ContextEstimate{
		EstimatedTokens: 91000,
		BudgetMin:       64000,
		BudgetMax:       192000,
		Fit:             "within",
		Confidence:      "high",
		Drivers:         []string{"7 referenced files"},
		Recommendation:  "keep",
	}

	data, err := EncodeEstimateJSON(est)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	var decoded ContextEstimate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.EstimatedTokens != est.EstimatedTokens {
		t.Errorf("EstimatedTokens mismatch: got %d, want %d", decoded.EstimatedTokens, est.EstimatedTokens)
	}
	if decoded.Fit != est.Fit {
		t.Errorf("Fit mismatch: got %q, want %q", decoded.Fit, est.Fit)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/contextsize/ -run TestDecodeCandidateJSON -v`
Expected: FAIL with "undefined: DecodeCandidateJSON"

- [ ] **Step 4: Run test to verify all tests pass**

Run: `go test ./internal/contextsize/ -v`
Expected: All tests PASS (including previous tasks)

- [ ] **Step 5: Commit**

```bash
git add internal/contextsize/json.go internal/contextsize/json_test.go
git commit -m "feat: add JSON serialization for ContextCandidate and ContextEstimate"
```

---

### Task 4: Create cmd/contextsize CLI binary

**Files:**
- Create: `cmd/contextsize/main.go`

- [ ] **Step 1: Write the CLI entrypoint**

Create `cmd/contextsize/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/contextsize"
)

func main() {
	candidatePath := flag.String("candidate", "", "Path to ContextCandidate JSON file")
	budgetFlag := flag.String("budget", "64000:192000", "Budget range in tokens (min:max)")
	baseDir := flag.String("base-dir", "", "Root for resolving referenced file paths")
	flag.Parse()

	if *candidatePath == "" {
		fmt.Fprintln(os.Stderr, "Error: --candidate is required")
		os.Exit(2)
	}

	// Parse budget.
	parts := strings.Split(*budgetFlag, ":")
	if len(parts) != 2 {
		fmt.Fprintln(os.Stderr, "Error: --budget must be in format min:max")
		os.Exit(2)
	}

	var minTokens, maxTokens int
	if _, err := fmt.Sscanf(parts[0], "%d", &minTokens); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid budget min: %s\n", parts[0])
		os.Exit(2)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &maxTokens); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid budget max: %s\n", parts[1])
		os.Exit(2)
	}

	budget := contextsize.Budget{
		MinTokens: minTokens,
		MaxTokens: maxTokens,
	}

	// Read candidate JSON.
	candidateData, err := os.ReadFile(*candidatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading candidate file: %v\n", err)
		os.Exit(3)
	}

	candidate, err := contextsize.DecodeCandidateJSON(candidateData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding candidate: %v\n", err)
		os.Exit(2)
	}

	// Run estimation.
	estimator := &contextsize.Estimator{
		BaseDir: *baseDir,
	}

	est, err := estimator.Estimate(candidate, budget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error estimating context: %v\n", err)
		os.Exit(2)
	}

	// Output result.
	output, err := contextsize.EncodeEstimateJSON(est)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding result: %v\n", err)
		os.Exit(3)
	}

	fmt.Print(string(output))
}
```

- [ ] **Step 2: Create a test candidate file**

Create a temporary test file for manual verification:

```bash
cat > /tmp/test-candidate.json << 'EOF'
{
  "id": "T1.1",
  "title": "Add context estimator",
  "description": "Implement deterministic context sizing for issue candidates.",
  "referenced_files": [
    "main.go"
  ],
  "referenced_sections": [],
  "depends_on": [],
  "source": "waveplan"
}
EOF
```

- [ ] **Step 3: Build and test the CLI**

Run: `go build -o /tmp/contextsize ./cmd/contextsize/`
Expected: no output (success)

Run: `/tmp/contextsize --candidate /tmp/test-candidate.json --base-dir /Users/darkbit1001/workspace/waveplan-mcp`
Expected: JSON output with estimated tokens, fit "under", recommendation "merge_candidate" or "keep"

- [ ] **Step 4: Commit**

```bash
git add cmd/contextsize/main.go
git commit -m "feat: add contextsize CLI binary"
```

---

### Task 5: Add Python wrapper to waveplan-cli

**Files:**
- Modify: `waveplan-cli`

- [ ] **Step 1: Add context subcommand parser**

Add to `waveplan-cli` after the `swim` subcommand block (around line 763):

```python
    context_p = sub.add_parser("context", help="Context estimation tools")
    context_sub = context_p.add_subparsers(dest="context_command", required=True)
    
    context_estimate = context_sub.add_parser(
        "estimate",
        help="Estimate context footprint of an issue candidate",
    )
    context_estimate.add_argument("--candidate", required=True, help="Path to ContextCandidate JSON file")
    context_estimate.add_argument("--budget", default="64000:192000", help="Budget range min:max in tokens (default: 64000:192000)")
    context_estimate.add_argument("--base-dir", dest="base_dir", required=False, help="Root for resolving referenced file paths")
```

- [ ] **Step 2: Add handler for context estimate**

Add the handler function before `main()`:

```python
def _handle_context_estimate(args: argparse.Namespace) -> None:
    candidate_path = _abs_user_path(args.candidate)
    if not candidate_path:
        _emit_json({"ok": False, "error": "--candidate is required", "subcommand": "context estimate"}, 2)
    
    # Resolve contextsize binary.
    contextsize_bin = _resolve_local_tool("contextsize")
    if not contextsize_bin:
        # Fall back to go run.
        contextsize_bin = str(_REPO_ROOT / "cmd" / "contextsize")
    
    cmd = [
        "go", "run", contextsize_bin,
        "--candidate", candidate_path,
        "--budget", args.budget,
    ]
    if args.base_dir:
        cmd.extend(["--base-dir", _abs_user_path(args.base_dir)])
    
    proc = subprocess.run(cmd, capture_output=True, text=True, cwd=_REPO_ROOT)
    if proc.returncode != 0:
        _emit_json({
            "ok": False,
            "error": proc.stderr.strip() or "context estimate failed",
            "subcommand": "context estimate",
        }, proc.returncode if proc.returncode in (2, 3) else 3)
    
    print(proc.stdout, end="")
```

- [ ] **Step 3: Wire handler in main()**

Add to the command routing in `main()` (after the `swim` block, before the MCP routing):

```python
    if args.command == "context":
        if args.context_command == "estimate":
            _handle_context_estimate(args)
        else:
            _swim_not_wired(args.context_command)
        return
```

- [ ] **Step 4: Test the Python wrapper**

Run: `python waveplan-cli context estimate --candidate /tmp/test-candidate.json --base-dir /Users/darkbit1001/workspace/waveplan-mcp`
Expected: JSON output with context estimate

- [ ] **Step 5: Commit**

```bash
git add waveplan-cli
git commit -m "feat: add waveplan-cli context estimate wrapper"
```

---

### Task 6: Add Makefile target for contextsize

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add contextsize build target**

Add to the Makefile (after existing swim targets):

```makefile
contextsize:
	go build -o $@ ./cmd/contextsize
```

- [ ] **Step 2: Test the build**

Run: `make contextsize`
Expected: binary created at `./contextsize`

Run: `./contextsize --help`
Expected: usage output showing --candidate, --budget, --base-dir flags

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add contextsize Makefile target"
```

---

### Task 7: Final verification and integration test

**Files:**
- No new files

- [ ] **Step 1: Run all tests**

Run: `go test ./internal/contextsize/ -v -race`
Expected: All tests PASS with race detector

- [ ] **Step 2: End-to-end test with real candidate**

Create a realistic candidate referencing actual repo files:

```bash
cat > /tmp/e2e-candidate.json << 'EOF'
{
  "id": "T2.1",
  "title": "Add context estimator to waveplan",
  "description": "Implement deterministic context sizing for issue candidates using byte-count heuristics.",
  "referenced_files": [
    "main.go",
    "internal/contextsize/types.go",
    "internal/contextsize/estimate.go"
  ],
  "referenced_sections": [
    {
      "path": "docs/superpowers/specs/2026-05-13-context-sizer-design.md",
      "heading": "Estimation Algorithm"
    }
  ],
  "depends_on": ["T1.1"],
  "source": "waveplan"
}
EOF
```

Run: `python waveplan-cli context estimate --candidate /tmp/e2e-candidate.json --base-dir /Users/darkbit1001/workspace/waveplan-mcp`
Expected: JSON output with estimated tokens > 0, fit depends on total size, recommendation "keep" or "split"

- [ ] **Step 3: Verify output contract**

Check that output contains all required fields:
- `estimated_tokens` (int)
- `budget_min` (int)
- `budget_max` (int)
- `fit` (string: "within", "over", or "under")
- `confidence` (string: "high", "medium", or "low")
- `drivers` (array of strings)
- `recommendation` (string: "keep", "split", or "merge_candidate")
- `missing_files` (array of strings)
- `missing_sections` (array of objects with path/heading)
- `unknown_files` (array of strings)
- `split_candidates` (array of strings)
- `merge_candidates` (array of strings)

- [ ] **Step 4: Commit**

```bash
git add .
git commit -m "test: add end-to-end verification for context sizer"
```

---

## Self-Review

**Spec coverage:**
- Task 1: types.go ✓ (ContextCandidate, Budget, ContextEstimate, SectionRef)
- Task 2: estimate.go ✓ (Estimator, Estimate(), token estimation, confidence, fit, recommendation)
- Task 3: json.go ✓ (DecodeCandidateJSON, EncodeEstimateJSON)
- Task 4: cmd/contextsize/main.go ✓ (CLI binary with --candidate, --budget, --base-dir)
- Task 5: waveplan-cli wrapper ✓ (context estimate subcommand)
- Task 6: Makefile target ✓
- Task 7: E2E verification ✓

**Placeholder scan:** No "TBD", "TODO", or incomplete sections. All code is complete.

**Type consistency:** All type names match the spec (ContextCandidate, Budget, ContextEstimate, SectionRef). Method signatures match (Estimate returns (ContextEstimate, error)).

**Gaps:** None identified. The plan covers all v1 requirements from the spec.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-13-context-sizer.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?