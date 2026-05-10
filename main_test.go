package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/swim"
	"github.com/mark3labs/mcp-go/mcp"
)

type traceRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

func startTraceServer(t *testing.T) (*httptest.Server, *[]traceRequest) {
	t.Helper()
	var (
		mu       sync.Mutex
		requests []traceRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload map[string]any
		if len(body) > 0 {
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Errorf("json.Unmarshal trace body: %v\nbody=%s", err, body)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		mu.Lock()
		requests = append(requests, traceRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   payload,
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

// makeTestServer creates a WaveplanServer with the given plan JSON bytes.
func makeTestServer(t *testing.T, planJSON []byte) *WaveplanServer {
	t.Helper()
	var plan WaveplanPlan
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		t.Fatalf("failed to parse plan JSON: %v", err)
	}
	state := &WaveplanState{
		Plan:      "test-plan.json",
		Taken:     make(map[string]TaskEntry),
		Completed: make(map[string]TaskEntry),
		Tail:      make(map[string]TaskEntry),
	}
	return &WaveplanServer{
		plan:      &plan,
		state:     state,
		planPath:  "test-plan.json",
		statePath: "",
	}
}

// parseJSONMap parses a JSON string into a map[string]any for comparison.
func parseJSONMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	return m
}

// testPlanJSON is a minimal plan for testing.
var testPlanJSON = []byte(`{
  "units": {
    "T1.1": {
      "task": "T1",
      "title": "Task 1",
      "kind": "impl",
      "wave": 1,
      "plan_line": 20,
      "depends_on": [],
      "doc_refs": ["plan", "mcp_main"],
      "fp_refs": [],
      "notes": ["Note 1"],
      "command": "go build"
    },
    "T2.1": {
      "task": "T2",
      "title": "Task 2",
      "kind": "impl",
      "wave": 2,
      "plan_line": 42,
      "depends_on": ["T1.1"],
      "doc_refs": ["spec"],
      "fp_refs": ["fp-123"],
      "notes": ["Note 2"],
      "command": "go test"
    },
    "T3.1": {
      "task": "T3",
      "title": "Task 3",
      "kind": "test",
      "wave": 2,
      "plan_line": 60,
      "depends_on": ["T1.1"],
      "doc_refs": [],
      "fp_refs": [],
      "notes": []
    }
  },
  "tasks": {
    "T1": {"plan_line": 20},
    "T2": {"plan_line": 42},
    "T3": {"plan_line": 60}
  },
  "doc_index": {
    "plan": {"path": "docs/superpowers/plans/test.json", "line": 20, "kind": "plan"},
    "spec": {"path": "docs/superpowers/specs/test.md", "line": 1, "kind": "spec"},
    "mcp_main": {"path": ".worktrees/waveplan-mcp/main.go", "line": 1, "kind": "code"}
  },
  "fp_index": {
    "fp-123": "https://fiberplane.com/issues/FP-123"
  }
}`)

func TestFindPlanRef(t *testing.T) {
	srv := makeTestServer(t, testPlanJSON)

	// T1.1 has plan_line 20, should match "plan" doc_index entry
	ref := srv.findPlanRef("T1.1")
	if ref == nil {
		t.Fatal("findPlanRef(T1.1) returned nil")
	}
	if ref["path"] != "docs/superpowers/plans/test.json" {
		t.Errorf("expected path 'docs/superpowers/plans/test.json', got '%v'", ref["path"])
	}
	if ref["line"] != 20 {
		t.Errorf("expected line 20, got %v", ref["line"])
	}

	// T2.1 has plan_line 42, should match "plan" doc_index entry with line 42
	// But our doc_index only has "plan" with line 20, so this should return nil
	ref = srv.findPlanRef("T2.1")
	if ref != nil {
		t.Errorf("expected nil for T2.1 (no matching plan_line in doc_index), got %v", ref)
	}
}

func TestResolveDocRefs(t *testing.T) {
	srv := makeTestServer(t, testPlanJSON)

	// T1.1 has doc_refs ["plan", "mcp_main"]
	unit := srv.plan.Units["T1.1"]
	refs := srv.resolveDocRefs(unit.DocRefs)
	if len(refs) != 2 {
		t.Fatalf("expected 2 doc refs, got %d", len(refs))
	}
	// Check that each ref has the correct path
	refMap := make(map[string]map[string]any)
	for _, r := range refs {
		refMap[r["ref"].(string)] = r
	}
	if p, ok := refMap["plan"]; ok {
		if p["path"] != "docs/superpowers/plans/test.json" {
			t.Errorf("plan ref path: got '%v'", p["path"])
		}
	} else {
		t.Error("missing 'plan' in doc refs")
	}
	if m, ok := refMap["mcp_main"]; ok {
		if m["path"] != ".worktrees/waveplan-mcp/main.go" {
			t.Errorf("mcp_main ref path: got '%v'", m["path"])
		}
	} else {
		t.Error("missing 'mcp_main' in doc refs")
	}

	// T3.1 has empty doc_refs
	unit3 := srv.plan.Units["T3.1"]
	refs3 := srv.resolveDocRefs(unit3.DocRefs)
	if len(refs3) != 0 {
		t.Errorf("expected 0 doc refs for T3.1, got %d", len(refs3))
	}
}

func TestResolveFpRefs(t *testing.T) {
	srv := makeTestServer(t, testPlanJSON)

	// T2.1 has fp_refs ["fp-123"]
	unit := srv.plan.Units["T2.1"]
	refs := srv.resolveFpRefs(unit.FpRefs)
	if len(refs) != 1 {
		t.Fatalf("expected 1 fp ref, got %d", len(refs))
	}
	if refs[0]["ref"] != "fp-123" {
		t.Errorf("expected ref 'fp-123', got '%v'", refs[0]["ref"])
	}
	if refs[0]["fp_id"] != "https://fiberplane.com/issues/FP-123" {
		t.Errorf("expected fp_id 'https://fiberplane.com/issues/FP-123', got '%v'", refs[0]["fp_id"])
	}

	// T1.1 has empty fp_refs
	unit1 := srv.plan.Units["T1.1"]
	refs1 := srv.resolveFpRefs(unit1.FpRefs)
	if len(refs1) != 0 {
		t.Errorf("expected 0 fp refs for T1.1, got %d", len(refs1))
	}
}

func TestNilIfEmpty(t *testing.T) {
	if nilIfEmpty("") != nil {
		t.Error("nilIfEmpty(\"\") should return nil")
	}
	if nilIfEmpty("hello") != "hello" {
		t.Error("nilIfEmpty(\"hello\") should return \"hello\"")
	}
}

func TestTaskInfo(t *testing.T) {
	srv := makeTestServer(t, testPlanJSON)

	info := srv.taskInfo("T1.1")
	if info["task_id"] != "T1.1" {
		t.Errorf("task_id: got '%v'", info["task_id"])
	}
	if info["title"] != "Task 1" {
		t.Errorf("title: got '%v'", info["title"])
	}
	if info["task"] != "T1" {
		t.Errorf("task: got '%v'", info["task"])
	}
	if info["kind"] != "impl" {
		t.Errorf("kind: got '%v'", info["kind"])
	}
	if info["wave"] != 1 {
		t.Errorf("wave: got %v", info["wave"])
	}
	if info["plan_line"] != 20 {
		t.Errorf("plan_line: got %v", info["plan_line"])
	}

	// Check plan_ref
	planRef, ok := info["plan_ref"].(map[string]any)
	if !ok {
		t.Fatal("plan_ref should be a map")
	}
	if planRef["path"] != "docs/superpowers/plans/test.json" {
		t.Errorf("plan_ref path: got '%v'", planRef["path"])
	}

	// Check doc_refs
	docRefs, ok := info["doc_refs"].([]map[string]any)
	if !ok {
		t.Fatal("doc_refs should be a slice")
	}
	if len(docRefs) != 2 {
		t.Errorf("expected 2 doc_refs, got %d", len(docRefs))
	}

	// Check fp_refs (T1.1 has none)
	if _, ok := info["fp_refs"]; ok {
		t.Error("T1.1 should not have fp_refs in taskInfo")
	}

	// Check notes
	notes, ok := info["notes"].([]string)
	if !ok {
		t.Fatal("notes should be a slice")
	}
	if len(notes) != 1 || notes[0] != "Note 1" {
		t.Errorf("notes: got %v", notes)
	}

	// Check command
	if info["command"] != "go build" {
		t.Errorf("command: got '%v'", info["command"])
	}
}

func TestTopologicalSort_NoDeps(t *testing.T) {
	srv := makeTestServer(t, testPlanJSON)

	sorted := srv.topologicalSort()
	if len(sorted) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(sorted))
	}

	// T1.1 has no deps, should be group 1
	if sorted[0].TaskID != "T1.1" {
		t.Errorf("first task should be T1.1, got '%s'", sorted[0].TaskID)
	}
	if sorted[0].Group != 1 {
		t.Errorf("T1.1 should be group 1, got %d", sorted[0].Group)
	}

	// T2.1 and T3.1 depend on T1.1, should be group 2
	// T2.1 and T3.1 are both wave 2, sorted by task ID
	var foundT2, foundT3 bool
	for _, item := range sorted {
		if item.TaskID == "T2.1" {
			foundT2 = true
			if item.Group != 2 {
				t.Errorf("T2.1 should be group 2, got %d", item.Group)
			}
		}
		if item.TaskID == "T3.1" {
			foundT3 = true
			if item.Group != 2 {
				t.Errorf("T3.1 should be group 2, got %d", item.Group)
			}
		}
	}
	if !foundT2 || !foundT3 {
		t.Error("missing T2.1 or T3.1 in sorted output")
	}
}

func TestCreateTools_IncludesSwimTools(t *testing.T) {
	srv := makeTestServer(t, testPlanJSON)
	tools := srv.createTools()
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Tool.Name] = true
	}
	for _, want := range []string{
		"waveplan_swim_compile",
		"waveplan_swim_next",
		"waveplan_swim_step",
		"waveplan_swim_run",
		"waveplan_swim_journal",
		"waveplan_swim_refine",
		"waveplan_swim_refine_run",
	} {
		if !names[want] {
			t.Fatalf("missing tool %s", want)
		}
	}
}

func TestHandleSwimNext(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := os.WriteFile(statePath, []byte(`{"plan":"demo","taken":{},"completed":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := makeTestServer(t, testPlanJSON)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"schedule": filepath.Join("tests", "swim", "fixtures", "expected-schedule.json"),
			"journal":  filepath.Join(dir, "journal.json"),
			"state":    statePath,
		}},
	}
	res, err := srv.handleSwimNext(nil, req)
	if err != nil {
		t.Fatalf("handleSwimNext: %v", err)
	}
	got := parseJSONMap(t, res.Content[0].(mcp.TextContent).Text)
	if got["action"] != "ready" {
		t.Fatalf("action = %v, want ready", got["action"])
	}
	row := got["row"].(map[string]any)
	if row["step_id"] != "S1_T1.1_implement" {
		t.Fatalf("step_id = %v", row["step_id"])
	}
}

func TestHandleSwimStepApply(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := os.WriteFile(statePath, []byte(`{"plan":"demo","taken":{},"completed":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	schedulePath := filepath.Join(dir, "apply-schedule.json")
	schedule := `{
  "schema_version": 2,
  "execution": [
    {
      "seq": 1,
      "step_id": "S1_T1.1_implement",
      "task_id": "T1.1",
      "action": "implement",
      "requires": {"task_status": "available"},
      "produces": {"task_status": "taken"},
      "invoke": {"argv": ["/bin/sh", "-c", "python3 - <<'PY'\nimport json\npath = r'` + statePath + `'\nwith open(path, 'r', encoding='utf-8') as f:\n    data = json.load(f)\ndata['taken']['T1.1'] = {'taken_by': 'phi', 'started_at': '2026-05-09T00:00:00Z'}\nwith open(path, 'w', encoding='utf-8') as f:\n    json.dump(data, f)\nPY"]}
    }
  ]
}`
	if err := os.WriteFile(schedulePath, []byte(schedule), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := makeTestServer(t, testPlanJSON)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"schedule": schedulePath,
			"journal":  filepath.Join(dir, "journal.json"),
			"state":    statePath,
			"apply":    true,
		}},
	}
	res, err := srv.handleSwimStep(nil, req)
	if err != nil {
		t.Fatalf("handleSwimStep: %v", err)
	}
	got := parseJSONMap(t, res.Content[0].(mcp.TextContent).Text)
	if got["status"] != "applied" {
		t.Fatalf("status = %v, want applied", got["status"])
	}
}

func TestHandleSwimStepAckUnknown(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "journal.json")
	body := `{
  "schema_version": 1,
  "schedule_path": "demo-schedule.json",
  "cursor": 0,
  "events": [
    {
      "event_id": "E0001",
      "step_id": "S1_T1.1_implement",
      "seq": 1,
      "task_id": "T1.1",
      "action": "implement",
      "attempt": 1,
      "started_on": "2026-05-09T00:00:00Z",
      "completed_on": "2026-05-09T00:00:01Z",
      "outcome": "unknown",
      "state_before": {"task_status": "available"},
      "state_after": {"task_status": "taken"}
    }
  ]
}`
	if err := os.WriteFile(journalPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := makeTestServer(t, testPlanJSON)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"journal":     journalPath,
			"ack_unknown": "S1_T1.1_implement",
			"as":          "waived",
		}},
	}
	res, err := srv.handleSwimStep(nil, req)
	if err != nil {
		t.Fatalf("handleSwimStep ack: %v", err)
	}
	got := parseJSONMap(t, res.Content[0].(mcp.TextContent).Text)
	if got["outcome"] != "waived" {
		t.Fatalf("outcome = %v, want waived", got["outcome"])
	}
	after, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), `"outcome": "waived"`) {
		t.Fatalf("journal not updated: %s", after)
	}
}

func TestHandleSwimRunDryRun(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := os.WriteFile(statePath, []byte(`{"plan":"demo","taken":{},"completed":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := makeTestServer(t, testPlanJSON)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"schedule": filepath.Join("tests", "swim", "fixtures", "expected-schedule.json"),
			"journal":  filepath.Join(dir, "journal.json"),
			"state":    statePath,
			"until":    "finish",
			"dry_run":  true,
		}},
	}
	res, err := srv.handleSwimRun(nil, req)
	if err != nil {
		t.Fatalf("handleSwimRun: %v", err)
	}
	got := parseJSONMap(t, res.Content[0].(mcp.TextContent).Text)
	if got["dry_run"] != true {
		t.Fatalf("dry_run = %v, want true", got["dry_run"])
	}
	steps := got["steps"].([]any)
	if len(steps) != 4 {
		t.Fatalf("steps len = %d, want 4", len(steps))
	}
}

func TestHandleSwimJournalTail(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "journal.json")
	body := `{
  "schema_version": 1,
  "schedule_path": "demo-schedule.json",
  "cursor": 5,
  "events": [
    {"event_id":"E0001","step_id":"S1_T1.1_implement","seq":1,"task_id":"T1.1","action":"implement","attempt":1,"started_on":"2026-05-09T00:00:00Z","completed_on":"2026-05-09T00:00:01Z","outcome":"applied","state_before":{"task_status":"available"},"state_after":{"task_status":"taken"}},
    {"event_id":"E0002","step_id":"S1_T1.1_review","seq":2,"task_id":"T1.1","action":"review","attempt":1,"started_on":"2026-05-09T00:00:00Z","completed_on":"2026-05-09T00:00:01Z","outcome":"applied","state_before":{"task_status":"taken"},"state_after":{"task_status":"review_taken"}},
    {"event_id":"E0003","step_id":"S1_T1.1_end_review","seq":3,"task_id":"T1.1","action":"end_review","attempt":1,"started_on":"2026-05-09T00:00:00Z","completed_on":"2026-05-09T00:00:01Z","outcome":"applied","state_before":{"task_status":"review_taken"},"state_after":{"task_status":"review_ended"}},
    {"event_id":"E0004","step_id":"S1_T1.1_finish","seq":4,"task_id":"T1.1","action":"finish","attempt":1,"started_on":"2026-05-09T00:00:00Z","completed_on":"2026-05-09T00:00:01Z","outcome":"applied","state_before":{"task_status":"review_ended"},"state_after":{"task_status":"completed"}},
    {"event_id":"E0005","step_id":"S1_T1.2_implement","seq":5,"task_id":"T1.2","action":"implement","attempt":1,"started_on":"2026-05-09T00:00:00Z","completed_on":"2026-05-09T00:00:01Z","outcome":"applied","state_before":{"task_status":"available"},"state_after":{"task_status":"taken"}}
  ]
}`
	if err := os.WriteFile(journalPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := makeTestServer(t, testPlanJSON)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"schedule": filepath.Join("tests", "swim", "fixtures", "expected-schedule.json"),
			"journal":  journalPath,
			"tail":     3.0,
		}},
	}
	res, err := srv.handleSwimJournal(nil, req)
	if err != nil {
		t.Fatalf("handleSwimJournal: %v", err)
	}
	got := parseJSONMap(t, res.Content[0].(mcp.TextContent).Text)
	events := got["events"].([]any)
	if len(events) != 3 {
		t.Fatalf("events len = %d, want 3", len(events))
	}
	first := events[0].(map[string]any)
	if first["event_id"] != "E0003" {
		t.Fatalf("first event_id = %v, want E0003", first["event_id"])
	}
}

func TestHandleSwimCompile(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, "waveagents.json")
	if err := os.WriteFile(agentsPath, []byte(`{
  "agents": [
    {"name": "phi", "provider": "codex"},
    {"name": "sigma", "provider": "claude"}
  ],
  "schedule": ["phi", "sigma"]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := makeTestServer(t, testPlanJSON)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"plan":       filepath.Join("docs", "plans", "2026-05-05-swim-execution-waves.json"),
			"agents":     agentsPath,
			"task_scope": "all",
		}},
	}
	res, err := srv.handleSwimCompile(nil, req)
	if err != nil {
		t.Fatalf("handleSwimCompile: %v", err)
	}
	got := parseJSONMap(t, res.Content[0].(mcp.TextContent).Text)
	if got["schema_version"] != float64(2) {
		t.Fatalf("schema_version = %v, want 2", got["schema_version"])
	}
}

func TestHandleSwimRefine(t *testing.T) {
	dir := t.TempDir()
	coarsePath := filepath.Join(dir, "coarse.json")
	coarse := `{
  "schema_version": 1,
  "generated_on": "2026-05-09",
  "plan_version": 1,
  "plan_generation": "2026-05-09T00:00:00Z",
  "plan": {"id": "t7-1-refine"},
  "fp_index": {},
  "doc_index": {},
  "tasks": {
    "T1": {"title": "task one", "files": ["a.go","b.go","c.go","d.go","e.go","f.go","g.go"]}
  },
  "units": {
    "T1.1": {
      "task": "T1",
      "title": "impl unit",
      "kind": "impl",
      "wave": 1,
      "plan_line": 1,
      "depends_on": [],
      "files": ["a.go","b.go","c.go","d.go","e.go","f.go","g.go"]
    }
  }
}`
	if err := os.WriteFile(coarsePath, []byte(coarse), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := makeTestServer(t, testPlanJSON)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"plan":    coarsePath,
			"targets": []any{"T1.1"},
		}},
	}
	res, err := srv.handleSwimRefine(nil, req)
	if err != nil {
		t.Fatalf("handleSwimRefine: %v", err)
	}
	got := parseJSONMap(t, res.Content[0].(mcp.TextContent).Text)
	if got["schema_version"] != float64(1) {
		t.Fatalf("schema_version = %v, want 1", got["schema_version"])
	}
	targets := got["targets"].([]any)
	if len(targets) != 1 || targets[0] != "T1.1" {
		t.Fatalf("targets = %v, want [T1.1]", targets)
	}
	units := got["units"].([]any)
	if len(units) != 2 {
		t.Fatalf("units len = %d, want 2", len(units))
	}
}

func TestHandleSwimRefineRunDryRun(t *testing.T) {
	dir := t.TempDir()
	coarsePath := filepath.Join(dir, "coarse.json")
	coarse := `{
  "schema_version": 1,
  "generated_on": "2026-05-09",
  "plan_version": 1,
  "plan_generation": "2026-05-09T00:00:00Z",
  "plan": {"id": "t7-1-refine-run"},
  "fp_index": {},
  "doc_index": {},
  "tasks": {
    "T1": {"title": "task one", "files": ["a.go","b.go","c.go","d.go","e.go","f.go","g.go"]}
  },
  "units": {
    "T1.1": {
      "task": "T1",
      "title": "impl unit",
      "kind": "impl",
      "wave": 1,
      "plan_line": 1,
      "depends_on": [],
      "files": ["a.go","b.go","c.go","d.go","e.go","f.go","g.go"]
    }
  }
}`
	if err := os.WriteFile(coarsePath, []byte(coarse), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecar, err := swim.Refine(swim.RefineOptions{
		CoarsePlanPath: coarsePath,
		Profile:        swim.ProfileEightK,
		Targets:        []string{"T1.1"},
	})
	if err != nil {
		t.Fatalf("Refine: %v", err)
	}
	body, err := swim.MarshalSidecar(sidecar)
	if err != nil {
		t.Fatalf("MarshalSidecar: %v", err)
	}
	refinePath := filepath.Join(dir, "refine.json")
	if err := os.WriteFile(refinePath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(dir, "state.json")
	state := `{
  "plan": "demo",
  "taken": {
    "T1.1": {
      "taken_by": "phi",
      "started_at": "2026-05-09 10:00",
      "review_entered_at": "2026-05-09 10:30"
    }
  },
  "completed": {}
}`
	if err := os.WriteFile(statePath, []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := makeTestServer(t, testPlanJSON)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{
			"refine":  refinePath,
			"state":   statePath,
			"dry_run": true,
		}},
	}
	res, err := srv.handleSwimRefineRun(nil, req)
	if err != nil {
		t.Fatalf("handleSwimRefineRun: %v", err)
	}
	got := parseJSONMap(t, res.Content[0].(mcp.TextContent).Text)
	if got["dry_run"] != true {
		t.Fatalf("dry_run = %v, want true", got["dry_run"])
	}
	steps := got["steps"].([]any)
	if len(steps) == 0 {
		t.Fatal("expected non-empty steps in dry-run report")
	}
}

func TestTopologicalSort_WaveOrdering(t *testing.T) {
	// Create a plan where T3.1 has wave 1 and T2.1 has wave 2, both with no deps
	planJSON := []byte(`{
		"units": {
			"T3.1": {"task": "T3", "title": "Task 3", "kind": "test", "wave": 1, "plan_line": 60, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []},
			"T2.1": {"task": "T2", "title": "Task 2", "kind": "impl", "wave": 2, "plan_line": 42, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []}
		},
		"tasks": {"T2": {"plan_line": 42}, "T3": {"plan_line": 60}},
		"doc_index": {},
		"fp_index": {}
	}`)
	srv := makeTestServer(t, planJSON)

	sorted := srv.topologicalSort()
	if len(sorted) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(sorted))
	}
	// T3.1 has wave 1, should come before T2.1 (wave 2) in the same group
	if sorted[0].TaskID != "T3.1" {
		t.Errorf("first task should be T3.1 (wave 1), got '%s'", sorted[0].TaskID)
	}
	if sorted[1].TaskID != "T2.1" {
		t.Errorf("second task should be T2.1 (wave 2), got '%s'", sorted[1].TaskID)
	}
}

func TestTopologicalSort_MultiLevel(t *testing.T) {
	// Create a plan with 3 levels: T1.1 (no deps), T2.1 (depends on T1.1), T3.1 (depends on T2.1)
	planJSON := []byte(`{
		"units": {
			"T1.1": {"task": "T1", "title": "Task 1", "kind": "impl", "wave": 1, "plan_line": 10, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []},
			"T2.1": {"task": "T2", "title": "Task 2", "kind": "impl", "wave": 2, "plan_line": 20, "depends_on": ["T1.1"], "doc_refs": [], "fp_refs": [], "notes": []},
			"T3.1": {"task": "T3", "title": "Task 3", "kind": "test", "wave": 3, "plan_line": 30, "depends_on": ["T2.1"], "doc_refs": [], "fp_refs": [], "notes": []}
		},
		"tasks": {"T1": {"plan_line": 10}, "T2": {"plan_line": 20}, "T3": {"plan_line": 30}},
		"doc_index": {},
		"fp_index": {}
	}`)
	srv := makeTestServer(t, planJSON)

	sorted := srv.topologicalSort()
	if len(sorted) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(sorted))
	}
	if sorted[0].TaskID != "T1.1" || sorted[0].Group != 1 {
		t.Errorf("T1.1 should be group 1, got %v", sorted[0])
	}
	if sorted[1].TaskID != "T2.1" || sorted[1].Group != 2 {
		t.Errorf("T2.1 should be group 2, got %v", sorted[1])
	}
	if sorted[2].TaskID != "T3.1" || sorted[2].Group != 3 {
		t.Errorf("T3.1 should be group 3, got %v", sorted[2])
	}
}

func TestTaskInfo_JSONMarshal(t *testing.T) {
	srv := makeTestServer(t, testPlanJSON)

	info := srv.taskInfo("T1.1")
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal taskInfo: %v", err)
	}

	// Verify it parses back correctly
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal taskInfo JSON: %v", err)
	}
	if parsed["task_id"] != "T1.1" {
		t.Errorf("task_id mismatch after round-trip")
	}
	if parsed["title"] != "Task 1" {
		t.Errorf("title mismatch after round-trip")
	}
}

func TestTaskInfo_EmptyFields(t *testing.T) {
	srv := makeTestServer(t, testPlanJSON)

	// T3.1 has no doc_refs, fp_refs, or notes
	info := srv.taskInfo("T3.1")
	if _, ok := info["doc_refs"]; ok {
		t.Error("T3.1 should not have doc_refs")
	}
	if _, ok := info["fp_refs"]; ok {
		t.Error("T3.1 should not have fp_refs")
	}
	if _, ok := info["notes"]; ok {
		t.Error("T3.1 should not have notes")
	}
	if _, ok := info["command"]; ok {
		t.Error("T3.1 should not have command")
	}
}

// TestFinBackfillsStartedAt verifies that handleFin backfills started_at
// when a task is completed without being popped (matches Python CLI behavior).
func TestFinBackfillsStartedAt(t *testing.T) {
	planJSON := []byte(`{
		"units": {
			"T1.1": {"task": "T1", "title": "Task 1", "kind": "impl", "wave": 1, "plan_line": 10, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []},
			"T2.1": {"task": "T2", "title": "Task 2", "kind": "impl", "wave": 2, "plan_line": 20, "depends_on": ["T1.1"], "doc_refs": [], "fp_refs": [], "notes": []}
		},
		"tasks": {"T1": {"plan_line": 10}, "T2": {"plan_line": 20}},
		"doc_index": {},
		"fp_index": {}
	}`)
	srv := makeTestServer(t, planJSON)

	// Pre-complete T1.1 so T2.1's deps are satisfied
	srv.state.Completed["T1.1"] = TaskEntry{
		TakenBy:    "psi",
		StartedAt:  "2026-01-01 00:00",
		FinishedAt: "2026-01-01 00:01",
	}

	// Call handleFin on T2.1 WITHOUT popping it first
	args := map[string]any{"task_id": "T2.1", "git_sha": "def5678"}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
	_, err := srv.handleFin(nil, req)
	if err != nil {
		t.Fatalf("handleFin returned error: %v", err)
	}

	// Verify the completed entry has a non-empty started_at
	completed := srv.state.Completed["T2.1"]
	if completed.StartedAt == "" {
		t.Error("started_at should be backfilled when task is not popped; got empty string")
	}
	if completed.GitSha != "def5678" {
		t.Errorf("git_sha should be 'def5678', got '%s'", completed.GitSha)
	}
	if completed.TakenBy != "" {
		t.Errorf("taken_by should be empty (never popped), got '%s'", completed.TakenBy)
	}
}

// TestFinWithPop verifies that handleFin preserves started_at when task was popped.
func TestFinWithPop(t *testing.T) {
	planJSON := []byte(`{
		"units": {
			"T1.1": {"task": "T1", "title": "Task 1", "kind": "impl", "wave": 1, "plan_line": 10, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []}
		},
		"tasks": {"T1": {"plan_line": 10}},
		"doc_index": {},
		"fp_index": {}
	}`)
	srv := makeTestServer(t, planJSON)

	// Pop the task first
	popArgs := map[string]any{"agent": "psi"}
	popReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: popArgs,
		},
	}
	_, err := srv.handlePop(nil, popReq)
	if err != nil {
		t.Fatalf("handlePop returned error: %v", err)
	}

	// Now fin it
	finArgs := map[string]any{"task_id": "T1.1", "git_sha": "abc1234"}
	finReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: finArgs,
		},
	}
	_, err = srv.handleFin(nil, finReq)
	if err != nil {
		t.Fatalf("handleFin returned error: %v", err)
	}

	completed := srv.state.Completed["T1.1"]
	if completed.StartedAt == "" {
		t.Error("started_at should be set from pop")
	}
	if completed.TakenBy != "psi" {
		t.Errorf("taken_by should be 'psi', got '%s'", completed.TakenBy)
	}
	if completed.GitSha != "abc1234" {
		t.Errorf("git_sha should be 'abc1234', got '%s'", completed.GitSha)
	}
}

// TestFinRecordsTail verifies that handleFin records the taken entry into Tail
// before deleting it from Taken.
func TestFinRecordsTail(t *testing.T) {
	planJSON := []byte(`{
		"units": {
			"T1.1": {"task": "T1", "title": "Task 1", "kind": "impl", "wave": 1, "plan_line": 10, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []}
		},
		"tasks": {"T1": {"plan_line": 10}},
		"doc_index": {},
		"fp_index": {}
	}`)
	srv := makeTestServer(t, planJSON)

	// Pop the task first
	popArgs := map[string]any{"agent": "phi"}
	popReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: popArgs,
		},
	}
	_, err := srv.handlePop(nil, popReq)
	if err != nil {
		t.Fatalf("handlePop returned error: %v", err)
	}

	// Start and end review
	startReviewReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"task_id": "T1.1", "reviewer": "reviewer1"},
		},
	}
	_, err = srv.handleStartReview(nil, startReviewReq)
	if err != nil {
		t.Fatalf("handleStartReview returned error: %v", err)
	}

	endReviewReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"task_id": "T1.1", "review_note": "looks good"},
		},
	}
	_, err = srv.handleEndReview(nil, endReviewReq)
	if err != nil {
		t.Fatalf("handleEndReview returned error: %v", err)
	}

	// Now fin it
	finArgs := map[string]any{"task_id": "T1.1", "git_sha": "abc1234"}
	finReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: finArgs,
		},
	}
	_, err = srv.handleFin(nil, finReq)
	if err != nil {
		t.Fatalf("handleFin returned error: %v", err)
	}

	// Verify the taken entry was removed
	if _, ok := srv.state.Taken["T1.1"]; ok {
		t.Error("T1.1 should be removed from Taken after fin")
	}

	// Verify the tail entry exists and matches the pre-fin taken entry
	tailEntry, ok := srv.state.Tail["T1.1"]
	if !ok {
		t.Fatal("T1.1 should exist in Tail after fin")
	}
	if tailEntry.TakenBy != "phi" {
		t.Errorf("tail taken_by should be 'phi', got '%s'", tailEntry.TakenBy)
	}
	if tailEntry.Reviewer != "reviewer1" {
		t.Errorf("tail reviewer should be 'reviewer1', got '%s'", tailEntry.Reviewer)
	}
	if tailEntry.ReviewNote != "looks good" {
		t.Errorf("tail review_note should be 'looks good', got '%s'", tailEntry.ReviewNote)
	}
}

func TestExecutionReviewLoop_TracesToLangSmith(t *testing.T) {
	server, requests := startTraceServer(t)
	t.Setenv("LANGSMITH_TRACING", "true")
	t.Setenv("LANGSMITH_API_KEY", "test-key")
	t.Setenv("LANGSMITH_ENDPOINT", server.URL)
	t.Setenv("LANGSMITH_PROJECT", "waveplan-test")

	planJSON := []byte(`{
		"units": {
			"T1.1": {"task": "T1", "title": "Task 1", "kind": "impl", "wave": 1, "plan_line": 10, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []}
		},
		"tasks": {"T1": {"plan_line": 10}},
		"doc_index": {},
		"fp_index": {}
	}`)
	srv := makeTestServer(t, planJSON)

	// pop
	_, err := srv.handlePop(nil, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{"agent": "sigma"}},
	})
	if err != nil {
		t.Fatalf("handlePop returned error: %v", err)
	}

	// start review
	_, err = srv.handleStartReview(nil, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{"task_id": "T1.1", "reviewer": "psi"}},
	})
	if err != nil {
		t.Fatalf("handleStartReview returned error: %v", err)
	}

	// end review
	_, err = srv.handleEndReview(nil, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{"task_id": "T1.1", "review_note": "ok"}},
	})
	if err != nil {
		t.Fatalf("handleEndReview returned error: %v", err)
	}

	// fin
	_, err = srv.handleFin(nil, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Arguments: map[string]any{"task_id": "T1.1", "git_sha": "abc1234"}},
	})
	if err != nil {
		t.Fatalf("handleFin returned error: %v", err)
	}

	if got, want := len(*requests), 8; got != want {
		t.Fatalf("trace request count = %d, want %d: %#v", got, want, *requests)
	}

	for i, req := range *requests {
		if i%2 == 0 {
			if req.Method != http.MethodPost || req.Path != "/runs" {
				t.Fatalf("request[%d] = %s %s, want POST /runs", i, req.Method, req.Path)
			}
		} else {
			if req.Method != http.MethodPatch || !strings.HasPrefix(req.Path, "/runs/") {
				t.Fatalf("request[%d] = %s %s, want PATCH /runs/<id>", i, req.Method, req.Path)
			}
			outputs, ok := req.Body["outputs"].(map[string]any)
			if !ok {
				t.Fatalf("request[%d] outputs = %#v, want object", i, req.Body["outputs"])
			}
			if outputs["status"] != "ok" {
				t.Fatalf("request[%d] outputs.status = %#v, want ok", i, outputs["status"])
			}
		}
	}

	if (*requests)[0].Body["name"] != "waveplan pop" {
		t.Fatalf("trace start[0] name = %#v, want waveplan pop", (*requests)[0].Body["name"])
	}
	if (*requests)[2].Body["name"] != "waveplan start_review" {
		t.Fatalf("trace start[2] name = %#v, want waveplan start_review", (*requests)[2].Body["name"])
	}
	if (*requests)[4].Body["name"] != "waveplan end_review" {
		t.Fatalf("trace start[4] name = %#v, want waveplan end_review", (*requests)[4].Body["name"])
	}
	if (*requests)[6].Body["name"] != "waveplan fin" {
		t.Fatalf("trace start[6] name = %#v, want waveplan fin", (*requests)[6].Body["name"])
	}
}

// TestReloadStateMergesTail verifies that reloadState merges Tail entries from
// the on-disk state file into in-memory state, like Taken/Completed.
func TestReloadStateMergesTail(t *testing.T) {
	planJSON := []byte(`{
		"units": {
			"T1.1": {"task": "T1", "title": "Task 1", "kind": "impl", "wave": 1, "plan_line": 10, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []}
		},
		"tasks": {"T1": {"plan_line": 10}},
		"doc_index": {},
		"fp_index": {}
	}`)
	srv := makeTestServer(t, planJSON)

	// Point server at a real state file so reloadState/saveState are not no-ops.
	dir := t.TempDir()
	srv.statePath = dir + "/state.json"

	// Simulate another process having recorded a tail entry on disk.
	diskState := WaveplanState{
		Plan:      "test-plan.json",
		Taken:     map[string]TaskEntry{},
		Completed: map[string]TaskEntry{},
		Tail: map[string]TaskEntry{
			"T1.1": {TakenBy: "external", StartedAt: "2026-05-04 10:00", FinishedAt: "2026-05-04 11:00"},
		},
	}
	data, err := json.MarshalIndent(diskState, "", "  ")
	if err != nil {
		t.Fatalf("marshal disk state: %v", err)
	}
	if err := os.WriteFile(srv.statePath, data, 0644); err != nil {
		t.Fatalf("write disk state: %v", err)
	}

	if err := srv.reloadState(); err != nil {
		t.Fatalf("reloadState: %v", err)
	}

	tailEntry, ok := srv.state.Tail["T1.1"]
	if !ok {
		t.Fatal("T1.1 should be merged into in-memory Tail from disk")
	}
	if tailEntry.TakenBy != "external" {
		t.Errorf("tail taken_by should be 'external', got '%s'", tailEntry.TakenBy)
	}
	if tailEntry.FinishedAt != "2026-05-04 11:00" {
		t.Errorf("tail finished_at should be '2026-05-04 11:00', got '%s'", tailEntry.FinishedAt)
	}
}

// TestHandleGetDeterministic verifies that handleGet returns tasks in sorted order.
func TestHandleGetDeterministic(t *testing.T) {
	planJSON := []byte(`{
		"units": {
			"T3.1": {"task": "T3", "title": "Task 3", "kind": "test", "wave": 3, "plan_line": 30, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []},
			"T1.1": {"task": "T1", "title": "Task 1", "kind": "impl", "wave": 1, "plan_line": 10, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []},
			"T2.1": {"task": "T2", "title": "Task 2", "kind": "impl", "wave": 2, "plan_line": 20, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []}
		},
		"tasks": {"T1": {"plan_line": 10}, "T2": {"plan_line": 20}, "T3": {"plan_line": 30}},
		"doc_index": {},
		"fp_index": {}
	}`)
	srv := makeTestServer(t, planJSON)

	// Manually set up state: T1.1 and T3.1 are taken, T2.1 is not
	srv.state.Taken["T1.1"] = TaskEntry{TakenBy: "psi", StartedAt: "2026-01-01 00:00"}
	srv.state.Taken["T3.1"] = TaskEntry{TakenBy: "psi", StartedAt: "2026-01-01 00:01"}

	// Test "taken" mode — should be sorted
	takenReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"mode": "taken"},
		},
	}
	takenResult, err := srv.handleGet(nil, takenReq)
	if err != nil {
		t.Fatalf("handleGet(taken) returned error: %v", err)
	}
	takenText := takenResult.Content[0].(mcp.TextContent).Text
	takenJSON := parseJSONMap(t, takenText)
	takenTasksAny := takenJSON["tasks"].([]any)
	takenTasks := make([]map[string]any, len(takenTasksAny))
	for i, v := range takenTasksAny {
		takenTasks[i] = v.(map[string]any)
	}
	if len(takenTasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(takenTasks))
	}
	if takenTasks[0]["task_id"] != "T1.1" {
		t.Errorf("first taken task should be T1.1 (sorted), got '%s'", takenTasks[0]["task_id"])
	}
	if takenTasks[1]["task_id"] != "T3.1" {
		t.Errorf("second taken task should be T3.1 (sorted), got '%s'", takenTasks[1]["task_id"])
	}

	// Test "all" mode — should include completed tasks sorted
	// Fin T1.1 so it's in completed
	finArgs := map[string]any{"task_id": "T1.1", "git_sha": "aaa"}
	finReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: finArgs,
		},
	}
	_, err = srv.handleFin(nil, finReq)
	if err != nil {
		t.Fatalf("handleFin(T1.1) returned error: %v", err)
	}

	allReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"mode": "all"},
		},
	}
	allResult, err := srv.handleGet(nil, allReq)
	if err != nil {
		t.Fatalf("handleGet(all) returned error: %v", err)
	}
	allText := allResult.Content[0].(mcp.TextContent).Text
	allJSON := parseJSONMap(t, allText)
	allTasksAny := allJSON["tasks"].([]any)
	allTasks := make([]map[string]any, len(allTasksAny))
	for i, v := range allTasksAny {
		allTasks[i] = v.(map[string]any)
	}
	if len(allTasks) != 2 {
		t.Fatalf("expected 2 tasks in all, got %d", len(allTasks))
	}
	// T1.1 and T3.1 should be sorted
	if allTasks[0]["task_id"] != "T1.1" {
		t.Errorf("first all task should be T1.1, got '%s'", allTasks[0]["task_id"])
	}
	if allTasks[1]["task_id"] != "T3.1" {
		t.Errorf("second all task should be T3.1, got '%s'", allTasks[1]["task_id"])
	}
}

// TestHandleGetDeptreeOrdering verifies deptree mode returns topologically sorted tasks with groups.
func TestHandleGetDeptreeOrdering(t *testing.T) {
	planJSON := []byte(`{
		"units": {
			"T3.1": {"task": "T3", "title": "Task 3", "kind": "test", "wave": 3, "plan_line": 30, "depends_on": ["T1.1"], "doc_refs": [], "fp_refs": [], "notes": []},
			"T1.1": {"task": "T1", "title": "Task 1", "kind": "impl", "wave": 1, "plan_line": 10, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []},
			"T2.1": {"task": "T2", "title": "Task 2", "kind": "impl", "wave": 2, "plan_line": 20, "depends_on": [], "doc_refs": [], "fp_refs": [], "notes": []}
		},
		"tasks": {"T1": {"plan_line": 10}, "T2": {"plan_line": 20}, "T3": {"plan_line": 30}},
		"doc_index": {},
		"fp_index": {}
	}`)
	srv := makeTestServer(t, planJSON)

	deptreeReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"mode": "deptree"},
		},
	}
	deptreeResult, err := srv.handleGet(nil, deptreeReq)
	if err != nil {
		t.Fatalf("handleGet(deptree) returned error: %v", err)
	}
	deptreeText := deptreeResult.Content[0].(mcp.TextContent).Text
	deptreeJSON := parseJSONMap(t, deptreeText)
	deptreeTasksAny := deptreeJSON["tasks"].([]any)
	deptreeTasks := make([]map[string]any, len(deptreeTasksAny))
	for i, v := range deptreeTasksAny {
		deptreeTasks[i] = v.(map[string]any)
	}
	if len(deptreeTasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(deptreeTasks))
	}
	// T1.1 and T2.1 should be group 1 (no deps), T3.1 should be group 2 (depends on T1.1)
	if deptreeTasks[0]["task_id"] != "T1.1" {
		t.Errorf("first task should be T1.1, got '%s'", deptreeTasks[0]["task_id"])
	}
	// JSON unmarshals numbers as float64
	g1, ok := deptreeTasks[0]["group"].(float64)
	if !ok || g1 != 1 {
		t.Errorf("T1.1 should be group 1, got %v", deptreeTasks[0]["group"])
	}
	// T3.1 depends on T1.1, should be group 2
	for _, task := range deptreeTasks {
		if task["task_id"] == "T3.1" {
			g, ok := task["group"].(float64)
			if !ok || g != 2 {
				t.Errorf("T3.1 should be group 2, got %v", task["group"])
			}
		}
	}
}

// TestGetUsesPlanNotPlanRef verifies get returns "plan" key (not "plan_ref").
func TestGetUsesPlanNotPlanRef(t *testing.T) {
	planJSON := []byte(`{
		"units": {
			"T1.1": {"task": "T1", "title": "Task 1", "kind": "impl", "wave": 1, "plan_line": 20, "depends_on": [], "doc_refs": ["plan"], "fp_refs": [], "notes": []}
		},
		"tasks": {"T1": {"plan_line": 20}},
		"doc_index": {"plan": {"path": "docs/superpowers/plans/test.json", "line": 20, "kind": "plan"}},
		"fp_index": {}
	}`)
	srv := makeTestServer(t, planJSON)

	// peek should return plan_ref
	peekReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{},
		},
	}
	peekResult, err := srv.handlePeek(nil, peekReq)
	if err != nil {
		t.Fatalf("handlePeek returned error: %v", err)
	}
	peekJSON := parseJSONMap(t, peekResult.Content[0].(mcp.TextContent).Text)
	if _, hasPlanRef := peekJSON["plan_ref"]; !hasPlanRef {
		t.Error("peek should return plan_ref")
	}
	if _, hasPlan := peekJSON["plan"]; hasPlan {
		t.Error("peek should not return plan")
	}

	// get should return plan (not plan_ref)
	getReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{"mode": "open"},
		},
	}
	getResult, err := srv.handleGet(nil, getReq)
	if err != nil {
		t.Fatalf("handleGet returned error: %v", err)
	}
	getJSON := parseJSONMap(t, getResult.Content[0].(mcp.TextContent).Text)
	getTasks := getJSON["tasks"].([]any)
	if len(getTasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(getTasks))
	}
	task := getTasks[0].(map[string]any)
	if _, hasPlan := task["plan"]; !hasPlan {
		t.Error("get should return plan")
	}
	if _, hasPlanRef := task["plan_ref"]; hasPlanRef {
		t.Error("get should not return plan_ref")
	}
}
