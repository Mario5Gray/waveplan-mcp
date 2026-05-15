package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/contextsize"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/swim"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var gitSha = "unknown"

// WaveplanState represents the state file structure
type WaveplanState struct {
	Plan      string               `json:"plan"`
	Taken     map[string]TaskEntry `json:"taken"`
	Completed map[string]TaskEntry `json:"completed"`
	Tail      map[string]TaskEntry `json:"tail"`
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
	mu           sync.Mutex
	planPath     string
	statePath    string
	plan         *WaveplanPlan
	state        *WaveplanState
	serverGitSha string
}

// NewWaveplanServer creates a new server instance
func NewWaveplanServer(planPath, statePath string) (*WaveplanServer, error) {
	return newWaveplanServer(planPath, statePath, gitSha)
}

func newWaveplanServer(planPath, statePath, serverGitSha string) (*WaveplanServer, error) {
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
		Tail:      make(map[string]TaskEntry),
	}

	if statePath != "" {
		if data, err := os.ReadFile(statePath); err == nil {
			if err := json.Unmarshal(data, state); err != nil {
				return nil, fmt.Errorf("failed to parse state file: %w", err)
			}
		}
	}

	return &WaveplanServer{
		planPath:     planPath,
		statePath:    statePath,
		plan:         &plan,
		state:        state,
		serverGitSha: serverGitSha,
	}, nil
}

// reloadState re-reads the state file from disk and merges it into the current state.
// This ensures changes made by other processes (e.g., CLI subprocesses) are visible
// to a long-running server instance before we write our own changes back.
func (s *WaveplanServer) reloadState() error {
	if s.statePath == "" {
		return nil
	}
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		// File doesn't exist yet or can't be read — nothing to merge.
		return nil
	}
	var fileState WaveplanState
	if err := json.Unmarshal(data, &fileState); err != nil {
		return fmt.Errorf("failed to parse state file for reload: %w", err)
	}
	// Merge: bring in Taken and Completed entries from disk that we don't have in memory.
	for k, v := range fileState.Taken {
		if _, ok := s.state.Taken[k]; !ok {
			if s.state.Taken == nil {
				s.state.Taken = make(map[string]TaskEntry)
			}
			s.state.Taken[k] = v
		}
	}
	for k, v := range fileState.Completed {
		if _, ok := s.state.Completed[k]; !ok {
			if s.state.Completed == nil {
				s.state.Completed = make(map[string]TaskEntry)
			}
			s.state.Completed[k] = v
		}
	}
	for k, v := range fileState.Tail {
		if _, ok := s.state.Tail[k]; !ok {
			if s.state.Tail == nil {
				s.state.Tail = make(map[string]TaskEntry)
			}
			s.state.Tail[k] = v
		}
	}
	return nil
}

// saveState writes the current state to disk.
// If statePath is empty, this is a no-op (useful for in-memory tests).
func (s *WaveplanServer) saveState() error {
	if s.statePath == "" {
		return nil
	}
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
			Tool: mcp.NewTool("waveplan_start_fix",
				mcp.WithDescription("Return a reviewed task to taken status for a fix cycle"),
				mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to fix")),
			),
			Handler: s.handleStartFix,
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
				mcp.WithString("plan_dir", mcp.Description("Directory to search for plans (default: ~/.local/share/waveplanner/plans)")),
			),
			Handler: s.handleListPlans,
		},
		{
			Tool: mcp.NewTool("waveplan_version",
				mcp.WithDescription("Show the version (git SHA) of this server"),
			),
			Handler: s.handleVersion,
		},
		{
			Tool: mcp.NewTool("waveplan_swim_compile",
				mcp.WithDescription("Compile a SWIM execution schedule from a plan and waveagents file"),
				mcp.WithString("plan", mcp.Required(), mcp.Description("Input plan JSON path")),
				mcp.WithString("agents", mcp.Description("Input waveagents JSON path (default: ~/.config/waveplan-mcp/waveagents.json)")),
				mcp.WithString("out", mcp.Description("Optional output schedule JSON path")),
				mcp.WithString("task_scope", mcp.Description("Task scope: all|open")),
				mcp.WithBoolean("bootstrap_state", mcp.Description("Create <plan>.state.json if it does not already exist")),
			),
			Handler: s.handleSwimCompile,
		},
		{
			Tool: mcp.NewTool("waveplan_swim_next",
				mcp.WithDescription("Resolve the next SWIM step"),
				mcp.WithString("schedule", mcp.Required(), mcp.Description("Input schedule JSON path")),
				mcp.WithString("journal", mcp.Description("Optional journal JSON path")),
				mcp.WithString("state", mcp.Description("Optional state JSON path")),
			),
			Handler: s.handleSwimNext,
		},
		{
			Tool: mcp.NewTool("waveplan_swim_step",
				mcp.WithDescription("Inspect, apply, or acknowledge a single SWIM step"),
				mcp.WithString("schedule", mcp.Description("Input schedule JSON path")),
				mcp.WithString("journal", mcp.Description("Optional journal JSON path")),
				mcp.WithString("state", mcp.Description("Optional state JSON path")),
				mcp.WithNumber("seq", mcp.Description("Target sequence number")),
				mcp.WithString("step_id", mcp.Description("Target step ID")),
				mcp.WithBoolean("apply", mcp.Description("Apply the selected step")),
				mcp.WithString("ack_unknown", mcp.Description("Acknowledge unknown step by ID")),
				mcp.WithString("as", mcp.Description("Ack outcome: failed|waived")),
			),
			Handler: s.handleSwimStep,
		},
		{
			Tool: mcp.NewTool("waveplan_swim_run",
				mcp.WithDescription("Run SWIM steps until a stop condition is met"),
				mcp.WithString("schedule", mcp.Required(), mcp.Description("Input schedule JSON path")),
				mcp.WithString("journal", mcp.Description("Optional journal JSON path")),
				mcp.WithString("state", mcp.Description("Optional state JSON path")),
				mcp.WithString("until", mcp.Required(), mcp.Description("Stop condition: action, seq:N, or step:<id>")),
				mcp.WithBoolean("dry_run", mcp.Description("Resolve without mutation")),
				mcp.WithNumber("max_steps", mcp.Description("Maximum number of steps to process")),
			),
			Handler: s.handleSwimRun,
		},
		{
			Tool: mcp.NewTool("waveplan_swim_journal",
				mcp.WithDescription("Inspect a SWIM journal"),
				mcp.WithString("schedule", mcp.Required(), mcp.Description("Input schedule JSON path")),
				mcp.WithString("journal", mcp.Description("Optional journal JSON path")),
				mcp.WithNumber("tail", mcp.Description("Show the last N journal events")),
			),
			Handler: s.handleSwimJournal,
		},
		{
			Tool: mcp.NewTool("waveplan_swim_refine",
				mcp.WithDescription("Compile a coarse SWIM plan into a deterministic refinement sidecar"),
				mcp.WithString("plan", mcp.Required(), mcp.Description("Input coarse *-execution-waves.json path")),
				mcp.WithArray("targets",
					mcp.Required(),
					mcp.Description("Unit IDs to refine"),
					mcp.WithStringItems(),
					mcp.MinItems(1),
				),
				mcp.WithString("profile", mcp.Description("Refinement profile (v1: 8k only)")),
				mcp.WithString("out", mcp.Description("Optional output refine JSON path")),
				mcp.WithString("invoker", mcp.Description("Invoker script path used in emitted invoke.argv")),
			),
			Handler: s.handleSwimRefine,
		},
		{
			Tool: mcp.NewTool("waveplan_swim_refine_run",
				mcp.WithDescription("Execute a refinement sidecar's fine steps with parent rollup"),
				mcp.WithString("refine", mcp.Required(), mcp.Description("Path to refinement sidecar JSON")),
				mcp.WithString("refine_journal", mcp.Description("Fine journal path (default: <refine>.journal.json)")),
				mcp.WithString("coarse_journal", mcp.Description("Coarse journal path for parent rollup events")),
				mcp.WithString("state", mcp.Description("Live waveplan state file")),
				mcp.WithBoolean("dry_run", mcp.Description("Resolve without lock, invoke, or journal mutation")),
				mcp.WithString("work_dir", mcp.Description("Working directory passed to invoke")),
			),
			Handler: s.handleSwimRefineRun,
		},
		{
			Tool: mcp.NewTool("waveplan_context_estimate",
				mcp.WithDescription("Estimate the context footprint (token budget) of an issue candidate. Accepts a ContextCandidate JSON object, optional budget range, and base directory for resolving file paths."),
				mcp.WithString("candidate", mcp.Required(), mcp.Description("ContextCandidate JSON object (id, title, description, referenced_files, referenced_sections, depends_on, source)")),
				mcp.WithString("budget", mcp.Description("Budget range in tokens, min:max (default: 64000:192000)")),
				mcp.WithString("base_dir", mcp.Description("Root directory for resolving referenced file paths")),
			),
			Handler: s.handleContextEstimate,
		},
	}
	return tools
}

// Handler implementations
func (s *WaveplanServer) handlePeek(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.reloadState(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to reload state: %v", err)), nil
	}

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
	trace := startLangSmithTrace("waveplan pop", map[string]any{
		"plan_path": s.planPath,
		"agent":     agent,
	})
	traceStatus := "error"
	traceErr := ""
	traceOut := map[string]any{}
	defer func() {
		traceOut["status"] = traceStatus
		if traceErr != "" {
			traceOut["error"] = traceErr
		}
		trace.finish(traceOut)
	}()

	if err != nil {
		traceErr = err.Error()
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := s.reloadState(); err != nil {
		traceErr = fmt.Sprintf("failed to reload state: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to reload state: %v", err)), nil
	}

	taskID := s.nextAvailableTask()
	if taskID == "" {
		traceStatus = "no_available_tasks"
		return mcp.NewToolResultText(`{"error":"No available tasks."}`), nil
	}
	ts := nowStr()
	s.state.Taken[taskID] = TaskEntry{
		TakenBy:   agent,
		StartedAt: ts,
		GitSha:    s.serverGitSha,
	}
	if err := s.saveState(); err != nil {
		traceErr = fmt.Sprintf("failed to save state: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to save state: %v", err)), nil
	}
	info := s.taskInfo(taskID)
	info["claimed"] = true
	info["taken_by"] = agent
	info["started_at"] = ts
	traceStatus = "ok"
	traceOut["task_id"] = taskID
	traceOut["taken_by"] = agent
	traceOut["started_at"] = ts
	data, _ := json.Marshal(info)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *WaveplanServer) handleStartReview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID, err := requiredStringParam(request.Params.Arguments, "task_id")
	reviewer, reviewErr := requiredStringParam(request.Params.Arguments, "reviewer")
	trace := startLangSmithTrace("waveplan start_review", map[string]any{
		"plan_path": s.planPath,
		"task_id":   taskID,
		"reviewer":  reviewer,
	})
	traceStatus := "error"
	traceErr := ""
	traceOut := map[string]any{}
	defer func() {
		traceOut["status"] = traceStatus
		if traceErr != "" {
			traceOut["error"] = traceErr
		}
		trace.finish(traceOut)
	}()

	if err != nil {
		traceErr = err.Error()
		return mcp.NewToolResultError(err.Error()), nil
	}
	if reviewErr != nil {
		traceErr = reviewErr.Error()
		return mcp.NewToolResultError(reviewErr.Error()), nil
	}

	if err := s.reloadState(); err != nil {
		traceErr = fmt.Sprintf("failed to reload state: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to reload state: %v", err)), nil
	}

	if _, ok := s.plan.Units[taskID]; !ok {
		traceErr = fmt.Sprintf("task '%s' not found in plan", taskID)
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' not found in plan", taskID)), nil
	}
	taken, ok := s.state.Taken[taskID]
	if !ok {
		traceErr = fmt.Sprintf("task '%s' is not currently taken", taskID)
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' is not currently taken", taskID)), nil
	}
	if taken.ReviewEnteredAt != "" {
		traceErr = fmt.Sprintf("review for %s already started", taskID)
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
		traceErr = fmt.Sprintf("failed to save state: %v", err)
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
	traceStatus = "ok"
	traceOut["task_id"] = taskID
	traceOut["reviewer"] = reviewer
	traceOut["review_entered_at"] = ts
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *WaveplanServer) handleStartFix(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID, err := requiredStringParam(request.Params.Arguments, "task_id")
	trace := startLangSmithTrace("waveplan start_fix", map[string]any{
		"plan_path": s.planPath,
		"task_id":   taskID,
	})
	traceStatus := "error"
	traceErr := ""
	traceOut := map[string]any{}
	defer func() {
		traceOut["status"] = traceStatus
		if traceErr != "" {
			traceOut["error"] = traceErr
		}
		trace.finish(traceOut)
	}()

	if err != nil {
		traceErr = err.Error()
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := s.reloadState(); err != nil {
		traceErr = fmt.Sprintf("failed to reload state: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to reload state: %v", err)), nil
	}

	if _, ok := s.plan.Units[taskID]; !ok {
		traceErr = fmt.Sprintf("task '%s' not found in plan", taskID)
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' not found in plan", taskID)), nil
	}
	taken, ok := s.state.Taken[taskID]
	if !ok {
		traceErr = fmt.Sprintf("task '%s' is not currently taken", taskID)
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' is not currently taken", taskID)), nil
	}
	if taken.ReviewEnteredAt == "" {
		traceErr = fmt.Sprintf("no active review for %s", taskID)
		return mcp.NewToolResultError(fmt.Sprintf("no active review for %s", taskID)), nil
	}
	if taken.ReviewEndedAt != "" {
		traceErr = fmt.Sprintf("review for %s already ended", taskID)
		return mcp.NewToolResultError(fmt.Sprintf("review for %s already ended", taskID)), nil
	}

	s.state.Taken[taskID] = TaskEntry{
		TakenBy:   taken.TakenBy,
		StartedAt: taken.StartedAt,
		GitSha:    taken.GitSha,
	}
	if err := s.saveState(); err != nil {
		traceErr = fmt.Sprintf("failed to save state: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to save state: %v", err)), nil
	}
	unit := s.plan.Units[taskID]
	result := map[string]any{
		"success": true,
		"task_id": taskID,
		"title":   unit.Title,
		"status":  "taken",
	}
	traceStatus = "ok"
	traceOut["task_id"] = taskID
	traceOut["status_after"] = "taken"
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *WaveplanServer) handleEndReview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID, err := requiredStringParam(request.Params.Arguments, "task_id")
	reviewNote, _ := optionalStringParam(request.Params.Arguments, "review_note")
	trace := startLangSmithTrace("waveplan end_review", map[string]any{
		"plan_path":   s.planPath,
		"task_id":     taskID,
		"review_note": reviewNote,
		"has_note":    reviewNote != "",
	})
	traceStatus := "error"
	traceErr := ""
	traceOut := map[string]any{}
	defer func() {
		traceOut["status"] = traceStatus
		if traceErr != "" {
			traceOut["error"] = traceErr
		}
		trace.finish(traceOut)
	}()

	if err != nil {
		traceErr = err.Error()
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := s.reloadState(); err != nil {
		traceErr = fmt.Sprintf("failed to reload state: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to reload state: %v", err)), nil
	}

	if _, ok := s.plan.Units[taskID]; !ok {
		traceErr = fmt.Sprintf("task '%s' not found in plan", taskID)
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' not found in plan", taskID)), nil
	}
	taken, ok := s.state.Taken[taskID]
	if !ok {
		traceErr = fmt.Sprintf("task '%s' is not currently taken", taskID)
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' is not currently taken", taskID)), nil
	}
	if taken.ReviewEnteredAt == "" {
		traceErr = fmt.Sprintf("no active review for %s", taskID)
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
		traceErr = fmt.Sprintf("failed to save state: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to save state: %v", err)), nil
	}
	unit := s.plan.Units[taskID]
	result := map[string]any{
		"success":         true,
		"task_id":         taskID,
		"title":           unit.Title,
		"reviewer":        taken.Reviewer,
		"review_ended_at": ts,
		"review_note":     reviewNote,
	}
	traceStatus = "ok"
	traceOut["task_id"] = taskID
	traceOut["reviewer"] = taken.Reviewer
	traceOut["review_ended_at"] = ts
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *WaveplanServer) handleFin(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID, err := requiredStringParam(request.Params.Arguments, "task_id")
	gitSha, _ := optionalStringParam(request.Params.Arguments, "git_sha")
	trace := startLangSmithTrace("waveplan fin", map[string]any{
		"plan_path": s.planPath,
		"task_id":   taskID,
		"git_sha":   gitSha,
	})
	traceStatus := "error"
	traceErr := ""
	traceOut := map[string]any{}
	defer func() {
		traceOut["status"] = traceStatus
		if traceErr != "" {
			traceOut["error"] = traceErr
		}
		trace.finish(traceOut)
	}()

	if err != nil {
		traceErr = err.Error()
		return mcp.NewToolResultError(err.Error()), nil
	}
	if _, ok := s.plan.Units[taskID]; !ok {
		traceErr = fmt.Sprintf("task '%s' not found in plan", taskID)
		return mcp.NewToolResultError(fmt.Sprintf("task '%s' not found in plan", taskID)), nil
	}
	if _, ok := s.state.Completed[taskID]; ok {
		traceErr = fmt.Sprintf("task %s is already completed", taskID)
		return mcp.NewToolResultError(fmt.Sprintf("task %s is already completed", taskID)), nil
	}
	unit := s.plan.Units[taskID]
	for _, dep := range unit.DependsOn {
		if _, ok := s.state.Completed[dep]; !ok {
			traceErr = fmt.Sprintf("task %s has incomplete dependencies: %s", taskID, strings.Join(unit.DependsOn, ", "))
			return mcp.NewToolResultError(fmt.Sprintf("task %s has incomplete dependencies: %s", taskID, strings.Join(unit.DependsOn, ", "))), nil
		}
	}
	taken := s.state.Taken[taskID]
	reviewEntered := taken.ReviewEnteredAt
	reviewEnded := taken.ReviewEndedAt
	if reviewEntered != "" && reviewEnded == "" {
		traceErr = fmt.Sprintf("task %s has an active review. End review before completing.", taskID)
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
		GitSha: func() string {
			if gitSha != "" {
				return gitSha
			}
			return s.serverGitSha
		}(),
		FinishedAt: ts,
	}
	if s.state.Tail == nil {
		s.state.Tail = make(map[string]TaskEntry)
	}
	s.state.Tail[taskID] = taken
	delete(s.state.Taken, taskID)
	if err := s.saveState(); err != nil {
		traceErr = fmt.Sprintf("failed to save state: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to save state: %v", err)), nil
	}
	result := map[string]any{
		"success":     true,
		"task_id":     taskID,
		"title":       unit.Title,
		"finished_at": ts,
		"git_sha":     gitSha,
	}
	traceStatus = "ok"
	traceOut["task_id"] = taskID
	traceOut["finished_at"] = ts
	traceOut["git_sha"] = gitSha
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *WaveplanServer) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.reloadState(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to reload state: %v", err)), nil
	}

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

	if err := s.reloadState(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to reload state: %v", err)), nil
	}
	return s.doDeptree(), nil
}

func (s *WaveplanServer) handleListPlans(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	planDir, _ := optionalStringParam(request.Params.Arguments, "plan_dir")
	if planDir == "" {
		planDir = defaultPlanDir()
	}
	matches, err := filepath.Glob(filepath.Join(planDir, "*-execution-waves.json"))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list plans: %v", err)), nil
	}
	result := map[string]any{"plans": matches}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *WaveplanServer) handleVersion(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result := map[string]any{"version": gitSha}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func (s *WaveplanServer) handleSwimCompile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	plan, err := requiredStringParam(request.Params.Arguments, "plan")
	if err != nil {
		return swimErrorResult(2, err.Error()), nil
	}
	agents, ok := optionalStringParam(request.Params.Arguments, "agents")
	if !ok || agents == "" {
		agents = defaultWaveagentsPath()
	}
	out, _ := optionalStringParam(request.Params.Arguments, "out")
	taskScope, _ := optionalStringParam(request.Params.Arguments, "task_scope")
	if taskScope == "" {
		taskScope = "all"
	}
	emitter, err := resolveSupportArtifact("wp-emit-wave-execution.sh")
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	cliPath, err := resolveSupportArtifact("waveplan-cli")
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	cmd := exec.Command(emitter,
		"--plan", plan,
		"--agents", agents,
		"--task-scope", taskScope,
	)
	if out != "" {
		cmd.Args = append(cmd.Args, "--out", out)
	}
	cmd.Env = append(os.Environ(), "WAVEPLAN_CLI_BIN="+cliPath)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return swimErrorResult(3, strings.TrimSpace(string(raw))), nil
	}
	statePath := plan + ".state.json"
	stateBootstrapped := false
	if mcp.ParseBoolean(request, "bootstrap_state", false) {
		var err error
		stateBootstrapped, err = bootstrapStateFile(statePath, filepath.Base(plan))
		if err != nil {
			return swimErrorResult(3, err.Error()), nil
		}
	}
	if out != "" {
		return swimJSONResult(map[string]any{
			"ok":                 true,
			"output":             out,
			"task_scope":         taskScope,
			"state_bootstrapped": stateBootstrapped,
			"state_path":         statePath,
			"agents":             agents,
		}), nil
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return swimErrorResult(3, fmt.Sprintf("compile output was not JSON: %v", err)), nil
	}
	return swimJSONResult(payload), nil
}

func (s *WaveplanServer) handleSwimNext(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	schedule, err := requiredStringParam(request.Params.Arguments, "schedule")
	if err != nil {
		return swimErrorResult(2, err.Error()), nil
	}
	journal := optionalOrDefault(request.Params.Arguments, "journal", schedule+".journal.json")
	state, err := s.resolveSwimStatePath(request.Params.Arguments, schedule)
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	decision, err := swim.ResolveNextFromPaths(swim.NextOptions{
		SchedulePath: schedule,
		JournalPath:  journal,
		StatePath:    state,
	})
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	return swimJSONResult(decision), nil
}

func (s *WaveplanServer) handleSwimStep(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ackUnknown, _ := optionalStringParam(request.Params.Arguments, "ack_unknown")
	journal, _ := optionalStringParam(request.Params.Arguments, "journal")
	if ackUnknown != "" {
		if journal == "" {
			return swimErrorResult(2, "missing required parameter: journal"), nil
		}
		outcome, ok := optionalStringParam(request.Params.Arguments, "as")
		if !ok || outcome == "" {
			return swimErrorResult(2, "missing required parameter: as"), nil
		}
		if err := swim.AckUnknown(journal, ackUnknown, outcome); err != nil {
			return swimErrorResult(3, err.Error()), nil
		}
		return swimJSONResult(map[string]any{"ok": true, "step_id": ackUnknown, "outcome": outcome}), nil
	}

	schedule, err := requiredStringParam(request.Params.Arguments, "schedule")
	if err != nil {
		return swimErrorResult(2, err.Error()), nil
	}
	if journal == "" {
		journal = schedule + ".journal.json"
	}
	state, err := s.resolveSwimStatePath(request.Params.Arguments, schedule)
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	decision, err := swim.ResolveNextFromPaths(swim.NextOptions{
		SchedulePath: schedule,
		JournalPath:  journal,
		StatePath:    state,
	})
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	seq := mcp.ParseInt(request, "seq", 0)
	if seq > 0 && decision.Row.Seq != seq {
		return swimErrorResult(3, "current cursor step does not match seq"), nil
	}
	stepID, _ := optionalStringParam(request.Params.Arguments, "step_id")
	if stepID != "" && decision.Row.StepID != stepID {
		return swimErrorResult(3, "current cursor step does not match step_id"), nil
	}
	if !mcp.ParseBoolean(request, "apply", false) {
		return swimJSONResult(decision), nil
	}
	report, err := swim.Apply(swim.ApplyOptions{
		SchedulePath: schedule,
		JournalPath:  journal,
		StatePath:    state,
	})
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	return swimJSONResult(report), nil
}

func (s *WaveplanServer) handleSwimRun(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	schedule, err := requiredStringParam(request.Params.Arguments, "schedule")
	if err != nil {
		return swimErrorResult(2, err.Error()), nil
	}
	until, err := requiredStringParam(request.Params.Arguments, "until")
	if err != nil {
		return swimErrorResult(2, err.Error()), nil
	}
	journal := optionalOrDefault(request.Params.Arguments, "journal", schedule+".journal.json")
	state, err := s.resolveSwimStatePath(request.Params.Arguments, schedule)
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	report, err := swim.Run(swim.RunOptions{
		SchedulePath: schedule,
		JournalPath:  journal,
		StatePath:    state,
		Until:        until,
		DryRun:       mcp.ParseBoolean(request, "dry_run", false),
		MaxSteps:     mcp.ParseInt(request, "max_steps", 0),
	})
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	return swimJSONResult(report), nil
}

func (s *WaveplanServer) handleSwimJournal(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	schedule, err := requiredStringParam(request.Params.Arguments, "schedule")
	if err != nil {
		return swimErrorResult(2, err.Error()), nil
	}
	journal := optionalOrDefault(request.Params.Arguments, "journal", schedule+".journal.json")
	view, err := swim.ReadJournalView(journal, schedule, "", mcp.ParseInt(request, "tail", 0))
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	return swimJSONResult(view), nil
}

func (s *WaveplanServer) handleSwimRefine(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	plan, err := requiredStringParam(request.Params.Arguments, "plan")
	if err != nil {
		return swimErrorResult(2, err.Error()), nil
	}
	targets, err := requiredStringSliceParam(request.Params.Arguments, "targets")
	if err != nil {
		return swimErrorResult(2, err.Error()), nil
	}
	profile := optionalOrDefault(request.Params.Arguments, "profile", string(swim.ProfileEightK))
	out, _ := optionalStringParam(request.Params.Arguments, "out")
	invoker, _ := optionalStringParam(request.Params.Arguments, "invoker")

	sidecar, err := swim.Refine(swim.RefineOptions{
		CoarsePlanPath: plan,
		Profile:        swim.RefineProfile(profile),
		Targets:        targets,
		InvokerBin:     invoker,
	})
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	body, err := swim.MarshalSidecar(sidecar)
	if err != nil {
		return swimErrorResult(3, "marshal: "+err.Error()), nil
	}
	if out != "" {
		if err := os.WriteFile(out, body, 0o644); err != nil {
			return swimErrorResult(3, "write: "+err.Error()), nil
		}
		return swimJSONResult(map[string]any{
			"ok":         true,
			"input":      plan,
			"output":     out,
			"profile":    profile,
			"targets":    sidecar.Targets,
			"unit_count": len(sidecar.Units),
		}), nil
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return swimErrorResult(3, fmt.Sprintf("refine output was not JSON: %v", err)), nil
	}
	return swimJSONResult(payload), nil
}

func (s *WaveplanServer) handleSwimRefineRun(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	refine, err := requiredStringParam(request.Params.Arguments, "refine")
	if err != nil {
		return swimErrorResult(2, err.Error()), nil
	}
	dryRun := mcp.ParseBoolean(request, "dry_run", false)
	state, _ := optionalStringParam(request.Params.Arguments, "state")
	if state == "" && !dryRun {
		return swimErrorResult(2, "missing required parameter: state"), nil
	}
	refineJournal, _ := optionalStringParam(request.Params.Arguments, "refine_journal")
	coarseJournal, _ := optionalStringParam(request.Params.Arguments, "coarse_journal")
	workDir, _ := optionalStringParam(request.Params.Arguments, "work_dir")

	report, err := swim.RefineRun(swim.RefineRunOptions{
		RefinePath:        refine,
		RefineJournalPath: refineJournal,
		CoarseJournalPath: coarseJournal,
		StatePath:         state,
		WorkDir:           workDir,
		DryRun:            dryRun,
	})
	if err != nil {
		return swimErrorResult(3, err.Error()), nil
	}
	return swimJSONResult(report), nil
}

func (s *WaveplanServer) handleContextEstimate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	candidateJSON, err := requiredStringParam(request.Params.Arguments, "candidate")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("missing required parameter: candidate - %v", err)), nil
	}

	var candidate contextsize.ContextCandidate
	if err := json.Unmarshal([]byte(candidateJSON), &candidate); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid candidate JSON: %v", err)), nil
	}

	budgetStr := optionalOrDefault(request.Params.Arguments, "budget", "64000:192000")
	budgetParts := strings.Split(budgetStr, ":")
	if len(budgetParts) != 2 {
		return mcp.NewToolResultError("budget must be in format min:max"), nil
	}

	var minTokens, maxTokens int
	if _, err := fmt.Sscanf(budgetParts[0], "%d", &minTokens); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid budget min: %s", budgetParts[0])), nil
	}
	if _, err := fmt.Sscanf(budgetParts[1], "%d", &maxTokens); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid budget max: %s", budgetParts[1])), nil
	}

	budget := contextsize.Budget{
		MinTokens: minTokens,
		MaxTokens: maxTokens,
	}

	baseDir, _ := optionalStringParam(request.Params.Arguments, "base_dir")

	estimator := &contextsize.Estimator{
		BaseDir: baseDir,
	}

	est, err := estimator.Estimate(candidate, budget)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("estimation failed: %v", err)), nil
	}

	output, err := contextsize.EncodeEstimateJSON(est)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(output)), nil
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

func requiredStringSliceParam(args any, name string) ([]string, error) {
	argsMap, ok := args.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid arguments type")
	}
	val, ok := argsMap[name]
	if !ok {
		return nil, fmt.Errorf("missing required parameter: %s", name)
	}
	switch vv := val.(type) {
	case []string:
		if len(vv) == 0 {
			return nil, fmt.Errorf("parameter %s must be a non-empty string array", name)
		}
		return append([]string{}, vv...), nil
	case []any:
		if len(vv) == 0 {
			return nil, fmt.Errorf("parameter %s must be a non-empty string array", name)
		}
		out := make([]string, 0, len(vv))
		for _, item := range vv {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil, fmt.Errorf("parameter %s must be a string array", name)
			}
			out = append(out, s)
		}
		return out, nil
	case string:
		if strings.TrimSpace(vv) == "" {
			return nil, fmt.Errorf("parameter %s must be a non-empty string array", name)
		}
		parts := strings.Split(vv, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("parameter %s must be a non-empty string array", name)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("parameter %s must be a string array", name)
	}
}

func optionalOrDefault(args any, name, fallback string) string {
	if v, ok := optionalStringParam(args, name); ok && v != "" {
		return v
	}
	return fallback
}

func swimJSONResult(v any) *mcp.CallToolResult {
	data, _ := json.Marshal(v)
	return mcp.NewToolResultText(string(data))
}

func swimErrorResult(code int, message string) *mcp.CallToolResult {
	return swimJSONResult(map[string]any{
		"ok":        false,
		"exit_code": code,
		"error":     message,
	})
}

func swimRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return "", fmt.Errorf("could not locate repo root")
}

func bootstrapStateFile(path, planBase string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat state file %q: %w", path, err)
	}
	body, err := json.MarshalIndent(map[string]any{
		"plan":      planBase,
		"taken":     map[string]any{},
		"completed": map[string]any{},
	}, "", "  ")
	if err != nil {
		return false, err
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return false, fmt.Errorf("write state file %q: %w", path, err)
	}
	return true, nil
}

func resolveSupportArtifact(name string) (string, error) {
	exePath, err := os.Executable()
	if err == nil && exePath != "" {
		sibling := filepath.Join(filepath.Dir(exePath), name)
		if st, statErr := os.Stat(sibling); statErr == nil && !st.IsDir() {
			return sibling, nil
		}
	}
	if resolved, lookErr := exec.LookPath(name); lookErr == nil {
		return resolved, nil
	}
	root, rootErr := swimRepoRoot()
	if rootErr == nil {
		candidate := filepath.Join(root, name)
		if st, statErr := os.Stat(candidate); statErr == nil && !st.IsDir() {
			return candidate, nil
		}
	}
	var pathErr *exec.Error
	if errors.As(err, &pathErr) {
		return "", pathErr
	}
	return "", fmt.Errorf("%s not found beside executable, in PATH, or repo root", name)
}

func defaultWaveagentsPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "waveplan-mcp", "waveagents.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "waveplan-mcp", "waveagents.json")
	}
	return filepath.Join(home, ".config", "waveplan-mcp", "waveagents.json")
}

func (s *WaveplanServer) resolveSwimStatePath(args any, schedule string) (string, error) {
	if v, ok := optionalStringParam(args, "state"); ok && v != "" {
		return v, nil
	}
	if s.statePath != "" {
		return s.statePath, nil
	}
	raw, err := os.ReadFile(schedule)
	if err != nil {
		return "", fmt.Errorf("read schedule: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("decode schedule: %w", err)
	}
	execRows, ok := payload["execution"].([]any)
	if !ok {
		return "", fmt.Errorf("could not derive state path")
	}
	for _, rowAny := range execRows {
		row, ok := rowAny.(map[string]any)
		if !ok {
			continue
		}
		invoke, ok := row["invoke"].(map[string]any)
		if !ok {
			continue
		}
		argv, ok := invoke["argv"].([]any)
		if !ok {
			continue
		}
		for i := 0; i+1 < len(argv); i++ {
			cur, ok := argv[i].(string)
			next, okNext := argv[i+1].(string)
			if ok && okNext && cur == "--plan" {
				return next + ".state.json", nil
			}
		}
	}
	return "", fmt.Errorf("could not derive state path")
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

func defaultPlanPath() string {
	usr, err := user.Current()
	if err != nil {
		return "plans/default-execution-waves.json"
	}
	return filepath.Join(usr.HomeDir, ".local", "share", "waveplan", "plans", "default-execution-waves.json")
}

func defaultPlanDir() string {
	usr, err := user.Current()
	if err != nil {
		return "plans"
	}
	return filepath.Join(usr.HomeDir, ".local", "share", "waveplan", "plans")
}

func resolvePlanPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(defaultPlanDir(), path)
}

func main() {
	var planFlag, stateFlag string
	flag.StringVar(&planFlag, "plan", "", "Plan file path or filename (default: ~/.local/share/waveplan/plans/default-execution-waves.json)")
	flag.StringVar(&stateFlag, "state", "", "State file path or filename (default: derived from --plan)")
	flag.Parse()

	planPath := os.Getenv("WAVEPLAN_PLAN")
	if planPath == "" {
		if planFlag != "" {
			planPath = resolvePlanPath(planFlag)
		} else {
			planPath = defaultPlanPath()
		}
	}

	statePath := os.Getenv("WAVEPLAN_STATE")
	if statePath == "" {
		if stateFlag != "" {
			statePath = resolvePlanPath(stateFlag)
		} else {
			statePath = planPath + ".state.json"
		}
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
