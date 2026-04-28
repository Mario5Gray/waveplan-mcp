package main

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

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
	}
	return &WaveplanServer{
		plan:     &plan,
		state:    state,
		planPath: "test-plan.json",
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