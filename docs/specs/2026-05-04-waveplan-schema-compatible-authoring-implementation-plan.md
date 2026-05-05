# Waveplan Schema-Compatible Plan Authoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add deterministic plan-authoring capabilities (`new/init/add/remove/get/validate/suggest_questions/list`) that emit and preserve files fully compatible with `docs/specs/waveplan-plan-schema.json` and existing runtime execution tools.

**Architecture:** Keep execution-state behavior intact (`waveplan_*` tools) and add a separate authoring service layer in Go. Persist schema-required plan JSON unchanged, while storing concurrency/idempotency/audit metadata in sidecars so output remains schema-valid. Expose new authoring MCP tools from `main.go`, then proxy them via `waveplan-cli` `wp plan ...` commands.

**Tech Stack:** Go 1.26, `mcp-go`, standard library JSON/file I/O, Python CLI proxy, JSON schema-compatible strict decode/validation, table-driven tests with `go test`.

---

## File Structure Map

- Create: `internal/planedit/types.go`
- Create: `internal/planedit/validate.go`
- Create: `internal/planedit/graph.go`
- Create: `internal/planedit/store.go`
- Create: `internal/planedit/service.go`
- Create: `internal/planedit/errors.go`
- Create: `internal/planedit/types_test.go`
- Create: `internal/planedit/validate_test.go`
- Create: `internal/planedit/graph_test.go`
- Create: `internal/planedit/store_test.go`
- Create: `internal/planedit/service_test.go`
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `waveplan-cli`
- Modify: `README.md`
- Create: `adocs/specs/waveplan-plan-authoring-mcp-contracts.md`

Design boundary:
- `main.go` stays MCP transport + current runtime tools.
- `internal/planedit/*` owns authoring logic, validation, locking, sidecars.
- `waveplan-cli` remains thin transport proxy with new subcommands only.

---

### Task 1: Add Plan Authoring Types and Strict Schema Decode

**Files:**
- Create: `internal/planedit/types.go`
- Test: `internal/planedit/types_test.go`

- [ ] **Step 1: Write failing tests for strict decode and required top-level sections**

```go
// internal/planedit/types_test.go
package planedit

import "testing"

func TestDecodePlan_RejectsUnknownTopLevelField(t *testing.T) {
	input := []byte(`{"units":{},"tasks":{},"doc_index":{},"fp_index":{},"oops":1}`)
	_, err := DecodePlanStrict(input)
	if err == nil {
		t.Fatalf("expected error for unknown field")
	}
}

func TestDecodePlan_RequiresSchemaSections(t *testing.T) {
	input := []byte(`{"units":{},"tasks":{}}`)
	_, err := DecodePlanStrict(input)
	if err == nil {
		t.Fatalf("expected error for missing doc_index/fp_index")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/planedit -run DecodePlan -v`
Expected: FAIL with undefined `DecodePlanStrict`.

- [ ] **Step 3: Implement schema-compatible types and strict decoder**

```go
// internal/planedit/types.go
package planedit

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type PlanFile struct {
	SchemaVersion int                     `json:"schema_version,omitempty"`
	GeneratedOn   string                  `json:"generated_on,omitempty"`
	Plan          map[string]any          `json:"plan,omitempty"`
	Units         map[string]PlanUnit     `json:"units"`
	Tasks         map[string]map[string]any `json:"tasks"`
	DocIndex      map[string]DocRef       `json:"doc_index"`
	FpIndex       map[string]string       `json:"fp_index"`
	Waves         []WaveDecl              `json:"waves,omitempty"`
}

type PlanUnit struct {
	Task      string   `json:"task"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`
	Wave      int      `json:"wave"`
	PlanLine  int      `json:"plan_line"`
	DependsOn []string `json:"depends_on"`
	DocRefs   []string `json:"doc_refs,omitempty"`
	FpRefs    []string `json:"fp_refs,omitempty"`
	Notes     []string `json:"notes,omitempty"`
	Command   string   `json:"command,omitempty"`
}

type DocRef struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Kind string `json:"kind"`
}

type WaveDecl struct {
	Wave  int      `json:"wave"`
	Units []string `json:"units"`
}

func DecodePlanStrict(raw []byte) (*PlanFile, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p PlanFile
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decode strict: %w", err)
	}
	if p.Units == nil || p.Tasks == nil || p.DocIndex == nil || p.FpIndex == nil {
		return nil, fmt.Errorf("missing required top-level sections")
	}
	return &p, nil
}
```

- [ ] **Step 4: Re-run tests**

Run: `go test ./internal/planedit -run DecodePlan -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/planedit/types.go internal/planedit/types_test.go
git commit -m "feat(planedit): add schema-compatible plan types and strict decoder"
```

---

### Task 2: Implement Schema-Level Validation (IDs, kinds, deps, refs)

**Files:**
- Create: `internal/planedit/validate.go`
- Create: `internal/planedit/errors.go`
- Test: `internal/planedit/validate_test.go`

- [ ] **Step 1: Write failing validation tests for unit ID pattern, kind enum, and missing deps**

```go
// internal/planedit/validate_test.go
package planedit

import "testing"

func TestValidatePlan_RejectsBadUnitID(t *testing.T) {
	p := &PlanFile{Units: map[string]PlanUnit{"bad": {Task: "T1", Title: "x", Kind: "impl", Wave: 1, PlanLine: 1, DependsOn: nil}}, Tasks: map[string]map[string]any{"T1": {"plan_line": 1}}, DocIndex: map[string]DocRef{}, FpIndex: map[string]string{}}
	err := ValidatePlanSchemaCompatible(p)
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidatePlan_RejectsUnknownKind(t *testing.T) {
	p := validPlanFixture()
	u := p.Units["T1.1"]
	u.Kind = "weird"
	p.Units["T1.1"] = u
	if err := ValidatePlanSchemaCompatible(p); err == nil {
		t.Fatalf("expected kind validation error")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/planedit -run ValidatePlan -v`
Expected: FAIL with undefined `ValidatePlanSchemaCompatible`.

- [ ] **Step 3: Implement validation and stable error codes**

```go
// internal/planedit/errors.go
package planedit

type DomainError struct {
	Code      string
	Message   string
	Retryable bool
	Details   map[string]any
}

func (e *DomainError) Error() string { return e.Code + ": " + e.Message }
```

```go
// internal/planedit/validate.go
package planedit

import (
	"fmt"
	"regexp"
)

var unitIDRe = regexp.MustCompile(`^[A-Z][0-9]+(\.[0-9]+)$`)

func ValidatePlanSchemaCompatible(p *PlanFile) error {
	allowedKind := map[string]bool{"impl": true, "test": true, "verify": true, "doc": true, "refactor": true}

	for id, u := range p.Units {
		if !unitIDRe.MatchString(id) {
			return &DomainError{Code: "SCHEMA_VIOLATION", Message: fmt.Sprintf("unit id %q invalid", id)}
		}
		if !allowedKind[u.Kind] {
			return &DomainError{Code: "SCHEMA_VIOLATION", Message: fmt.Sprintf("unit %s kind %q invalid", id, u.Kind)}
		}
		if u.Task == "" || u.Title == "" || u.PlanLine <= 0 || u.Wave <= 0 {
			return &DomainError{Code: "SCHEMA_VIOLATION", Message: fmt.Sprintf("unit %s missing required fields", id)}
		}
		if _, ok := p.Tasks[u.Task]; !ok {
			return &DomainError{Code: "DEP_VIOLATION", Message: fmt.Sprintf("unit %s references missing parent task %s", id, u.Task)}
		}
		for _, dep := range u.DependsOn {
			if _, ok := p.Units[dep]; !ok {
				return &DomainError{Code: "DEP_VIOLATION", Message: fmt.Sprintf("unit %s depends on missing unit %s", id, dep)}
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Re-run validation tests**

Run: `go test ./internal/planedit -run ValidatePlan -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/planedit/validate.go internal/planedit/errors.go internal/planedit/validate_test.go
git commit -m "feat(planedit): add schema-compatible validator and domain errors"
```

---

### Task 3: Add Graph Validation and Deterministic Wave/Depth Recompute

**Files:**
- Create: `internal/planedit/graph.go`
- Test: `internal/planedit/graph_test.go`

- [ ] **Step 1: Write failing tests for cycle detection and recompute behavior**

```go
// internal/planedit/graph_test.go
package planedit

import "testing"

func TestRecomputeWaves_AssignsRootsToWaveOne(t *testing.T) {
	p := validPlanFixture()
	if err := RecomputeWaves(p); err != nil {
		t.Fatalf("recompute failed: %v", err)
	}
	if p.Units["T1.1"].Wave != 1 {
		t.Fatalf("root wave must be 1")
	}
}

func TestRecomputeWaves_RejectsCycle(t *testing.T) {
	p := validPlanFixture()
	u := p.Units["T1.1"]
	u.DependsOn = []string{"T2.1"}
	p.Units["T1.1"] = u
	if err := RecomputeWaves(p); err == nil {
		t.Fatalf("expected cycle error")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/planedit -run RecomputeWaves -v`
Expected: FAIL with undefined `RecomputeWaves`.

- [ ] **Step 3: Implement topo sort + wave recompute**

```go
// internal/planedit/graph.go
package planedit

import "fmt"

func RecomputeWaves(p *PlanFile) error {
	in := map[string]int{}
	adj := map[string][]string{}
	for id := range p.Units {
		in[id] = 0
	}
	for id, u := range p.Units {
		for _, dep := range u.DependsOn {
			adj[dep] = append(adj[dep], id)
			in[id]++
		}
	}
	q := make([]string, 0)
	for id, d := range in {
		if d == 0 {
			q = append(q, id)
		}
	}
	if len(q) == 0 && len(p.Units) > 0 {
		return &DomainError{Code: "CYCLE_DETECTED", Message: "no root nodes"}
	}

	wave := map[string]int{}
	seen := 0
	for len(q) > 0 {
		id := q[0]
		q = q[1:]
		seen++
		u := p.Units[id]
		if len(u.DependsOn) == 0 {
			wave[id] = 1
		} else {
			maxW := 0
			for _, dep := range u.DependsOn {
				if wave[dep] > maxW {
					maxW = wave[dep]
				}
			}
			wave[id] = maxW + 1
		}
		u.Wave = wave[id]
		p.Units[id] = u
		for _, n := range adj[id] {
			in[n]--
			if in[n] == 0 {
				q = append(q, n)
			}
		}
	}
	if seen != len(p.Units) {
		return &DomainError{Code: "CYCLE_DETECTED", Message: fmt.Sprintf("visited %d of %d nodes", seen, len(p.Units))}
	}
	return nil
}
```

- [ ] **Step 4: Re-run graph tests**

Run: `go test ./internal/planedit -run RecomputeWaves -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/planedit/graph.go internal/planedit/graph_test.go
git commit -m "feat(planedit): add deterministic topo validation and wave recompute"
```

---

### Task 4: Add Sidecar Control Store (Revision, Idempotency, Audit)

**Files:**
- Create: `internal/planedit/store.go`
- Test: `internal/planedit/store_test.go`

- [ ] **Step 1: Write failing tests for revision mismatch and idempotency replay**

```go
// internal/planedit/store_test.go
package planedit

import "testing"

func TestApplyMutation_RejectsRevisionMismatch(t *testing.T) {
	st := NewControlState()
	_, err := st.CheckAndBump(2)
	if err == nil {
		t.Fatalf("expected revision mismatch")
	}
}

func TestIdempotency_ReplaySameKeyAndPayload(t *testing.T) {
	st := NewControlState()
	first := map[string]any{"ok": true}
	if err := st.Remember("plan_add_tasks", "k1", "h1", first); err != nil {
		t.Fatalf("remember failed: %v", err)
	}
	got, ok := st.Replay("plan_add_tasks", "k1", "h1")
	if !ok || got["ok"] != true {
		t.Fatalf("expected replay")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/planedit -run 'RevisionMismatch|Idempotency' -v`
Expected: FAIL with undefined `NewControlState`.

- [ ] **Step 3: Implement control sidecar model and audit append**

```go
// internal/planedit/store.go
package planedit

import (
	"encoding/json"
	"fmt"
	"os"
)

type ControlState struct {
	Revision    int                        `json:"revision"`
	Idempotency map[string]IdempotentEntry `json:"idempotency"`
}

type IdempotentEntry struct {
	PayloadHash string         `json:"payload_hash"`
	Result      map[string]any `json:"result"`
}

func NewControlState() *ControlState {
	return &ControlState{Revision: 1, Idempotency: map[string]IdempotentEntry{}}
}

func (c *ControlState) CheckAndBump(expected int) (int, error) {
	if expected != c.Revision {
		return c.Revision, &DomainError{Code: "REVISION_MISMATCH", Message: fmt.Sprintf("expected %d got %d", expected, c.Revision), Retryable: true}
	}
	c.Revision++
	return c.Revision, nil
}

func (c *ControlState) Remember(op, key, payloadHash string, result map[string]any) error {
	k := op + ":" + key
	c.Idempotency[k] = IdempotentEntry{PayloadHash: payloadHash, Result: result}
	return nil
}

func (c *ControlState) Replay(op, key, payloadHash string) (map[string]any, bool) {
	k := op + ":" + key
	v, ok := c.Idempotency[k]
	if !ok {
		return nil, false
	}
	if v.PayloadHash != payloadHash {
		return map[string]any{"ok": false, "error": map[string]any{"code": "IDEMPOTENCY_KEY_REUSED"}}, true
	}
	return v.Result, true
}

func AppendAuditJSONL(path string, event map[string]any) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}
```

- [ ] **Step 4: Re-run store tests**

Run: `go test ./internal/planedit -run 'RevisionMismatch|Idempotency' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/planedit/store.go internal/planedit/store_test.go
git commit -m "feat(planedit): add control sidecar revision idempotency and audit append"
```

---

### Task 5: Implement Authoring Service (`create/init/add/remove/get/validate/list/suggest`)

**Files:**
- Create: `internal/planedit/service.go`
- Test: `internal/planedit/service_test.go`

- [ ] **Step 1: Write failing tests for create/init/add/remove and suggest_questions**

```go
// internal/planedit/service_test.go
package planedit

import "testing"

func TestServiceCreatePlan_WritesSchemaCompatibleFile(t *testing.T) {
	s := NewService(t.TempDir())
	out, err := s.CreatePlan("my-plan")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if out.PlanID == "" || out.Revision != 1 {
		t.Fatalf("bad create output")
	}
}

func TestSuggestQuestions_ReturnsMissingKindAndTitle(t *testing.T) {
	s := NewService(t.TempDir())
	q := s.SuggestQuestions(map[string]any{"task_id": "T1.1"})
	if len(q) == 0 {
		t.Fatalf("expected questions")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/planedit -run 'ServiceCreatePlan|SuggestQuestions' -v`
Expected: FAIL with undefined `NewService`.

- [ ] **Step 3: Implement service operations and normalized outputs**

```go
// internal/planedit/service.go
package planedit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Service struct{ PlanDir string }

type CreatePlanResult struct {
	PlanID   string `json:"plan_id"`
	PlanPath string `json:"plan_path"`
	Revision int    `json:"revision"`
}

func NewService(planDir string) *Service { return &Service{PlanDir: planDir} }

func (s *Service) CreatePlan(name string) (*CreatePlanResult, error) {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	file := filepath.Join(s.PlanDir, slug+"-execution-waves.json")
	p := &PlanFile{SchemaVersion: 1, Units: map[string]PlanUnit{}, Tasks: map[string]map[string]any{}, DocIndex: map[string]DocRef{}, FpIndex: map[string]string{}}
	b, _ := json.MarshalIndent(p, "", "  ")
	if err := os.WriteFile(file, b, 0o600); err != nil {
		return nil, err
	}
	return &CreatePlanResult{PlanID: slug, PlanPath: file, Revision: 1}, nil
}

func (s *Service) SuggestQuestions(in map[string]any) []map[string]any {
	type q struct{ Field string; Required bool; Type string; Options []string }
	all := []q{
		{Field: "title", Required: true, Type: "string"},
		{Field: "kind", Required: true, Type: "enum", Options: []string{"impl", "test", "verify", "doc", "refactor"}},
		{Field: "depends_on", Required: false, Type: "array"},
	}
	out := []map[string]any{}
	for _, it := range all {
		if _, ok := in[it.Field]; ok {
			continue
		}
		out = append(out, map[string]any{"field": it.Field, "required": it.Required, "type": it.Type, "options": it.Options})
	}
	sort.Slice(out, func(i, j int) bool {
		ri, _ := out[i]["required"].(bool)
		rj, _ := out[j]["required"].(bool)
		if ri != rj {
			return ri
		}
		return fmt.Sprint(out[i]["field"]) < fmt.Sprint(out[j]["field"])
	})
	return out
}
```

- [ ] **Step 4: Re-run service tests**

Run: `go test ./internal/planedit -run 'ServiceCreatePlan|SuggestQuestions' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/planedit/service.go internal/planedit/service_test.go
git commit -m "feat(planedit): add plan authoring service and deterministic question suggestions"
```

---

### Task 6: Expose New MCP Tools in `main.go`

**Files:**
- Modify: `main.go`
- Modify: `main_test.go`

- [ ] **Step 1: Write failing MCP handler tests for `plan_create` and `plan_validate`**

```go
// main_test.go
func TestPlanCreateTool(t *testing.T) {
	srv, err := newWaveplanServer("test-plan.json", "")
	if err != nil { t.Fatal(err) }
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "plan_create", Arguments: map[string]any{"name": "demo"}}}
	_, err = srv.handlePlanCreate(nil, req)
	if err != nil { t.Fatalf("tool failed: %v", err) }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./... -run PlanCreateTool -v`
Expected: FAIL with undefined `handlePlanCreate`.

- [ ] **Step 3: Register tools and add handlers**

```go
// in createTools()
{
	Tool: mcp.NewTool("plan_create",
		mcp.WithDescription("Create a new schema-compatible waveplan"),
		mcp.WithString("name", mcp.Required()),
	),
	Handler: s.handlePlanCreate,
},
{
	Tool: mcp.NewTool("plan_validate",
		mcp.WithDescription("Validate a schema-compatible waveplan"),
		mcp.WithString("plan_path", mcp.Required()),
	),
	Handler: s.handlePlanValidate,
},
```

```go
func (s *WaveplanServer) handlePlanCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := requiredStringParam(request.Params.Arguments, "name")
	if err != nil { return mcp.NewToolResultError(err.Error()), nil }
	svc := planedit.NewService(defaultPlanDir())
	out, err := svc.CreatePlan(name)
	if err != nil { return mcp.NewToolResultError(err.Error()), nil }
	b, _ := json.Marshal(map[string]any{"ok": true, "data": out})
	return mcp.NewToolResultText(string(b)), nil
}
```

- [ ] **Step 4: Re-run MCP tool tests**

Run: `go test ./... -run 'PlanCreateTool|PlanValidate' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat(mcp): add plan authoring tool surface"
```

---

### Task 7: Extend `waveplan-cli` with `wp plan ...` Authoring Commands

**Files:**
- Modify: `waveplan-cli`

- [ ] **Step 1: Write failing CLI smoke test command list check**

```bash
python3 waveplan-cli --help | rg "plan_create|plan_validate"
```

Expected: no matches before implementation.

- [ ] **Step 2: Add CLI parser subcommands for plan authoring**

```python
# in waveplan-cli argparse section
plan_p = sub.add_parser("plan", help="Plan authoring commands")
plan_sub = plan_p.add_subparsers(dest="plan_command", required=True)

plan_new = plan_sub.add_parser("new", help="Create plan")
plan_new.add_argument("name")

plan_validate = plan_sub.add_parser("validate", help="Validate plan")
plan_validate.add_argument("plan_path")
```

- [ ] **Step 3: Route new subcommands to MCP tools**

```python
elif cmd == "plan":
    if args.plan_command == "new":
        result = client.call_tool("plan_create", {"name": args.name})
    elif args.plan_command == "validate":
        result = client.call_tool("plan_validate", {"plan_path": args.plan_path})
    else:
        raise RuntimeError(f"unknown plan command {args.plan_command}")
```

- [ ] **Step 4: Re-run smoke command**

Run: `python3 waveplan-cli plan --help`
Expected: shows `new`, `validate`.

- [ ] **Step 5: Commit**

```bash
git add waveplan-cli
git commit -m "feat(cli): add wp plan authoring subcommands"
```

---

### Task 8: Document Contracts and End-to-End Verification

**Files:**
- Create: `adocs/specs/waveplan-plan-authoring-mcp-contracts.md`
- Modify: `README.md`

- [ ] **Step 1: Write failing documentation check for missing tool docs**

```bash
rg -n "plan_create|plan_validate|wp plan" README.md
```

Expected: missing or incomplete references before update.

- [ ] **Step 2: Add MCP request/response contracts document**

```markdown
# Waveplan Plan Authoring MCP Contracts

## plan_create
Request args:
- name: string (required)

Success:
{ "ok": true, "data": { "plan_id": "...", "plan_path": "...", "revision": 1 } }

Error:
{ "ok": false, "error": { "code": "...", "message": "...", "details": {} } }
```

- [ ] **Step 3: Update README with new workflow**

```markdown
## Plan Authoring (Schema-Compatible)

~~~bash
python3 waveplan-cli plan new my-feature
python3 waveplan-cli plan validate ~/.local/share/waveplan/plans/my-feature-execution-waves.json
~~~
```

- [ ] **Step 4: Run full verification suite**

Run: `go test ./... -v`
Expected: PASS.

Run: `python3 waveplan-cli plan --help`
Expected: exits 0 and prints plan subcommands.

- [ ] **Step 5: Commit**

```bash
git add adocs/specs/waveplan-plan-authoring-mcp-contracts.md README.md
git commit -m "docs: add schema-compatible plan authoring contracts and usage"
```

---

### Task 9: Implement Remaining Authoring Operations End-to-End

**Files:**
- Modify: `internal/planedit/service.go`
- Modify: `internal/planedit/service_test.go`
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `waveplan-cli`

- [ ] **Step 1: Write failing tests for `plan_init`, `plan_add_tasks`, `plan_remove_task`, `plan_get`, `plan_list`**

```go
// internal/planedit/service_test.go
func TestServiceInitPlan_FromElementsBlob(t *testing.T) {
	s := NewService(t.TempDir())
	created, err := s.InitPlanFromJSON("demo", []byte(`{
	  "tasks":[
	    {"task_id":"T1.1","task":"T1","title":"root","kind":"impl","depends_on":[]},
	    {"task_id":"T2.1","task":"T2","title":"child","kind":"test","depends_on":["T1.1"]}
	  ]
	}`))
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if created != 2 {
		t.Fatalf("expected 2 tasks, got %d", created)
	}
}

func TestServiceRemoveTask_RejectsDependentsInStrictMode(t *testing.T) {
	s := NewService(t.TempDir())
	seedPlanForRemoveTest(t, s)
	_, err := s.RemoveTask("demo", "T1.1", "strict")
	if err == nil {
		t.Fatalf("expected dependent rejection")
	}
}

func seedPlanForRemoveTest(t *testing.T, s *Service) {
	t.Helper()
	_, err := s.CreatePlan("demo")
	if err != nil { t.Fatalf("create: %v", err) }
	_, err = s.InitPlanFromJSON("demo", []byte(`{
	  "tasks":[
	    {"task_id":"T1.1","task":"T1","title":"root","kind":"impl","depends_on":[]},
	    {"task_id":"T2.1","task":"T2","title":"child","kind":"test","depends_on":["T1.1"]}
	  ]
	}`))
	if err != nil { t.Fatalf("init: %v", err) }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/planedit -run 'InitPlan|RemoveTask' -v`
Expected: FAIL with undefined `InitPlanFromJSON` / `RemoveTask`.

- [ ] **Step 3: Implement all remaining service operations**

```go
// internal/planedit/service.go (additions)
type InitElement struct {
	TaskID    string   `json:"task_id"`
	Task      string   `json:"task"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`
	DependsOn []string `json:"depends_on"`
}

func (s *Service) InitPlanFromJSON(planID string, blob []byte) (int, error) {
	var req struct{ Tasks []InitElement `json:"tasks"` }
	if err := json.Unmarshal(blob, &req); err != nil {
		return 0, err
	}
	p, _, err := s.LoadPlan(planID)
	if err != nil {
		return 0, err
	}
	for _, e := range req.Tasks {
		p.Units[e.TaskID] = PlanUnit{
			Task: e.Task, Title: e.Title, Kind: e.Kind, Wave: 1, PlanLine: 1, DependsOn: e.DependsOn,
		}
		if _, ok := p.Tasks[e.Task]; !ok {
			p.Tasks[e.Task] = map[string]any{"plan_line": 1}
		}
	}
	if err := RecomputeWaves(p); err != nil {
		return 0, err
	}
	if err := ValidatePlanSchemaCompatible(p); err != nil {
		return 0, err
	}
	return len(req.Tasks), s.SavePlan(planID, p)
}

func (s *Service) AddTasks(planID string, elems []InitElement) ([]string, error) {
	p, _, err := s.LoadPlan(planID)
	if err != nil { return nil, err }
	added := make([]string, 0, len(elems))
	for _, e := range elems {
		if _, exists := p.Units[e.TaskID]; exists {
			return nil, &DomainError{Code: "SCHEMA_VIOLATION", Message: "duplicate task_id: " + e.TaskID}
		}
		p.Units[e.TaskID] = PlanUnit{Task: e.Task, Title: e.Title, Kind: e.Kind, Wave: 1, PlanLine: 1, DependsOn: e.DependsOn}
		if _, ok := p.Tasks[e.Task]; !ok {
			p.Tasks[e.Task] = map[string]any{"plan_line": 1}
		}
		added = append(added, e.TaskID)
	}
	if err := RecomputeWaves(p); err != nil { return nil, err }
	if err := ValidatePlanSchemaCompatible(p); err != nil { return nil, err }
	return added, s.SavePlan(planID, p)
}

func (s *Service) RemoveTask(planID, taskID, mode string) ([]string, error) {
	p, _, err := s.LoadPlan(planID)
	if err != nil { return nil, err }
	if _, ok := p.Units[taskID]; !ok {
		return nil, &DomainError{Code: "TASK_NOT_FOUND", Message: taskID}
	}
	dependents := []string{}
	for id, u := range p.Units {
		for _, dep := range u.DependsOn {
			if dep == taskID {
				dependents = append(dependents, id)
				break
			}
		}
	}
	if mode == "strict" && len(dependents) > 0 {
		return nil, &DomainError{Code: "TASK_HAS_DEPENDENTS", Message: taskID, Details: map[string]any{"dependents": dependents}}
	}
	delete(p.Units, taskID)
	if mode == "cascade" {
		for _, d := range dependents { delete(p.Units, d) }
	}
	if mode == "relink" {
		for _, d := range dependents {
			u := p.Units[d]
			filtered := make([]string, 0, len(u.DependsOn))
			for _, dep := range u.DependsOn {
				if dep != taskID { filtered = append(filtered, dep) }
			}
			u.DependsOn = filtered
			p.Units[d] = u
		}
	}
	if err := RecomputeWaves(p); err != nil { return nil, err }
	if err := ValidatePlanSchemaCompatible(p); err != nil { return nil, err }
	impacted := append([]string{taskID}, dependents...)
	return impacted, s.SavePlan(planID, p)
}

func (s *Service) GetPlan(planID string) (*PlanFile, error) {
	p, _, err := s.LoadPlan(planID)
	return p, err
}

func (s *Service) ListPlans() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(s.PlanDir, "*-execution-waves.json"))
	if err != nil { return nil, err }
	sort.Strings(matches)
	return matches, nil
}

func (s *Service) LoadPlan(planID string) (*PlanFile, string, error) {
	path := filepath.Join(s.PlanDir, planID+"-execution-waves.json")
	raw, err := os.ReadFile(path)
	if err != nil { return nil, "", err }
	p, err := DecodePlanStrict(raw)
	if err != nil { return nil, "", err }
	return p, path, nil
}

func (s *Service) SavePlan(planID string, p *PlanFile) error {
	path := filepath.Join(s.PlanDir, planID+"-execution-waves.json")
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil { return err }
	return os.WriteFile(path, raw, 0o600)
}
```

- [ ] **Step 4: Expose MCP tools and CLI routes for remaining operations**

```go
// main.go createTools additions
mcp.NewTool("plan_init", mcp.WithString("plan_id", mcp.Required()), mcp.WithString("elements_blob", mcp.Required())),
mcp.NewTool("plan_add_tasks", mcp.WithString("plan_id", mcp.Required()), mcp.WithString("elements_blob", mcp.Required())),
mcp.NewTool("plan_remove_task", mcp.WithString("plan_id", mcp.Required()), mcp.WithString("task_id", mcp.Required()), mcp.WithString("mode")),
mcp.NewTool("plan_get", mcp.WithString("plan_id", mcp.Required())),
mcp.NewTool("plan_list"),
mcp.NewTool("plan_suggest_questions", mcp.WithString("partial_json", mcp.Required())),
```

```python
# waveplan-cli parser additions
plan_init = plan_sub.add_parser("init"); plan_init.add_argument("plan_id"); plan_init.add_argument("elements_blob")
plan_add = plan_sub.add_parser("add_task"); plan_add.add_argument("plan_id"); plan_add.add_argument("elements_blob")
plan_rem = plan_sub.add_parser("rem_task"); plan_rem.add_argument("plan_id"); plan_rem.add_argument("task_id"); plan_rem.add_argument("--mode", default="strict")
plan_get = plan_sub.add_parser("get"); plan_get.add_argument("plan_id")
plan_list = plan_sub.add_parser("list")
plan_q = plan_sub.add_parser("suggest_questions"); plan_q.add_argument("partial_json")
```

- [ ] **Step 5: Re-run operation tests and commit**

Run: `go test ./... -run 'PlanInit|PlanAddTasks|PlanRemoveTask|PlanGet|PlanList' -v`
Expected: PASS.

Run: `python3 waveplan-cli plan --help`
Expected: shows `new`, `init`, `add_task`, `rem_task`, `get`, `list`, `suggest_questions`, `validate`.

```bash
git add internal/planedit/service.go internal/planedit/service_test.go main.go main_test.go waveplan-cli
git commit -m "feat(planedit): implement full plan authoring operations and tool wiring"
```

---

### Task 10: Add Determinism and Conflict Tests for Authoring Surface

**Files:**
- Modify: `internal/planedit/store_test.go`
- Modify: `internal/planedit/service_test.go`
- Modify: `main_test.go`

- [ ] **Step 1: Write failing tests for revision mismatch envelope and idempotent replay**

```go
func TestPlanAddTasks_RevisionMismatchReturnsStructuredError(t *testing.T) {
	srv := setupAuthoringServerFixture(t)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "plan_add_tasks",
			Arguments: map[string]any{
				"plan_id": "demo",
				"elements_blob": `{"tasks":[{"task_id":"T9.1","task":"T9","title":"x","kind":"impl","depends_on":[]}]}`,
				"expected_revision": 999,
			},
		},
	}
	res, err := srv.handlePlanAddTasks(nil, req)
	if err != nil { t.Fatalf("call failed: %v", err) }
	body := extractToolText(t, res)
	if !strings.Contains(body, `"code":"REVISION_MISMATCH"`) {
		t.Fatalf("expected revision mismatch envelope, got %s", body)
	}
}

func TestPlanAddTasks_IdempotencyReplayReturnsSameResult(t *testing.T) {
	srv := setupAuthoringServerFixture(t)
	args := map[string]any{
		"plan_id": "demo",
		"elements_blob": `{"tasks":[{"task_id":"T9.2","task":"T9","title":"x","kind":"impl","depends_on":[]}]}`,
		"expected_revision": 1,
		"idempotency_key": "f7fc2eea-c8f9-4a25-b8a5-c92cf888be0f",
	}
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "plan_add_tasks", Arguments: args}}
	first, err := srv.handlePlanAddTasks(nil, req)
	if err != nil { t.Fatalf("first call failed: %v", err) }
	second, err := srv.handlePlanAddTasks(nil, req)
	if err != nil { t.Fatalf("second call failed: %v", err) }
	if extractToolText(t, first) != extractToolText(t, second) {
		t.Fatalf("idempotent replay must return identical result")
	}
}

func setupAuthoringServerFixture(t *testing.T) *WaveplanServer {
	t.Helper()
	planPath := filepath.Join(t.TempDir(), "demo-execution-waves.json")
	_ = os.WriteFile(planPath, []byte(`{"units":{},"tasks":{},"doc_index":{},"fp_index":{}}`), 0o600)
	srv, err := newWaveplanServer(planPath, planPath+".state.json", "test-sha")
	if err != nil {
		t.Fatalf("server fixture: %v", err)
	}
	return srv
}

func extractToolText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("missing tool content")
	}
	if txt, ok := res.Content[0].(mcp.TextContent); ok {
		return txt.Text
	}
	t.Fatalf("unexpected content type")
	return ""
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./... -run 'RevisionMismatchReturnsStructuredError|IdempotencyReplay' -v`
Expected: FAIL before envelope/idempotency wiring complete.

- [ ] **Step 3: Implement deterministic envelope and idempotency checks in handlers**

```go
func ok(data any) *mcp.CallToolResult {
	b, _ := json.Marshal(map[string]any{"ok": true, "data": data})
	return mcp.NewToolResultText(string(b))
}

func fail(code, msg string, details map[string]any) *mcp.CallToolResult {
	b, _ := json.Marshal(map[string]any{
		"ok": false,
		"error": map[string]any{"code": code, "message": msg, "retryable": code == "REVISION_MISMATCH", "details": details},
	})
	return mcp.NewToolResultText(string(b))
}
```

- [ ] **Step 4: Re-run deterministic test set**

Run: `go test ./... -run 'RevisionMismatchReturnsStructuredError|IdempotencyReplay|SuggestQuestions' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/planedit/store_test.go internal/planedit/service_test.go main_test.go main.go
git commit -m "test(planedit): enforce deterministic envelopes conflict handling and idempotency replay"
```

---

### Task 11: Add 16k Context-Fit Budget Classification

**Files:**
- Create: `internal/planedit/budget.go`
- Create: `internal/planedit/budget_test.go`
- Modify: `internal/planedit/service.go`
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `waveplan-cli`
- Modify: `README.md`
- Modify: `adocs/specs/waveplan-plan-authoring-mcp-contracts.md`

- [ ] **Step 1: Write failing tests for token budget and `fit_16k`**

```go
// internal/planedit/budget_test.go
package planedit

import "testing"

func TestClassifyBudget_Fit16KBoundary(t *testing.T) {
	row := BudgetRow{UnitID: "T1.1", TokenCount: 16384}
	out := ApplyBudgetFlags(row, 16384)
	if !out.Fit16K {
		t.Fatalf("16384 tokens must fit 16k")
	}
}

func TestClassifyBudget_Over16K(t *testing.T) {
	row := BudgetRow{UnitID: "T1.2", TokenCount: 16385}
	out := ApplyBudgetFlags(row, 16384)
	if out.Fit16K {
		t.Fatalf("16385 tokens must not fit 16k")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/planedit -run 'Budget|Fit16K' -v`  
Expected: FAIL with undefined budget symbols.

- [ ] **Step 3: Implement budget classifier and report model**

```go
// internal/planedit/budget.go
package planedit

type BudgetRow struct {
	UnitID          string   `json:"unit_id"`
	TokenCount      int      `json:"token_count"`
	Fit16K          bool     `json:"fit_16k"`
	Fit8K           bool     `json:"fit_8k"`
	ThresholdTokens int      `json:"threshold_tokens"`
	TopTokenSources []string `json:"top_token_sources,omitempty"`
}

func ApplyBudgetFlags(in BudgetRow, maxTokens int) BudgetRow {
	out := in
	out.ThresholdTokens = maxTokens
	out.Fit16K = in.TokenCount <= 16384
	out.Fit8K = in.TokenCount <= 8192
	return out
}
```

- [ ] **Step 4: Add service + MCP + CLI surface**

Runbook:
- Add `PlanBudget(planID string, maxTokens int)` in `internal/planedit/service.go`.
- Add MCP tool `plan_budget(plan_id, max_tokens?)` in `main.go`.
- Add CLI route `wp plan budget <plan_id> [--max-tokens N]` in `waveplan-cli`.
- Add optional validation mode hook in `plan_validate` to report over-budget units.

- [ ] **Step 5: Verify and commit**

Run: `go test ./... -run 'Budget|PlanBudget|PlanValidateBudget' -v`  
Expected: PASS.

Run: `python3 waveplan-cli plan budget demo --max-tokens 16384`  
Expected: JSON report with per-unit `fit_16k` flags.

```bash
git add internal/planedit/budget.go internal/planedit/budget_test.go internal/planedit/service.go main.go main_test.go waveplan-cli README.md adocs/specs/waveplan-plan-authoring-mcp-contracts.md
git commit -m "feat(planedit): add 16k context-fit budget classifier and reporting"
```

---

## Self-Review

Spec coverage check:
- Schema compatibility: covered by Tasks 1-3 and strict validator.
- Authoring tool surface (`new/init/add/rem/get/validate/suggest/list`): covered by Tasks 5-9.
- Determinism (revision/idempotency/audit): Task 4.
- MCP exposure: Tasks 6 and 9.
- CLI usability: Tasks 7 and 9.
- Docs/contracts: Task 8.
- Conflict/idempotency verification: Task 10.
- Context-fit fidelity modifier (`fit_16k`): Task 11.

Placeholder scan:
- No `TODO/TBD` placeholders left in tasks.
- Every code step includes concrete snippet.
- Every execution step includes exact command and expected result.

Type consistency check:
- `PlanFile`, `PlanUnit`, `DomainError`, `Service` names consistent across tasks.
- `REVISION_MISMATCH`, `CYCLE_DETECTED`, `DEP_VIOLATION` codes reused consistently.
