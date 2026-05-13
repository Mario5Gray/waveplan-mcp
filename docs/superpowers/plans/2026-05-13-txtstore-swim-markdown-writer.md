# txtstore SWIM Markdown Writer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a deterministic `txtstore` writer that emits the strict SWIM markdown plan format in one pass, without changing existing `txtstore_append` or `txtstore_edit` behavior.

**Architecture:** Extend `internal/txtstore` with a typed `SwimPlanDoc` model plus validation and render functions, then expose that path through a new MCP tool and a matching CLI command. The SWIM writer overwrites the full target file atomically; the section/TOC writer remains untouched.

**Tech Stack:** Go 1.26, standard library, existing `internal/txtstore` package, existing `mcp-go` integration in `cmd/txtstore-mcp`.

---

### Task 1: Add failing SWIM markdown model tests

**Files:**
- Create: `internal/txtstore/swim_markdown_test.go`
- Test: `internal/txtstore/swim_markdown_test.go`

- [ ] **Step 1: Write the failing validation and render tests**

Create `internal/txtstore/swim_markdown_test.go`:

```go
package txtstore

import (
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```bash
go test ./internal/txtstore -run 'TestValidateSwimPlan|TestRenderSwimPlan'
```

Expected: FAIL with undefined `SwimPlanDoc`, `ValidateSwimPlan`, and `RenderSwimPlan`.

- [ ] **Step 3: Commit the failing tests**

```bash
git add internal/txtstore/swim_markdown_test.go
git commit -m "test: add failing txtstore swim markdown tests"
```

### Task 2: Implement SWIM model validation and renderer

**Files:**
- Create: `internal/txtstore/swim_markdown.go`
- Modify: `internal/txtstore/swim_markdown_test.go`
- Test: `internal/txtstore/swim_markdown_test.go`

- [ ] **Step 1: Add the typed model and validation helpers**

Create `internal/txtstore/swim_markdown.go`:

```go
package txtstore

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type SwimPlanDoc struct {
	Title    string
	Meta     SwimMeta
	Plan     SwimPlan
	DocIndex []SwimDocRef
	FPIndex  []SwimFPRef
	Tasks    []SwimTask
	Units    []SwimUnit
}

type SwimMeta struct {
	SchemaVersion  int
	GeneratedOn    string
	PlanVersion    int
	PlanGeneration string
	TitleOverride  string
}

type SwimPlan struct {
	PlanID      string
	PlanTitle   string
	PlanDocPath string
	SpecDocPath string
}

type SwimDocRef struct {
	Ref  string
	Path string
	Line int
	Kind string
}

type SwimFPRef struct {
	FPRef string
	FPID  string
}

type SwimTask struct {
	TaskID   string
	Title    string
	PlanLine int
	DocRefs  []string
	Files    []string
}

type SwimUnit struct {
	UnitID    string
	TaskID    string
	Title     string
	Kind      string
	Wave      int
	PlanLine  int
	DependsOn []string
	FPRefs    []string
	DocRefs   []string
}

func ValidateSwimPlan(doc SwimPlanDoc) error {
	// Implement required top-level field checks, duplicate detection,
	// reference validation, enum checks, and unit dependency cycle detection.
	return nil
}
```

- [ ] **Step 2: Add deterministic render functions**

Extend `internal/txtstore/swim_markdown.go`:

```go
func RenderSwimPlan(doc SwimPlanDoc) (string, error) {
	if err := ValidateSwimPlan(doc); err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("# " + trim(doc.Title) + "\n\n")
	sb.WriteString(renderMeta(doc.Meta))
	sb.WriteString("\n\n")
	sb.WriteString(renderPlan(doc.Plan))
	sb.WriteString("\n\n")
	sb.WriteString(renderDocIndex(doc.DocIndex))
	sb.WriteString("\n\n")
	sb.WriteString(renderFPIndex(doc.FPIndex))
	sb.WriteString("\n\n")
	sb.WriteString(renderTasks(doc.Tasks))
	sb.WriteString("\n\n")
	sb.WriteString(renderUnits(doc.Units))
	sb.WriteString("\n")
	return sb.String(), nil
}
```

- [ ] **Step 3: Run the package tests and make them pass**

Run:

```bash
go test ./internal/txtstore
```

Expected: PASS

- [ ] **Step 4: Commit the core implementation**

```bash
git add internal/txtstore/swim_markdown.go internal/txtstore/swim_markdown_test.go
git commit -m "feat: add deterministic txtstore swim markdown renderer"
```

### Task 3: Add atomic file writing for SWIM markdown

**Files:**
- Modify: `internal/txtstore/filestore.go`
- Modify: `internal/txtstore/swim_markdown_test.go`
- Test: `internal/txtstore/swim_markdown_test.go`

- [ ] **Step 1: Write the failing FileStore integration test**

Append to `internal/txtstore/swim_markdown_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/txtstore -run TestFileStoreWriteSwimPlanOverwritesCanonicalDocument
```

Expected: FAIL with undefined `WriteSwimPlan`.

- [ ] **Step 3: Implement `WriteSwimPlan` without changing append/edit semantics**

Update [filestore.go](/Users/darkbit1001/workspace/waveplan-mcp/internal/txtstore/filestore.go:1):

```go
func (fs *FileStore) WriteSwimPlan(doc SwimPlanDoc) error {
	content, err := RenderSwimPlan(doc)
	if err != nil {
		return err
	}
	if _, err := fs.readOrCreate(); err != nil {
		return err
	}
	return fs.writeAtomically(content)
}
```

- [ ] **Step 4: Run the package tests**

Run:

```bash
go test ./internal/txtstore
```

Expected: PASS

- [ ] **Step 5: Commit the FileStore integration**

```bash
git add internal/txtstore/filestore.go internal/txtstore/swim_markdown_test.go
git commit -m "feat: add txtstore swim markdown file writer"
```

### Task 4: Expose the new writer through txtstore-mcp

**Files:**
- Modify: `cmd/txtstore-mcp/main.go`
- Create: `cmd/txtstore-mcp/main_test.go`
- Test: `cmd/txtstore-mcp/main_test.go`

- [ ] **Step 1: Write a failing MCP handler test**

Create `cmd/txtstore-mcp/main_test.go`:

```go
package main

import "testing"

func TestBuildSwimPlanDocRequiresTaskAndUnitPayloads(t *testing.T) {
	args := map[string]any{
		"filepath": "tmp/plan.md",
		"title":    "Swim Plan Source",
		"meta":     map[string]any{"schema_version": 1, "generated_on": "2026-05-13", "plan_version": 1, "plan_generation": "2026-05-13T00:00:00Z"},
		"plan":     map[string]any{"plan_id": "p", "plan_title": "Plan", "plan_doc_path": "plan.md", "spec_doc_path": "spec.md"},
		"doc_index": []any{},
		"fp_index":  []any{},
		"tasks":     []any{},
		"units":     []any{},
	}
	if _, err := buildSwimPlanDoc(args); err != nil {
		t.Fatalf("buildSwimPlanDoc() error = %v", err)
	}
}
```

- [ ] **Step 2: Run the MCP test to verify it fails**

Run:

```bash
go test ./cmd/txtstore-mcp -run TestBuildSwimPlanDocRequiresTaskAndUnitPayloads
```

Expected: FAIL with undefined `buildSwimPlanDoc`.

- [ ] **Step 3: Add the MCP tool and payload conversion helper**

Update `cmd/txtstore-mcp/main.go`:

```go
swimTool := mcp.NewTool("txtstore_write_swim_plan",
	mcp.WithDescription("Write a deterministic SWIM markdown plan document."),
	mcp.WithString("filepath", mcp.Required(), mcp.Description("Target markdown file path")),
	mcp.WithString("title", mcp.Required(), mcp.Description("Top-level # heading")),
	mcp.WithObject("meta", mcp.Required()),
	mcp.WithObject("plan", mcp.Required()),
	mcp.WithArray("doc_index", mcp.Required()),
	mcp.WithArray("fp_index", mcp.Required()),
	mcp.WithArray("tasks", mcp.Required()),
	mcp.WithArray("units", mcp.Required()),
)
```

- [ ] **Step 4: Run the MCP tests**

Run:

```bash
go test ./cmd/txtstore-mcp
```

Expected: PASS

- [ ] **Step 5: Commit the MCP tool**

```bash
git add cmd/txtstore-mcp/main.go cmd/txtstore-mcp/main_test.go
git commit -m "feat: add txtstore_write_swim_plan mcp tool"
```

### Task 5: Expose the new writer through the txtstore CLI proxy

**Files:**
- Modify: `cmd/txtstore/main.go`
- Create: `cmd/txtstore/main_test.go`
- Test: `cmd/txtstore/main_test.go`

- [ ] **Step 1: Write a failing CLI parser test**

Create `cmd/txtstore/main_test.go`:

```go
package main

import "testing"

func TestToolNameForWriteSwimPlan(t *testing.T) {
	got := toolNameForCommand("write-swim-plan")
	if got != "txtstore_write_swim_plan" {
		t.Fatalf("toolNameForCommand() = %q, want %q", got, "txtstore_write_swim_plan")
	}
}
```

- [ ] **Step 2: Run the CLI test to verify it fails**

Run:

```bash
go test ./cmd/txtstore -run TestToolNameForWriteSwimPlan
```

Expected: FAIL with undefined `toolNameForCommand`.

- [ ] **Step 3: Add the new command without breaking append/edit**

Update `cmd/txtstore/main.go`:

```go
func toolNameForCommand(command string) string {
	switch command {
	case "append", "edit":
		return "txtstore_" + command
	case "write-swim-plan":
		return "txtstore_write_swim_plan"
	default:
		return ""
	}
}
```

Support:

```bash
txtstore write-swim-plan <filepath> <json>
```

The CLI should parse the second argument as either:
- raw JSON payload, or
- a file path whose contents are JSON

- [ ] **Step 4: Run the CLI tests and targeted package tests**

Run:

```bash
go test ./cmd/txtstore ./cmd/txtstore-mcp ./internal/txtstore
```

Expected: PASS

- [ ] **Step 5: Commit the CLI command**

```bash
git add cmd/txtstore/main.go cmd/txtstore/main_test.go
git commit -m "feat: add txtstore write-swim-plan command"
```

### Task 6: Final verification

**Files:**
- Modify: none
- Test: existing test suite

- [ ] **Step 1: Run the focused verification commands**

Run:

```bash
go test ./internal/txtstore ./cmd/txtstore ./cmd/txtstore-mcp
```

Expected: PASS

- [ ] **Step 2: Run the broader project verification**

Run:

```bash
go test ./...
```

Expected: PASS

- [ ] **Step 3: Build the txtstore binaries**

Run:

```bash
make build-mcp
```

Expected: PASS, producing `txtstore` and `txtstore-mcp`.

- [ ] **Step 4: Commit the verified end state**

```bash
git status --short
git commit -m "feat: add deterministic txtstore swim markdown writer"
```
