# Waveplan MCP — Feature Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the Go MCP service (`waveplan-mcp/main.go`) to feature parity with the Python CLI (`../scripts/waveplan`).

**Architecture:** Single Go file (`main.go`) — expand data structures, add helper functions, convert all handlers to return protocol-native structured MCP results, add `deptree` mode, add `git_sha`/`review_note` parameters.

**Tech Stack:** Go 1.26.1+, `github.com/mark3labs/mcp-go` v0.49.0, stdlib only.

**Implementation note:** Prefer `mcp.NewToolResultJSON(...)` for successful tool responses. The library emits both text fallback and `structuredContent`, which matches the “structured JSON” objective better than `json.Marshal(...)` + `mcp.NewToolResultText(...)`.

---

## File Structure

| File | Responsibility |
|------|----------------|
| `.main.go` | All changes — data structures, helpers, handlers, tools |
| `.main_test.go` | New test file for helper functions and handler logic |

---

### Task 1: Add new fields to `TaskEntry`

**Files:**
- Modify: `.main.go:27-34`

- [ ] **Step 1: Add `ReviewNote` and `GitSha` fields to `TaskEntry`**

Replace the existing `TaskEntry` struct (lines 27-34):

```go
// TaskEntry represents a task's state entry
type TaskEntry struct {
	TakenBy         string `json:"taken_by"`
	StartedAt       string `json:"started_at"`
	ReviewEnteredAt string `json:"review_entered_at"`
	ReviewEndedAt   string `json:"review_ended_at"`
	Reviewer        string `json:"reviewer"`
	ReviewNote      string `json:"review_note,omitempty"`
	GitSha          string `json:"git_sha,omitempty"`
	FinishedAt      string `json:"finished_at"`
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd . && go build`
Expected: clean build, no errors.

- [ ] **Step 3: Commit**

```bash
cd .
git add main.go
git commit -m "feat(waveplan-mcp): add ReviewNote and GitSha fields to TaskEntry"
```

---

### Task 2: Implement helper functions

**Files:**
- Modify: `.main.go` (add after `nextAvailableTask`, before `createTools`)

- [ ] **Step 1: Add `findPlanRef` helper**

Add after line 163 (after `nextAvailableTask`):

```go
// findPlanRef resolves a task to its plan document reference.
// For sub-tasks (e.g. T1.1), the task_id maps to a parent task via the
// 'task' field (e.g. T1.1.task == 'T1'). We find the parent task's
// plan_line, then match it against doc_index entries with kind='plan'.
func (s *WaveplanServer) findPlanRef(taskID string) map[string]any {
	unit := s.plan.Units[taskID]
	parentTask := unit.Task
	if parentTask == "" {
		parentTask = taskID
	}
	parent, ok := s.plan.Tasks[parentTask]
	if !ok {
		return nil
	}
	parentMap, ok := parent.(map[string]any)
	if !ok {
		return nil
	}
	planLineVal, ok := parentMap["plan_line"]
	if !ok {
		return nil
	}
	planLine, ok := planLineVal.(float64)
	if !ok {
		return nil
	}
	for ref, entry := range s.plan.DocIndex {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if entryMap["kind"] == "plan" {
			lineVal, ok := entryMap["line"]
			if !ok {
				continue
			}
			line, ok := lineVal.(float64)
			if !ok {
				continue
			}
			if int(line) == int(planLine) {
				return map[string]any{
					"ref":  ref,
					"path": entryMap["path"],
					"line": int(line),
				}
			}
		}
	}
	return nil
}

// resolveDocRefs resolves a list of ref names to their full index entries.
func (s *WaveplanServer) resolveDocRefs(refNames []string) []map[string]any {
	result := make([]map[string]any, 0, len(refNames))
	for _, ref := range refNames {
		entry, ok := s.plan.DocIndex[ref]
		if !ok {
			result = append(result, map[string]any{"ref": ref})
			continue
		}
		entryMap, ok := entry.(map[string]any)
		if !ok {
			result = append(result, map[string]any{"ref": ref})
			continue
		}
		out := make(map[string]any, len(entryMap)+1)
		out["ref"] = ref
		for k, v := range entryMap {
			out[k] = v
		}
		result = append(result, out)
	}
	return result
}

// resolveFpRefs resolves fp_refs to full entries.
func (s *WaveplanServer) resolveFpRefs(refNames []string) []map[string]any {
	result := make([]map[string]any, 0, len(refNames))
	for _, ref := range refNames {
		fpID := ref
		if val, ok := s.plan.FpIndex[ref]; ok {
			fpID = val
		}
		result = append(result, map[string]any{
			"ref":   ref,
			"fp_id": fpID,
		})
	}
	return result
}

// taskInfo builds a complete task info map for a given task ID.
func (s *WaveplanServer) taskInfo(taskID string) map[string]any {
	unit := s.plan.Units[taskID]
	taken := s.state.Taken[taskID]
	completed := s.state.Completed[taskID]

	// Determine status
	var status string
	if _, ok := s.state.Completed[taskID]; ok {
		status = "completed"
	} else if _, ok := s.state.Taken[taskID]; ok {
		status = "taken"
	} else {
		status = "available"
	}

	started := taken.StartedAt
	if started == "" {
		started = completed.StartedAt
	}
	finished := completed.FinishedAt
	agent := taken.TakenBy
	if agent == "" {
		agent = completed.TakenBy
	}
	reviewEntered := taken.ReviewEnteredAt
	if reviewEntered == "" {
		reviewEntered = completed.ReviewEnteredAt
	}
	reviewEnded := taken.ReviewEndedAt
	if reviewEnded == "" {
		reviewEnded = completed.ReviewEndedAt
	}
	reviewer := taken.Reviewer
	if reviewer == "" {
		reviewer = completed.Reviewer
	}
	reviewNote := taken.ReviewNote
	if reviewNote == "" {
		reviewNote = completed.ReviewNote
	}
	gitSha := completed.GitSha

	info := map[string]any{
		"task_id":           taskID,
		"title":             unit.Title,
		"task":              unit.Task,
		"kind":              unit.Kind,
		"wave":              unit.Wave,
		"plan_line":         unit.PlanLine,
		"depends_on":        unit.DependsOn,
		"status":            status,
		"started_at":        nilIfEmpty(started),
		"finished_at":       nilIfEmpty(finished),
		"taken_by":          nilIfEmpty(agent),
		"review_entered_at": nilIfEmpty(reviewEntered),
		"review_ended_at":   nilIfEmpty(reviewEnded),
		"reviewer":          nilIfEmpty(reviewer),
		"review_note":       nilIfEmpty(reviewNote),
		"git_sha":           nilIfEmpty(gitSha),
	}

	if planRef := s.findPlanRef(taskID); planRef != nil {
		info["plan"] = map[string]any{
			"path": planRef["path"],
			"line": planRef["line"],
		}
	}

	if len(unit.DocRefs) > 0 {
		info["doc_refs"] = s.resolveDocRefs(unit.DocRefs)
	}
	if len(unit.FpRefs) > 0 {
		info["fp_refs"] = s.resolveFpRefs(unit.FpRefs)
	}

	return info
}

// nilIfEmpty returns nil for empty strings, useful for JSON null output.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
```

- [ ] **Step 2: Add `topologicalSort` helper**

Add after `taskInfo`:

```go
// topologicalSort returns task IDs in dependency order with parallel group numbers.
// Tasks with no dependencies are group 1, tasks whose dependencies are
// all in group N are group N+1, etc. Within the same group, tasks are
// sorted by wave number then task ID for determinism.
func (s *WaveplanServer) topologicalSort() []map[string]any {
	// Build in-degree map
	inDegree := make(map[string]int)
	for tid := range s.plan.Units {
		inDegree[tid] = 0
	}
	for tid, unit := range s.plan.Units {
		for _, dep := range unit.DependsOn {
			if _, ok := inDegree[dep]; ok {
				inDegree[tid]++
			}
		}
	}

	// Start with tasks that have no dependencies (group 1)
	var queue []string
	for tid, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, tid)
		}
	}
	sort.Slice(queue, func(i, j int) bool {
		wi := s.plan.Units[queue[i]].Wave
		wj := s.plan.Units[queue[j]].Wave
		if wi != wj {
			return wi < wj
		}
		return queue[i] < queue[j]
	})

	var result []map[string]any
	group := 1

	for len(queue) > 0 {
		nextQueue := []string{}
		for _, tid := range queue {
			info := s.taskInfo(tid)
			info["group"] = group
			result = append(result, info)
			// Reduce in-degree for dependents
			for otherTid, otherUnit := range s.plan.Units {
				for _, dep := range otherUnit.DependsOn {
					if dep == tid {
						inDegree[otherTid]--
						if inDegree[otherTid] == 0 {
							nextQueue = append(nextQueue, otherTid)
						}
					}
				}
			}
		}
		sort.Slice(nextQueue, func(i, j int) bool {
			wi := s.plan.Units[nextQueue[i]].Wave
			wj := s.plan.Units[nextQueue[j]].Wave
			if wi != wj {
				return wi < wj
			}
			return nextQueue[i] < nextQueue[j]
		})
		queue = nextQueue
		group++
	}

	return result
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd . && go build`
Expected: clean build.

- [ ] **Step 4: Commit**

```bash
cd .
git add main.go
git commit -m "feat(waveplan-mcp): add helper functions for JSON output and topological sort"
```

---

### Task 3: Convert `handlePeek` to JSON output

**Files:**
- Modify: `.main.go:222-238`

- [ ] **Step 1: Replace `handlePeek` handler**

Replace lines 222-238:

```go
func (s *WaveplanServer) handlePeek(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID := s.nextAvailableTask()
	if taskID == "" {
		return mcp.NewToolResultJSON(map[string]any{"error": "No available tasks."})
	}
	info := s.taskInfo(taskID)
	if unit := s.plan.Units[taskID]; unit.Command != "" {
		info["command"] = unit.Command
	}
	return mcp.NewToolResultJSON(info)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd . && go build`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
cd .
git add main.go
git commit -m "feat(waveplan-mcp): convert handlePeek to return JSON"
```

---

### Task 4: Convert `handlePop` to JSON output

**Files:**
- Modify: `.main.go:240-268`

- [ ] **Step 1: Replace `handlePop` handler**

Replace lines 240-268:

```go
func (s *WaveplanServer) handlePop(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, err := requiredStringParam(request.Params.Arguments, "agent")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	taskID := s.nextAvailableTask()
	if taskID == "" {
		return mcp.NewToolResultJSON(map[string]any{"error": "No available tasks."})
	}
	ts := nowStr()
	s.state.Taken[taskID] = TaskEntry{
		TakenBy:   agent,
		StartedAt: ts,
	}
	if err := s.saveState(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to save state: %v", err)), nil
	}
	info := s.taskInfo(taskID)
	info["claimed"] = true
	info["taken_by"] = agent
	info["started_at"] = ts
	if unit := s.plan.Units[taskID]; unit.Command != "" {
		info["command"] = unit.Command
	}
	return mcp.NewToolResultJSON(info)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd . && go build`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
cd .
git add main.go
git commit -m "feat(waveplan-mcp): convert handlePop to return JSON"
```

---

### Task 5: Convert `handleStartReview` to JSON output

**Files:**
- Modify: `.main.go:270-304`

- [ ] **Step 1: Replace `handleStartReview` handler**

Replace lines 270-304:

```go
func (s *WaveplanServer) handleStartReview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID, err := requiredStringParam(request.Params.Arguments, "task_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	reviewer, err := requiredStringParam(request.Params.Arguments, "reviewer")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if _, ok := s.plan.Units[taskID]; !ok {
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' not found in plan", taskID)), nil
	}
	taken, ok := s.state.Taken[taskID]
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' is not currently taken", taskID)), nil
	}
	if taken.ReviewEnteredAt != "" {
		return mcp.NewToolResultError(fmt.Sprintf("review for %s already started", taskID)), nil
	}
	ts := nowStr()
	s.state.Taken[taskID] = TaskEntry{
		TakenBy:         taken.TakenBy,
		StartedAt:       taken.StartedAt,
		ReviewEnteredAt: ts,
		Reviewer:        reviewer,
		ReviewNote:      taken.ReviewNote,
		GitSha:          taken.GitSha,
	}
	if err := s.saveState(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to save state: %v", err)), nil
	}
	info := map[string]any{
		"success":            true,
		"task_id":            taskID,
		"title":              s.plan.Units[taskID].Title,
		"reviewer":           reviewer,
		"review_entered_at":  ts,
	}
	return mcp.NewToolResultJSON(info)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd . && go build`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
cd .
git add main.go
git commit -m "feat(waveplan-mcp): convert handleStartReview to return JSON"
```

---

### Task 6: Convert `handleEndReview` to JSON output + add `review_note` parameter

**Files:**
- Modify: `.main.go:306-337`

- [ ] **Step 1: Update tool definition for `waveplan_end_review`**

Replace the tool definition (lines 189-195):

```go
{
	Tool: mcp.NewTool("waveplan_end_review",
		mcp.WithDescription("End a review for a task"),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to end review for")),
		mcp.WithString("review_note", mcp.Description("Optional review note")),
	),
	Handler: s.handleEndReview,
},
```

- [ ] **Step 2: Replace `handleEndReview` handler**

Replace lines 306-337:

```go
func (s *WaveplanServer) handleEndReview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID, err := requiredStringParam(request.Params.Arguments, "task_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	reviewNote, _ := optionalStringParam(request.Params.Arguments, "review_note")
	if _, ok := s.plan.Units[taskID]; !ok {
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' not found in plan", taskID)), nil
	}
	taken, ok := s.state.Taken[taskID]
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' is not currently taken", taskID)), nil
	}
	if taken.ReviewEnteredAt == "" {
		return mcp.NewToolResultError(fmt.Sprintf("no active review for %s", taskID)), nil
	}
	ts := nowStr()
	s.state.Taken[taskID] = TaskEntry{
		TakenBy:         taken.TakenBy,
		StartedAt:       taken.StartedAt,
		ReviewEnteredAt: taken.ReviewEnteredAt,
		ReviewEndedAt:   ts,
		Reviewer:        taken.Reviewer,
		ReviewNote:      reviewNote,
		GitSha:          taken.GitSha,
	}
	if err := s.saveState(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to save state: %v", err)), nil
	}
	info := map[string]any{
		"success":         true,
		"task_id":         taskID,
		"title":           s.plan.Units[taskID].Title,
		"reviewer":        taken.Reviewer,
		"review_ended_at": ts,
	}
	if reviewNote != "" {
		info["review_note"] = reviewNote
	}
	return mcp.NewToolResultJSON(info)
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd . && go build`
Expected: clean build.

- [ ] **Step 4: Commit**

```bash
cd .
git add main.go
git commit -m "feat(waveplan-mcp): convert handleEndReview to JSON, add review_note param"
```

---

### Task 7: Convert `handleFin` to JSON output + add `git_sha` parameter

**Files:**
- Modify: `.main.go:196-202` (tool def) and 339-379 (handler)

- [ ] **Step 1: Update tool definition for `waveplan_fin`**

Replace lines 196-202:

```go
{
	Tool: mcp.NewTool("waveplan_fin",
		mcp.WithDescription("Mark a task as completed"),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to complete")),
		mcp.WithString("git_sha", mcp.Description("Optional git SHA of the commit")),
	),
	Handler: s.handleFin,
},
```

- [ ] **Step 2: Replace `handleFin` handler**

Replace lines 339-379:

```go
func (s *WaveplanServer) handleFin(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID, err := requiredStringParam(request.Params.Arguments, "task_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	gitSha, _ := optionalStringParam(request.Params.Arguments, "git_sha")
	if _, ok := s.plan.Units[taskID]; !ok {
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' not found in plan", taskID)), nil
	}
	if _, ok := s.state.Completed[taskID]; ok {
		return mcp.NewToolResultError(fmt.Sprintf("task %s is already completed", taskID)), nil
	}
	unit := s.plan.Units[taskID]
	for _, dep := range unit.DependsOn {
		if _, ok := s.state.Completed[dep]; !ok {
			return mcp.NewToolResultError(fmt.Sprintf("task %s has incomplete dependencies: %s", taskID, strings.Join(unit.DependsOn, ", "))), nil
		}
	}
	taken := s.state.Taken[taskID]
	reviewEntered := taken.ReviewEnteredAt
	reviewEnded := taken.ReviewEndedAt
	if reviewEntered != "" && reviewEnded == "" {
		return mcp.NewToolResultError(fmt.Sprintf("task %s has an active review. End review before completing.", taskID)), nil
	}
	ts := nowStr()
	s.state.Completed[taskID] = TaskEntry{
		TakenBy:         taken.TakenBy,
		StartedAt:       taken.StartedAt,
		ReviewEnteredAt: reviewEntered,
		ReviewEndedAt:   reviewEnded,
		Reviewer:        taken.Reviewer,
		ReviewNote:      taken.ReviewNote,
		GitSha:          gitSha,
		FinishedAt:      ts,
	}
	delete(s.state.Taken, taskID)
	if err := s.saveState(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to save state: %v", err)), nil
	}
	info := map[string]any{
		"success":      true,
		"task_id":      taskID,
		"title":        unit.Title,
		"finished_at":  ts,
	}
	if gitSha != "" {
		info["git_sha"] = gitSha
	}
	return mcp.NewToolResultJSON(info)
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd . && go build`
Expected: clean build.

- [ ] **Step 4: Commit**

```bash
cd .
git add main.go
git commit -m "feat(waveplan-mcp): convert handleFin to JSON, add git_sha param"
```

---

### Task 8: Convert `handleGet` to JSON output + add `deptree` mode

**Files:**
- Modify: `.main.go:381-500`

- [ ] **Step 1: Replace `handleGet` handler**

Replace lines 381-500 entirely:

```go
func (s *WaveplanServer) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mode, _ := optionalStringParam(request.Params.Arguments, "mode")
	if mode == "" {
		mode = "all"
	}
	var taskIDs []string
	var deptreeSorted []map[string]any
	switch mode {
	case "taken":
		for tid := range s.state.Taken {
			taskIDs = append(taskIDs, tid)
		}
		sort.Strings(taskIDs)
	case "open":
		for tid := range s.plan.Units {
			if s.isAvailable(tid) {
				taskIDs = append(taskIDs, tid)
			}
		}
		sort.Strings(taskIDs)
	case "complete":
		for tid := range s.state.Completed {
			taskIDs = append(taskIDs, tid)
		}
		sort.Strings(taskIDs)
	case "deptree":
		deptreeSorted = s.topologicalSort()
		taskIDs = make([]string, len(deptreeSorted))
		for i, info := range deptreeSorted {
			taskIDs[i] = info["task_id"].(string)
		}
	case "all":
		seen := make(map[string]struct{})
		for tid := range s.state.Taken {
			seen[tid] = struct{}{}
		}
		for tid := range s.state.Completed {
			seen[tid] = struct{}{}
		}
		for tid := range seen {
			taskIDs = append(taskIDs, tid)
		}
		sort.Strings(taskIDs)
	default:
		if strings.HasPrefix(mode, "task-") {
			tid := mode[5:]
			if _, ok := s.plan.Units[tid]; !ok {
				return mcp.NewToolResultError(fmt.Sprintf("task '%s' not found in plan", tid)), nil
			}
			taskIDs = []string{tid}
		} else {
			// Agent filter
			seen := make(map[string]struct{})
			for tid, entry := range s.state.Taken {
				if entry.TakenBy == mode {
					seen[tid] = struct{}{}
				}
			}
			for tid, entry := range s.state.Completed {
				if entry.TakenBy == mode {
					seen[tid] = struct{}{}
				}
			}
			for tid := range seen {
				taskIDs = append(taskIDs, tid)
			}
			sort.Strings(taskIDs)
		}
	}
	if len(taskIDs) == 0 {
		return mcp.NewToolResultJSON(map[string]any{"tasks": []map[string]any{}})
	}

	tasks := make([]map[string]any, 0, len(taskIDs))
	for _, tid := range taskIDs {
		info := s.taskInfo(tid)
		if mode == "deptree" {
			// Add group info from pre-computed topological sort
			for _, si := range deptreeSorted {
				if si["task_id"] == tid {
					info["group"] = si["group"]
					break
				}
			}
		}
		tasks = append(tasks, info)
	}
	result := map[string]any{"tasks": tasks}
	return mcp.NewToolResultJSON(result)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd . && go build`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
cd .
git add main.go
git commit -m "feat(waveplan-mcp): convert handleGet to JSON, add deptree mode"
```

---

### Task 9: Convert `handleListPlans` to JSON output

**Files:**
- Modify: `.main.go:502-519`

- [ ] **Step 1: Replace `handleListPlans` handler**

Replace lines 502-519:

```go
func (s *WaveplanServer) handleListPlans(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	planDir, _ := optionalStringParam(request.Params.Arguments, "plan_dir")
	if planDir == "" {
		planDir = "docs/plans"
	}
	matches, err := filepath.Glob(filepath.Join(planDir, "*-execution-waves.json"))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list plans: %v", err)), nil
	}
	result := map[string]any{"plans": matches}
	return mcp.NewToolResultJSON(result)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd . && go build`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
cd .
git add main.go
git commit -m "feat(waveplan-mcp): convert handleListPlans to return JSON"
```

---

### Task 10: Write tests

**Files:**
- Create: `.main_test.go`

- [ ] **Step 1: Create test file with helper function tests**

```go
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func createTestPlan(t *testing.T, units map[string]PlanUnit, tasks map[string]any, docIndex map[string]any, fpIndex map[string]string) (string, string) {
	t.Helper()
	dir := t.TempDir()

	plan := WaveplanPlan{
		Units:    units,
		Tasks:    tasks,
		DocIndex: docIndex,
		FpIndex:  fpIndex,
	}

	planPath := filepath.Join(dir, "test-plan.json")
	planData, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal plan: %v", err)
	}
	if err := os.WriteFile(planPath, planData, 0644); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}

	statePath := filepath.Join(dir, "test-plan.json.state.json")
	state := WaveplanState{
		Plan:      "test-plan.json",
		Taken:     make(map[string]TaskEntry),
		Completed: make(map[string]TaskEntry),
	}
	stateData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal state: %v", err)
	}
	if err := os.WriteFile(statePath, stateData, 0644); err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	return planPath, statePath
}

func TestFindPlanRef(t *testing.T) {
	units := map[string]PlanUnit{
		"T1.1": {Task: "T1", Title: "Sub task 1", Kind: "test", Wave: 1, PlanLine: 42, DependsOn: []string{}},
	}
	tasks := map[string]any{
		"T1": map[string]any{"plan_line": float64(42)},
	}
	docIndex := map[string]any{
		"plan1": map[string]any{"kind": "plan", "path": "docs/plan1.md", "line": float64(42)},
		"plan2": map[string]any{"kind": "plan", "path": "docs/plan2.md", "line": float64(100)},
	}
	_, statePath := createTestPlan(t, units, tasks, docIndex, nil)
	planPath := filepath.Dir(statePath) + "/test-plan.json"

	srv, err := NewWaveplanServer(planPath, statePath)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ref := srv.findPlanRef("T1.1")
	if ref == nil {
		t.Fatal("expected plan ref for T1.1")
	}
	if ref["path"] != "docs/plan1.md" {
		t.Errorf("expected path docs/plan1.md, got %v", ref["path"])
	}
	if ref["line"] != 42 {
		t.Errorf("expected line 42, got %v", ref["line"])
	}
}

func TestTopologicalSort(t *testing.T) {
	units := map[string]PlanUnit{
		"T1.1": {Task: "T1", Title: "No deps", Kind: "test", Wave: 1, PlanLine: 1, DependsOn: []string{}},
		"T1.2": {Task: "T1", Title: "Depends on T1.1", Kind: "test", Wave: 2, PlanLine: 1, DependsOn: []string{"T1.1"}},
		"T2.1": {Task: "T2", Title: "No deps", Kind: "test", Wave: 1, PlanLine: 2, DependsOn: []string{}},
		"T2.2": {Task: "T2", Title: "Depends on T2.1", Kind: "test", Wave: 2, PlanLine: 2, DependsOn: []string{"T2.1"}},
	}
	_, statePath := createTestPlan(t, units, nil, nil, nil)
	planPath := filepath.Dir(statePath) + "/test-plan.json"

	srv, err := NewWaveplanServer(planPath, statePath)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	sorted := srv.topologicalSort()
	if len(sorted) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(sorted))
	}

	// Group 1: T1.1 and T2.1 (no deps, wave 1)
	if sorted[0]["task_id"] != "T1.1" || sorted[0]["group"] != 1 {
		t.Errorf("expected T1.1 group 1, got %v", sorted[0])
	}
	if sorted[1]["task_id"] != "T2.1" || sorted[1]["group"] != 1 {
		t.Errorf("expected T2.1 group 1, got %v", sorted[1])
	}

	// Group 2: T1.2 and T2.2 (depend on group 1)
	if sorted[2]["task_id"] != "T1.2" || sorted[2]["group"] != 2 {
		t.Errorf("expected T1.2 group 2, got %v", sorted[2])
	}
	if sorted[3]["task_id"] != "T2.2" || sorted[3]["group"] != 2 {
		t.Errorf("expected T2.2 group 2, got %v", sorted[3])
	}
}

func TestResolveDocRefs(t *testing.T) {
	units := map[string]PlanUnit{
		"T1.1": {Task: "T1", Title: "Test", Kind: "test", Wave: 1, PlanLine: 1, DependsOn: []string{}, DocRefs: []string{"plan1", "plan2"}},
	}
	docIndex := map[string]any{
		"plan1": map[string]any{"kind": "plan", "path": "docs/plan1.md", "line": float64(10)},
		"plan2": map[string]any{"kind": "plan", "path": "docs/plan2.md", "line": float64(20)},
	}
	_, statePath := createTestPlan(t, units, nil, docIndex, nil)
	planPath := filepath.Dir(statePath) + "/test-plan.json"

	srv, err := NewWaveplanServer(planPath, statePath)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	resolved := srv.resolveDocRefs([]string{"plan1", "plan2"})
	if len(resolved) != 2 {
		t.Fatalf("expected 2 resolved refs, got %d", len(resolved))
	}
	if resolved[0]["path"] != "docs/plan1.md" {
		t.Errorf("expected docs/plan1.md, got %v", resolved[0]["path"])
	}
	if resolved[1]["path"] != "docs/plan2.md" {
		t.Errorf("expected docs/plan2.md, got %v", resolved[1]["path"])
	}
}

func TestResolveFpRefs(t *testing.T) {
	units := map[string]PlanUnit{
		"T1.1": {Task: "T1", Title: "Test", Kind: "test", Wave: 1, PlanLine: 1, DependsOn: []string{}, FpRefs: []string{"T1.1"}},
	}
	fpIndex := map[string]string{"T1.1": "FP-42"}
	_, statePath := createTestPlan(t, units, nil, nil, fpIndex)
	planPath := filepath.Dir(statePath) + "/test-plan.json"

	srv, err := NewWaveplanServer(planPath, statePath)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	resolved := srv.resolveFpRefs([]string{"T1.1"})
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved ref, got %d", len(resolved))
	}
	if resolved[0]["fp_id"] != "FP-42" {
		t.Errorf("expected FP-42, got %v", resolved[0]["fp_id"])
	}
}

func TestNilIfEmpty(t *testing.T) {
	if nilIfEmpty("") != nil {
		t.Error("nilIfEmpty(\"\") should return nil")
	}
	s := "hello"
	if nilIfEmpty(s) == nil || *nilIfEmpty(s) != "hello" {
		t.Error("nilIfEmpty(\"hello\") should return &\"hello\"")
	}
}

func TestTaskInfo(t *testing.T) {
	units := map[string]PlanUnit{
		"T1.1": {Task: "T1", Title: "Test task", Kind: "test", Wave: 1, PlanLine: 42, DependsOn: []string{}, DocRefs: []string{"plan1"}, FpRefs: []string{"T1.1"}},
	}
	tasks := map[string]any{"T1": map[string]any{"plan_line": float64(42)}}
	docIndex := map[string]any{"plan1": map[string]any{"kind": "plan", "path": "docs/plan1.md", "line": float64(42)}}
	fpIndex := map[string]string{"T1.1": "FP-42"}
	_, statePath := createTestPlan(t, units, tasks, docIndex, fpIndex)
	planPath := filepath.Dir(statePath) + "/test-plan.json"

	srv, err := NewWaveplanServer(planPath, statePath)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Mark task as taken
	srv.state.Taken["T1.1"] = TaskEntry{TakenBy: "psi", StartedAt: "2026-04-25 14:00"}

	info := srv.taskInfo("T1.1")
	if info["status"] != "taken" {
		t.Errorf("expected status 'taken', got %v", info["status"])
	}
	takenBy, ok := info["taken_by"].(*string)
	if !ok || takenBy == nil || *takenBy != "psi" {
		t.Errorf("expected taken_by 'psi', got %v", info["taken_by"])
	}
	if info["plan"] == nil {
		t.Error("expected plan ref in task info")
	}
	if info["doc_refs"] == nil {
		t.Error("expected doc_refs in task info")
	}
	if info["fp_refs"] == nil {
		t.Error("expected fp_refs in task info")
	}
}

func TestHandleEndReviewStoresReviewNote(t *testing.T) {
	units := map[string]PlanUnit{
		"T1.1": {Task: "T1", Title: "Test task", Kind: "test", Wave: 1, PlanLine: 42, DependsOn: []string{}},
	}
	planPath, statePath := createTestPlan(t, units, nil, nil, nil)

	srv, err := NewWaveplanServer(planPath, statePath)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.state.Taken["T1.1"] = TaskEntry{
		TakenBy:         "psi",
		StartedAt:       "2026-04-25 14:00",
		ReviewEnteredAt: "2026-04-25 14:30",
		Reviewer:        "sigma",
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = "waveplan_end_review"
	req.Params.Arguments = map[string]any{
		"task_id":     "T1.1",
		"review_note": "Looks good",
	}

	if _, err := srv.handleEndReview(context.Background(), req); err != nil {
		t.Fatalf("handleEndReview returned error: %v", err)
	}
	if got := srv.state.Taken["T1.1"].ReviewNote; got != "Looks good" {
		t.Fatalf("expected review note to persist, got %q", got)
	}
}

func TestHandleFinStoresGitSha(t *testing.T) {
	units := map[string]PlanUnit{
		"T1.1": {Task: "T1", Title: "Test task", Kind: "test", Wave: 1, PlanLine: 42, DependsOn: []string{}},
	}
	planPath, statePath := createTestPlan(t, units, nil, nil, nil)

	srv, err := NewWaveplanServer(planPath, statePath)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.state.Taken["T1.1"] = TaskEntry{
		TakenBy:   "psi",
		StartedAt: "2026-04-25 14:00",
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = "waveplan_fin"
	req.Params.Arguments = map[string]any{
		"task_id": "T1.1",
		"git_sha": "abc1234",
	}

	if _, err := srv.handleFin(context.Background(), req); err != nil {
		t.Fatalf("handleFin returned error: %v", err)
	}
	if got := srv.state.Completed["T1.1"].GitSha; got != "abc1234" {
		t.Fatalf("expected git_sha to persist, got %q", got)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `cd . && go test -v ./...`
Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
cd .
git add main_test.go
git commit -m "test(waveplan-mcp): add helper and handler parity tests"
```

---

### Task 11: End-to-end verification

**Files:**
- Use: `.main.go`

- [ ] **Step 1: Build the binary**

Run: `cd . && go build -o waveplan-mcp`
Expected: binary created.

- [ ] **Step 2: Verify MCP envelope + structured payloads using an isolated temp plan/state**

Do **not** point the verification run at the real repo state file. The probes below are stateful (`pop`, `start_review`, `end_review`, `fin`) and must run against temporary copies.

```bash
cd .
tmpdir="$(mktemp -d)"
cp docs/plans/2026-04-22-controlnet-track-3-backend-execution-waves.json "$tmpdir/plan.json"
printf '{"plan":"plan.json","taken":{},"completed":{}}\n' > "$tmpdir/plan.json.state.json"
export WAVEPLAN_PLAN="$tmpdir/plan.json"
export WAVEPLAN_STATE="$tmpdir/plan.json.state.json"
```

Use this helper to extract protocol-native `structuredContent` when present, and otherwise fall back to parsing `result.content[0].text`:

```bash
parse_tool_result() {
  python3 -c '
import json, sys
msg = json.loads(sys.stdin.read())
result = msg["result"]
if "structuredContent" in result and result["structuredContent"] is not None:
    print(json.dumps(result["structuredContent"]))
else:
    print(result["content"][0]["text"])
'
}
```

Run the probes in a valid state order:

```bash
# peek
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"waveplan_peek","arguments":{}}}' \
  | ./waveplan-mcp 2>/dev/null | parse_tool_result \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); assert "task_id" in d; print("peek OK")'

# pop
printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"waveplan_pop","arguments":{"agent":"psi"}}}' \
  | ./waveplan-mcp 2>/dev/null | parse_tool_result \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["claimed"] is True; print("pop OK")'

# get taken
printf '%s\n' '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"waveplan_get","arguments":{"mode":"taken"}}}' \
  | ./waveplan-mcp 2>/dev/null | parse_tool_result \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); assert "tasks" in d and len(d["tasks"]) == 1; print("get taken OK")'

# get deptree (via waveplan_get mode=deptree, not a separate tool)
printf '%s\n' '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"waveplan_get","arguments":{"mode":"deptree"}}}' \
  | ./waveplan-mcp 2>/dev/null | parse_tool_result \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); assert "tasks" in d and all("group" in t for t in d["tasks"]); print("deptree OK")'

# start review for the claimed task
printf '%s\n' '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"waveplan_start_review","arguments":{"task_id":"T1.1","reviewer":"sigma"}}}' \
  | ./waveplan-mcp 2>/dev/null | parse_tool_result \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["success"] is True; print("start_review OK")'

# end review with note
printf '%s\n' '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"waveplan_end_review","arguments":{"task_id":"T1.1","review_note":"Looks good"}}}' \
  | ./waveplan-mcp 2>/dev/null | parse_tool_result \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["success"] is True and d["review_note"] == "Looks good"; print("end_review OK")'

# fin with git_sha
printf '%s\n' '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"waveplan_fin","arguments":{"task_id":"T1.1","git_sha":"abc1234"}}}' \
  | ./waveplan-mcp 2>/dev/null | parse_tool_result \
  | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["success"] is True and d["git_sha"] == "abc1234"; print("fin OK")'
```

- [ ] **Step 3: Verify state-file compatibility against the same temp state**

```bash
cd .
python3 ../scripts/waveplan --plan "$WAVEPLAN_PLAN" --state "$WAVEPLAN_STATE" get task-T1.1 --json
```

Expected: the Python CLI reads the MCP-written temp state without errors and shows `review_note` / `git_sha` fields for `T1.1`.

- [ ] **Step 4: Clean up temp verification state**

```bash
rm -rf "$tmpdir"
unset WAVEPLAN_PLAN WAVEPLAN_STATE
unset -f parse_tool_result
```

Expected: no repo-local state files changed during verification.

---

## Self-Review Checklist

1. **Spec coverage:** Every gap listed in the design doc has a corresponding task:
   - JSON output → Tasks 3, 4, 5, 6, 7, 8, 9
   - `deptree` mode → Task 8
   - `review_note` on `end_review` → Task 6
   - `git_sha` on `fin` → Task 7
   - `peek`/`pop` full detail → Tasks 3, 4 (via `taskInfo`)
   - `get` full fields → Task 8 (via `taskInfo`)

2. **Placeholder scan:** No TBD, TODO, or "implement later" patterns found.

3. **Type consistency:** All Go types use consistent naming (`TaskEntry`, `PlanUnit`, `WaveplanPlan`, `WaveplanState`). Helper functions use `(s *WaveplanServer)` receiver consistently.

4. **State file compatibility:** New fields (`ReviewNote`, `GitSha`) default to empty strings, which is backward-compatible with existing state files.
