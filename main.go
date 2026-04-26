package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// WaveplanState represents the state file structure
type WaveplanState struct {
	Plan      string                  `json:"plan"`
	Taken     map[string]TaskEntry    `json:"taken"`
	Completed map[string]TaskEntry    `json:"completed"`
}

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

// WaveplanPlan represents the plan file structure
type WaveplanPlan struct {
	Units    map[string]PlanUnit `json:"units"`
	Tasks    map[string]any      `json:"tasks"`
	DocIndex map[string]any      `json:"doc_index"`
	FpIndex  map[string]string   `json:"fp_index"`
}

// PlanUnit represents a unit in the plan
type PlanUnit struct {
	Task      string   `json:"task"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`
	Wave      int      `json:"wave"`
	PlanLine  int      `json:"plan_line"`
	DependsOn []string `json:"depends_on"`
	DocRefs   []string `json:"doc_refs"`
	FpRefs    []string `json:"fp_refs"`
	Notes     []string `json:"notes"`
	Command   string   `json:"command,omitempty"`
}

// WaveplanServer holds the server state
type WaveplanServer struct {
	mu        sync.Mutex
	planPath  string
	statePath string
	plan      *WaveplanPlan
	state     *WaveplanState
}

// NewWaveplanServer creates a new server instance
func NewWaveplanServer(planPath, statePath string) (*WaveplanServer, error) {
	// Load plan
	planData, err := os.ReadFile(planPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read plan file: %w", err)
	}

	var plan WaveplanPlan
	if err := json.Unmarshal(planData, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan file: %w", err)
	}

	// Load or create state
	state := &WaveplanState{
		Plan:      filepath.Base(planPath),
		Taken:     make(map[string]TaskEntry),
		Completed: make(map[string]TaskEntry),
	}

	if statePath != "" {
		if data, err := os.ReadFile(statePath); err == nil {
			if err := json.Unmarshal(data, state); err != nil {
				return nil, fmt.Errorf("failed to parse state file: %w", err)
			}
		}
	}

	return &WaveplanServer{
		planPath:  planPath,
		statePath: statePath,
		plan:      &plan,
		state:     state,
	}, nil
}

// saveState writes the current state to disk
func (s *WaveplanServer) saveState() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}
	return os.WriteFile(s.statePath, data, 0644)
}

// nowStr returns current timestamp
func nowStr() string {
	return time.Now().Format("2006-01-02 15:04")
}

// isAvailable checks if a task is available
func (s *WaveplanServer) isAvailable(taskID string) bool {
	if _, ok := s.state.Taken[taskID]; ok {
		return false
	}
	if _, ok := s.state.Completed[taskID]; ok {
		return false
	}
	unit, ok := s.plan.Units[taskID]
	if !ok {
		return false
	}
	for _, dep := range unit.DependsOn {
		if _, ok := s.state.Completed[dep]; !ok {
			return false
		}
	}
	return true
}

// nextAvailableTask returns the next available task
func (s *WaveplanServer) nextAvailableTask() string {
	var available []string
	for taskID := range s.plan.Units {
		if s.isAvailable(taskID) {
			available = append(available, taskID)
		}
	}
	if len(available) == 0 {
		return ""
	}
	// Sort by wave, then by task ID
	sort.Strings(available)
	minWave := 9999
	for _, tid := range available {
		wave := s.plan.Units[tid].Wave
		if wave < minWave {
			minWave = wave
		}
	}
	for _, tid := range available {
		if s.plan.Units[tid].Wave == minWave {
			return tid
		}
	}
	return ""
}

// createTools creates the MCP tools
func (s *WaveplanServer) createTools() []server.ServerTool {
	tools := []server.ServerTool{
		{
			Tool: mcp.NewTool("waveplan_peek",
				mcp.WithDescription("Show the next available task without claiming it"),
			),
			Handler: s.handlePeek,
		},
		{
			Tool: mcp.NewTool("waveplan_pop",
				mcp.WithDescription("Claim the next available task for an agent"),
				mcp.WithString("agent", mcp.Required(), mcp.Description("Agent name claiming the task")),
			),
			Handler: s.handlePop,
		},
		{
			Tool: mcp.NewTool("waveplan_start_review",
				mcp.WithDescription("Start a review for a task"),
				mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to review")),
				mcp.WithString("reviewer", mcp.Required(), mcp.Description("Agent name reviewing the task")),
			),
			Handler: s.handleStartReview,
		},
		{
			Tool: mcp.NewTool("waveplan_end_review",
				mcp.WithDescription("End a review for a task"),
				mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to end review for")),
				mcp.WithString("review_note", mcp.Description("Optional review note")),
			),
			Handler: s.handleEndReview,
		},
		{
			Tool: mcp.NewTool("waveplan_fin",
				mcp.WithDescription("Mark a task as completed"),
				mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to complete")),
				mcp.WithString("git_sha", mcp.Description("Optional git SHA of the commit")),
			),
			Handler: s.handleFin,
		},
		{
			Tool: mcp.NewTool("waveplan_get",
				mcp.WithDescription("Report tasks based on filter mode"),
				mcp.WithString("mode", mcp.Description("Filter: all (default), taken, open, complete, deptree, task-<id>, or agent name")),
			),
			Handler: s.handleGet,
		},
		{
			Tool: mcp.NewTool("waveplan_deptree",
				mcp.WithDescription("Show tasks in dependency order with parallel groups"),
			),
			Handler: s.handleDeptree,
		},
		{
			Tool: mcp.NewTool("waveplan_list_plans",
				mcp.WithDescription("List available execution wave plans"),
				mcp.WithString("plan_dir", mcp.Description("Directory to search for plans (default: docs/superpowers/plans)")),
			),
			Handler: s.handleListPlans,
		},
	}
	return tools
}

// Handler implementations
func (s *WaveplanServer) handlePeek(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	taskID := s.nextAvailableTask()
	if taskID == "" {
		return mcp.NewToolResultText(`{"error":"No available tasks."}`), nil
	}
	info := s.taskInfo(taskID)
	data, _ := json.Marshal(info)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *WaveplanServer) handlePop(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	agent, err := requiredStringParam(request.Params.Arguments, "agent")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	taskID := s.nextAvailableTask()
	if taskID == "" {
		return mcp.NewToolResultText(`{"error":"No available tasks."}`), nil
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
	data, _ := json.Marshal(info)
	return mcp.NewToolResultText(string(data)), nil
}

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
	}
	if err := s.saveState(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to save state: %v", err)), nil
	}
	unit := s.plan.Units[taskID]
	result := map[string]any{
		"success":           true,
		"task_id":           taskID,
		"title":             unit.Title,
		"reviewer":          reviewer,
		"review_entered_at": ts,
	}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

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
	}
	if err := s.saveState(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to save state: %v", err)), nil
	}
	unit := s.plan.Units[taskID]
	result := map[string]any{
		"success":           true,
		"task_id":           taskID,
		"title":             unit.Title,
		"reviewer":          taken.Reviewer,
		"review_ended_at":   ts,
		"review_note":       reviewNote,
	}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

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
	// Backfill started_at if task was completed without being popped (matches Python CLI behavior)
	startedAt := taken.StartedAt
	if startedAt == "" {
		startedAt = ts
	}
	s.state.Completed[taskID] = TaskEntry{
		TakenBy:         taken.TakenBy,
		StartedAt:       startedAt,
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
	result := map[string]any{
		"success":      true,
		"task_id":      taskID,
		"title":        unit.Title,
		"finished_at":  ts,
		"git_sha":      gitSha,
	}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *WaveplanServer) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	mode, _ := optionalStringParam(request.Params.Arguments, "mode")
	if mode == "" {
		mode = "all"
	}

	// deptree mode uses topological sort with group numbers
	if mode == "deptree" {
		return s.doDeptree(), nil
	}

	var taskIDs []string
	switch mode {
	case "taken":
		for tid := range s.state.Taken {
			taskIDs = append(taskIDs, tid)
		}
	case "open":
		for tid := range s.plan.Units {
			if s.isAvailable(tid) {
				taskIDs = append(taskIDs, tid)
			}
		}
	case "complete":
		for tid := range s.state.Completed {
			taskIDs = append(taskIDs, tid)
		}
	case "all":
		for tid := range s.state.Taken {
			taskIDs = append(taskIDs, tid)
		}
		for tid := range s.state.Completed {
			taskIDs = append(taskIDs, tid)
		}
	default:
		if strings.HasPrefix(mode, "task-") {
			tid := mode[5:]
			if _, ok := s.plan.Units[tid]; !ok {
				return mcp.NewToolResultError(fmt.Sprintf("task '%s' not found in plan", tid)), nil
			}
			taskIDs = []string{tid}
		} else {
			// Agent filter
			for tid, entry := range s.state.Taken {
				if entry.TakenBy == mode {
					taskIDs = append(taskIDs, tid)
				}
			}
			for tid, entry := range s.state.Completed {
				if entry.TakenBy == mode {
					taskIDs = append(taskIDs, tid)
				}
			}
		}
	}
	// Sort for deterministic output (matches Python CLI behavior)
	sort.Strings(taskIDs)
	if len(taskIDs) == 0 {
		return mcp.NewToolResultText(`{"tasks":[]}`), nil
	}
	var tasks []map[string]any
	for _, tid := range taskIDs {
		tasks = append(tasks, s.buildTaskEntry(tid, mode == "open"))
	}
	result := map[string]any{"tasks": tasks}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

// buildTaskEntry builds a task info map for a given task ID.
func (s *WaveplanServer) buildTaskEntry(tid string, includeDeps bool) map[string]any {
	taken := s.state.Taken[tid]
	completed := s.state.Completed[tid]
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
	// Determine status
	var status string
	if _, ok := s.state.Completed[tid]; ok {
		status = "completed"
	} else if _, ok := s.state.Taken[tid]; ok {
		status = "taken"
	} else {
		status = "available"
	}
	info := s.taskInfo(tid)
	info["status"] = status
	info["started_at"] = nilIfEmpty(started)
	info["finished_at"] = nilIfEmpty(finished)
	info["taken_by"] = nilIfEmpty(agent)
	info["review_entered_at"] = nilIfEmpty(reviewEntered)
	info["review_ended_at"] = nilIfEmpty(reviewEnded)
	info["reviewer"] = nilIfEmpty(reviewer)
	info["review_note"] = nilIfEmpty(reviewNote)
	info["git_sha"] = nilIfEmpty(gitSha)
	// get/deptree use "plan" key (peek/pop use "plan_ref")
	if pr, ok := info["plan_ref"]; ok {
		delete(info, "plan_ref")
		info["plan"] = pr
	}
	return info
}

// doDeptree returns deptree JSON (caller must hold s.mu).
func (s *WaveplanServer) doDeptree() *mcp.CallToolResult {
	sorted := s.topologicalSort()
	if len(sorted) == 0 {
		return mcp.NewToolResultText(`{"tasks":[]}`)
	}
	var tasks []map[string]any
	for _, item := range sorted {
		info := s.buildTaskEntry(item.TaskID, false)
		info["group"] = item.Group
		tasks = append(tasks, info)
	}
	result := map[string]any{"tasks": tasks}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data))
}

func (s *WaveplanServer) handleDeptree(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doDeptree(), nil
}

func (s *WaveplanServer) handleListPlans(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	planDir, _ := optionalStringParam(request.Params.Arguments, "plan_dir")
	if planDir == "" {
		planDir = "plans"
	}
	matches, err := filepath.Glob(filepath.Join(planDir, "*-execution-waves.json"))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list plans: %v", err)), nil
	}
	result := map[string]any{"plans": matches}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

// Helper functions
func requiredStringParam(args any, name string) (string, error) {
	argsMap, ok := args.(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid arguments type")
	}
	val, ok := argsMap[name]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", name)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s must be a string", name)
	}
	return str, nil
}

func optionalStringParam(args any, name string) (string, bool) {
	argsMap, ok := args.(map[string]any)
	if !ok {
		return "", false
	}
	val, ok := argsMap[name]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// findPlanRef resolves a task to its plan document reference.
// Matches the parent task's plan_line against doc_index entries with kind="plan".
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
	planLineVal := parentMap["plan_line"]
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
			if lineVal, ok := entryMap["line"].(float64); ok && lineVal == planLine {
				return map[string]any{
					"ref":  ref,
					"path": entryMap["path"],
					"line": int(lineVal),
				}
			}
		}
	}
	return nil
}

// resolveDocRefs resolves doc ref names to full entries.
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
		m := map[string]any{"ref": ref}
		for k, v := range entryMap {
			m[k] = v
		}
		result = append(result, m)
	}
	return result
}

// resolveFpRefs resolves fp ref names to full entries.
func (s *WaveplanServer) resolveFpRefs(refNames []string) []map[string]any {
	result := make([]map[string]any, 0, len(refNames))
	for _, ref := range refNames {
		fpID := s.plan.FpIndex[ref]
		if fpID == "" {
			fpID = ref
		}
		result = append(result, map[string]any{"ref": ref, "fp_id": fpID})
	}
	return result
}

// taskInfo builds a full task info map for a given task ID.
func (s *WaveplanServer) taskInfo(taskID string) map[string]any {
	unit := s.plan.Units[taskID]
	info := map[string]any{
		"task_id":    taskID,
		"task":       unit.Task,
		"title":      unit.Title,
		"kind":       unit.Kind,
		"wave":       unit.Wave,
		"plan_line":  unit.PlanLine,
		"depends_on": unit.DependsOn,
	}
	if planRef := s.findPlanRef(taskID); planRef != nil {
		info["plan_ref"] = planRef
	}
	if len(unit.DocRefs) > 0 {
		info["doc_refs"] = s.resolveDocRefs(unit.DocRefs)
	}
	if len(unit.FpRefs) > 0 {
		info["fp_refs"] = s.resolveFpRefs(unit.FpRefs)
	}
	if len(unit.Notes) > 0 {
		info["notes"] = unit.Notes
	}
	if unit.Command != "" {
		info["command"] = unit.Command
	}
	return info
}

// nilIfEmpty returns nil for empty strings, useful for JSON null output.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// topoSortResult holds a task ID and its parallel group number.
type topoSortResult struct {
	TaskID string
	Group  int
}

// topologicalSort returns task IDs in dependency order with parallel group numbers.
// Tasks with no dependencies are group 1. Tasks whose dependencies are all in group N are group N+1.
// Within each group, sorted by wave then task ID.
func (s *WaveplanServer) topologicalSort() []topoSortResult {
	units := s.plan.Units
	inDegree := make(map[string]int)
	for tid := range units {
		inDegree[tid] = 0
	}
	for tid, task := range units {
		for _, dep := range task.DependsOn {
			if _, ok := inDegree[dep]; ok {
				inDegree[tid]++
			}
		}
	}

	// Sort key: wave then task ID
	sortKey := func(tid string) (int, string) {
		return units[tid].Wave, tid
	}

	// Start with tasks that have no dependencies (group 1)
	var queue []string
	for tid, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, tid)
		}
	}
	sort.Slice(queue, func(i, j int) bool {
		wi, si := sortKey(queue[i])
		wj, sj := sortKey(queue[j])
		if wi != wj {
			return wi < wj
		}
		return si < sj
	})

	var result []topoSortResult
	group := 1

	for len(queue) > 0 {
		nextQueue := []string{}
		for _, tid := range queue {
			result = append(result, topoSortResult{TaskID: tid, Group: group})
			for otherTid, otherTask := range units {
				for _, dep := range otherTask.DependsOn {
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
			wi, si := sortKey(nextQueue[i])
			wj, sj := sortKey(nextQueue[j])
			if wi != wj {
				return wi < wj
			}
			return si < sj
		})
		queue = nextQueue
		group++
	}

	return result
}

func main() {
	planPath := os.Getenv("WAVEPLAN_PLAN")
	statePath := os.Getenv("WAVEPLAN_STATE")
	if planPath == "" {
		planPath = "plans/default-execution-waves.json"
	}
	if statePath == "" {
		statePath = planPath + ".state.json"
	}

	srv, err := NewWaveplanServer(planPath, statePath)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	mcpServer := server.NewMCPServer(
		"waveplan",
		"0.1.0",
	)

	// Add all tools
	for _, tool := range srv.createTools() {
		mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}