# Waveplan MCP Service — Usage Guide

## What It Is

`waveplan-mcp` is an MCP (Model Context Protocol) server that exposes the **execution wave plan** as callable tools. It lets agents peek, claim, review, and complete tasks from `*-execution-waves.json` plan files — all through the MCP tool interface, without needing the Python CLI.

It is the Go rewrite of the `scripts/waveplan` CLI, designed specifically for MCP integration.

## Binary

```
.worktrees/waveplan-mcp/waveplan-mcp   # compiled Go binary (darwin/amd64)
```

## Configuration

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `WAVEPLAN_PLAN` | Path to the `*-execution-waves.json` plan file | `docs/superpowers/plans/2026-04-22-controlnet-track-3-backend-execution-waves.json` |
| `WAVEPLAN_STATE` | Path to the `.state.json` sidecar | `<plan_path>.state.json` |

Both are resolved relative to the repo root if not absolute.

### Adding to `.mcp.json`

```json
{
  "mcpServers": {
    "waveplan": {
      "command": "/Users/darkbit1001/workspace/Stability-Toys/.worktrees/waveplan-mcp/waveplan-mcp",
      "args": [],
      "env": {
        "WAVEPLAN_PLAN": "/Users/darkbit1001/workspace/Stability-Toys/docs/superpowers/plans/2026-04-22-controlnet-track-3-backend-execution-waves.json",
        "WAVEPLAN_STATE": "/Users/darkbit1001/workspace/Stability-Toys/docs/superpowers/plans/2026-04-22-controlnet-track-3-backend-execution-waves.json.state.json"
      }
    }
  }
}
```

After adding, restart your MCP client (opencode, Claude Code, etc.) to pick up the new server.

---

## Available Tools

### 1. `waveplan_peek`

Show the next available task **without** claiming it.

**Arguments:** none

**Use when:** You want to see what's next before committing to a task.

**Example:**
```
Tool: waveplan_peek
→ task_id: T1.1
  task: T1
  title: "Set up controlnet backend infrastructure"
  kind: infra
  wave: 1
  plan_line: 42
  depends_on:
  command: source /Users/darkbit1001/miniforge3/bin/activate base && python -m pytest tests/test_controlnet_infra.py -q
```

---

### 2. `waveplan_pop`

Claim the next available task for an agent. This is the primary way to **take ownership** of work.

**Arguments:**
| Name | Required | Description |
|------|----------|-------------|
| `agent` | Yes | Agent name claiming the task (e.g., `theta`, `sigma`, `psi`) |

**Use when:** You are ready to start working on a task.

**Example:**
```
Tool: waveplan_pop
  agent: "psi"
→ Claimed T1.1: Set up controlnet backend infrastructure by psi
  task_id: T1.1
  task: T1
  title: "Set up controlnet backend infrastructure"
  kind: infra
  wave: 1
  plan_line: 42
  command: source /Users/darkbit1001/miniforge3/bin/activate base && python -m pytest tests/test_controlnet_infra.py -q
```

**State written:** `taken.T1.1.taken_by = "psi"`, `taken.T1.1.started_at = "2026-04-25 15:30"`

---

### 3. `waveplan_start_review`

Mark a taken task as entering review.

**Arguments:**
| Name | Required | Description |
|------|----------|-------------|
| `task_id` | Yes | Task ID (e.g., `T1.1`) |
| `reviewer` | Yes | Agent name reviewing the task |

**Use when:** You've finished a task and want another agent to review it.

**Example:**
```
Tool: waveplan_start_review
  task_id: "T1.1"
  reviewer: "sigma"
→ Review started for T1.1: Set up controlnet backend infrastructure by sigma
```

**State written:** `taken.T1.1.review_entered_at`, `taken.T1.1.reviewer = "sigma"`

**Guard:** Fails if the task is not currently taken, or if a review is already active.

---

### 4. `waveplan_end_review`

Mark a review as complete.

**Arguments:**
| Name | Required | Description |
|------|----------|-------------|
| `task_id` | Yes | Task ID (e.g., `T1.1`) |

**Use when:** The reviewer has approved the task.

**Example:**
```
Tool: waveplan_end_review
  task_id: "T1.1"
→ Review ended for T1.1: Set up controlnet backend infrastructure
```

**State written:** `taken.T1.1.review_ended_at`

**Guard:** Fails if no active review exists for the task.

---

### 5. `waveplan_fin`

Mark a task as completed. Moves it from `taken` to `completed`.

**Arguments:**
| Name | Required | Description |
|------|----------|-------------|
| `task_id` | Yes | Task ID (e.g., `T1.1`) |

**Use when:** The task is fully done (and reviewed, if review was started).

**Example:**
```
Tool: waveplan_fin
  task_id: "T1.1"
→ Completed T1.1: Set up controlnet backend infrastructure
```

**State written:** `completed.T1.1` with all timestamps preserved, `taken.T1.1` removed.

**Guards:**
- Fails if task is not in the plan.
- Fails if task is already completed.
- Fails if dependencies are not all completed.
- Fails if there is an active review (must call `end_review` first).

---

### 6. `waveplan_get`

Report tasks filtered by mode.

**Arguments:**
| Name | Required | Description |
|------|----------|-------------|
| `mode` | No | Filter mode (default: `all`) |

**Modes:**

| Mode | Description |
|------|-------------|
| `all` | All taken + completed tasks |
| `taken` | Only currently taken tasks |
| `open` | Only available (not taken, not completed, deps met) tasks |
| `complete` | Only completed tasks |
| `task-<id>` | Single task lookup (e.g., `task-T1.1`) |
| `<agent>` | All tasks for a specific agent (taken + completed) |

**Examples:**
```
Tool: waveplan_get
  mode: "open"
→ T1.2, Set up controlnet model loading
  started: —
  finished: —
  depends_on: T1.1

Tool: waveplan_get
  mode: "taken"
→ T1.1, Set up controlnet backend infrastructure
  started: 2026-04-25 15:30
  finished: —
  by: psi

Tool: waveplan_get
  mode: "psi"
→ T1.1, Set up controlnet backend infrastructure
  started: 2026-04-25 15:30
  finished: —
  by: psi
  entered review: 2026-04-25 16:00
  ended review: 2026-04-25 16:15
  reviewer: sigma
```

---

### 7. `waveplan_list_plans`

List available `*-execution-waves.json` plan files.

**Arguments:**
| Name | Required | Description |
|------|----------|-------------|
| `plan_dir` | No | Directory to search (default: `docs/superpowers/plans`) |

**Example:**
```
Tool: waveplan_list_plans
→ /Users/darkbit1001/workspace/Stability-Toys/docs/superpowers/plans/2026-04-22-controlnet-track-3-backend-execution-waves.json
```

---

## Typical Workflow

```
1. peek              → See what's next
2. pop <agent>       → Claim the task
3. ... work ...      → Implement, test, commit
4. start_review <task_id> <reviewer>  → Request review
5. ... review ...    → Reviewer checks work
6. end_review <task_id>                 → Approve review
7. fin <task_id>                     → Mark complete
8. get                               → Verify status
```

### Minimal Workflow (no review)

```
1. peek
2. pop <agent>
3. ... work ...
4. fin <task_id>
5. get
```

---

## State File

The state file is a JSON sidecar auto-created on first `pop`:

```json
{
  "plan": "2026-04-22-controlnet-track-3-backend-execution-waves.json",
  "taken": {
    "T1.2": {
      "taken_by": "psi",
      "started_at": "2026-04-25 15:30",
      "review_entered_at": "2026-04-25 16:00",
      "reviewer": "sigma",
      "review_ended_at": "2026-04-25 16:15"
    }
  },
  "completed": {
    "T1.1": {
      "taken_by": "theta",
      "started_at": "2026-04-25 14:00",
      "review_entered_at": "2026-04-25 14:30",
      "reviewer": "sigma",
      "review_ended_at": "2026-04-25 14:45",
      "finished_at": "2026-04-25 14:45"
    }
  }
}
```

**Key properties:**
- State is **persisted to disk** after every tool call.
- State survives server restarts (loaded from disk on startup).
- `taken` entries are removed when `fin` is called (moved to `completed`).
- Timestamps use `YYYY-MM-DD HH:MM` format.

---

## Task Availability Logic

A task is **available** (can be popped) when:
1. It is not in `taken` and not in `completed`.
2. All entries in `depends_on` are in `completed`.

Tasks are selected by **lowest wave first**, then by task ID (alphabetical).

---

## Quick Reference Card

| Action | Tool | Args |
|--------|------|------|
| See next task | `waveplan_peek` | (none) |
| Claim task | `waveplan_pop` | `agent` |
| Start review | `waveplan_start_review` | `task_id`, `reviewer` |
| End review | `waveplan_end_review` | `task_id` |
| Complete task | `waveplan_fin` | `task_id` |
| List tasks | `waveplan_get` | `mode` (all/taken/open/complete/task-<id>/<agent>) |
| List plans | `waveplan_list_plans` | `plan_dir` (optional) |

---

## Troubleshooting

### "No available tasks"
All tasks are either taken, completed, or blocked by unmet dependencies. Run `waveplan_get open` to see what's available, or `waveplan_get taken` to see what's in progress.

### "Task not found in plan"
The task ID doesn't exist in the current plan file. Verify with `waveplan_get task-<id>`.

### "Task is not currently taken"
The task is not in the `taken` set. You may need to `pop` it first.

### "Review already started" / "No active review"
You can only have one active review per task. End the current review before starting a new one.

### "Has an active review. End review before completing."
You must call `end_review` before `fin` if a review was started.

### "Has incomplete dependencies"
All `depends_on` tasks must be in `completed` before a task can be finished.

---

## Relationship to `scripts/waveplan` CLI

The MCP service is the Go equivalent of the Python CLI (`scripts/waveplan`). They share the same state file format and plan file format, so they are **fully compatible** — you can use the CLI and MCP tools interchangeably on the same plan.

| CLI | MCP Tool |
|-----|----------|
| `waveplan peek` | `waveplan_peek` |
| `waveplan pop <agent>` | `waveplan_pop` |
| `waveplan start_review <id> <reviewer>` | `waveplan_start_review` |
| `waveplan end_review <id>` | `waveplan_end_review` |
| `waveplan fin <id>` | `waveplan_fin` |
| `waveplan get [mode]` | `waveplan_get` |
| `waveplan list_plans` | `waveplan_list_plans` |