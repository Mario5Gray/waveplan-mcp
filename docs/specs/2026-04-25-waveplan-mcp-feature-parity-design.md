# Waveplan MCP — Feature Parity Design

**Date:** 2026-04-25
**Scope:** Bring the Go MCP service (`waveplan-mcp`) to feature parity with the Python CLI (`scripts/waveplan`).

## Problem

The MCP service (`main.go`, 579 lines) is a Go rewrite of the Python CLI (`scripts/waveplan`, 555 lines) but is missing several features:

- No JSON output — all tools return plain text strings
- Missing `deptree` mode (topological sort with parallel groups)
- Missing `review_note` on `end_review`
- Missing `git_sha` on `fin`
- `peek`/`pop` return minimal text instead of full task detail
- `get` omits `status`, `plan`, `doc_refs`, `fp_refs`, `review_note`, `git_sha`

## Goals

1. All MCP tools return structured JSON objects.
2. MCP service exposes every feature the CLI has.
3. No regression on existing behavior (same state file format, same plan file format).

## Non-Goals

- Adding new features beyond what the CLI has.
- Changing the state file format.
- Adding parallel execution support.
- Docker/container support for the MCP binary.

## Architecture

### Single binary, single server

The existing `waveplan-mcp` binary serves all tools via stdio MCP. No architectural change — just expanded tool set and richer data structures.

### Data Structures

#### `PlanUnit` (no change)

Already has all fields: `task`, `title`, `kind`, `wave`, `plan_line`, `depends_on`, `doc_refs`, `fp_refs`, `notes`, `command`.

#### `TaskEntry` (add 2 fields)

```go
type TaskEntry struct {
    TakenBy         string `json:"taken_by"`
    StartedAt       string `json:"started_at"`
    ReviewEnteredAt string `json:"review_entered_at"`
    ReviewEndedAt   string `json:"review_ended_at"`
    Reviewer        string `json:"reviewer"`
    ReviewNote      string `json:"review_note,omitempty"`   // NEW
    GitSha          string `json:"git_sha,omitempty"`        // NEW
    FinishedAt      string `json:"finished_at"`
}
```

#### `WaveplanPlan` (no change)

Already has `units`, `tasks`, `doc_index`, `fp_index`.

### Helper Functions (new)

#### `findPlanRef(units, taskID, docIndex, tasks)`

Resolves a task to its plan document reference by matching the parent task's `plan_line` against `doc_index` entries with `kind="plan"`. Returns `{"path": "...", "line": N}` or nil.

#### `resolveDocRefs(refNames, docIndex)`

Resolves doc ref names to full entries: `[{"ref": "...", "path": "...", "line": N}, ...]`.

#### `resolveFpRefs(refNames, fpIndex)`

Resolves fp ref names to full entries: `[{"ref": "...", "fp_id": "..."}, ...]`.

#### `topologicalSort(units)`

Returns `[]struct{TaskID string; Group int}`. Tasks with no dependencies are group 1. Tasks whose dependencies are all in group N are group N+1. Within each group, sorted by wave then task ID.

### Tool Definitions

All tools return JSON via `mcp.NewToolResultText(jsonString)`. Errors use `mcp.NewToolResultError(message)`.

#### `waveplan_peek`

Show next available task without claiming.

**Args:** none

**Success JSON:**
```json
{
  "task_id": "T1.1",
  "task": "T1",
  "title": "...",
  "kind": "...",
  "wave": 1,
  "plan_line": 42,
  "plan_ref": {"path": "...", "line": 42},
  "depends_on": [],
  "doc_refs": [...],
  "fp_refs": [...],
  "notes": [],
  "command": "..."
}
```

**Error JSON:** `{"error": "No available tasks."}`

#### `waveplan_pop`

Claim next available task.

**Args:** `agent` (string, required)

**Success JSON:**
```json
{
  "claimed": true,
  "task_id": "T1.1",
  "task": "T1",
  "title": "...",
  "kind": "...",
  "wave": 1,
  "plan_line": 42,
  "plan_ref": {"path": "...", "line": 42},
  "depends_on": [],
  "doc_refs": [...],
  "fp_refs": [...],
  "notes": [],
  "command": "...",
  "taken_by": "psi",
  "started_at": "2026-04-25 15:30"
}
```

**Error JSON:** `{"error": "No available tasks."}` or `{"error": "..."}`

#### `waveplan_start_review`

Start review for a taken task.

**Args:** `task_id` (string, required), `reviewer` (string, required)

**Success JSON:**
```json
{
  "success": true,
  "task_id": "T1.1",
  "title": "...",
  "reviewer": "sigma",
  "review_entered_at": "2026-04-25 16:00"
}
```

**Error JSON:** `{"error": "..."}`

#### `waveplan_end_review`

End active review.

**Args:** `task_id` (string, required), `review_note` (string, optional)

**Success JSON:**
```json
{
  "success": true,
  "task_id": "T1.1",
  "title": "...",
  "reviewer": "sigma",
  "review_ended_at": "2026-04-25 16:15",
  "review_note": "Looks good"
}
```

**Error JSON:** `{"error": "..."}`

#### `waveplan_fin`

Mark task as completed.

**Args:** `task_id` (string, required), `git_sha` (string, optional)

**Success JSON:**
```json
{
  "success": true,
  "task_id": "T1.1",
  "title": "...",
  "finished_at": "2026-04-25 16:30",
  "git_sha": "abc1234"
}
```

**Error JSON:** `{"error": "..."}`

#### `waveplan_get`

Report tasks filtered by mode.

**Args:** `mode` (string, optional, default: "all")

**Modes:** `all`, `taken`, `open`, `complete`, `task-<id>`, `<agent>`

**Success JSON:**
```json
{
  "tasks": [
    {
      "task_id": "T1.1",
      "title": "...",
      "task": "T1",
      "kind": "...",
      "wave": 1,
      "plan_line": 42,
      "depends_on": [],
      "status": "completed",
      "started_at": "2026-04-25 14:00",
      "finished_at": "2026-04-25 14:45",
      "taken_by": "theta",
      "review_entered_at": "2026-04-25 14:30",
      "review_ended_at": "2026-04-25 14:45",
      "reviewer": "sigma",
      "review_note": "",
      "git_sha": "",
      "plan": {"path": "...", "line": 42},
      "doc_refs": [...],
      "fp_refs": [...]
    }
  ]
}
```

**Empty result:** `{"tasks": []}`

#### `waveplan_deptree` (NEW)

Topological sort with parallel groups.

**Args:** none

**Success JSON:**
```json
{
  "tasks": [
    {
      "task_id": "T1.1",
      "title": "...",
      "task": "T1",
      "kind": "...",
      "wave": 1,
      "plan_line": 42,
      "depends_on": [],
      "status": "available",
      "started_at": null,
      "finished_at": null,
      "taken_by": null,
      "review_entered_at": null,
      "review_ended_at": null,
      "reviewer": null,
      "review_note": null,
      "git_sha": null,
      "plan": {"path": "...", "line": 42},
      "doc_refs": [...],
      "fp_refs": [...],
      "group": 1
    }
  ]
}
```

**Empty result:** `{"tasks": []}`

#### `waveplan_list_plans` (no change)

List available plan files.

**Args:** `plan_dir` (string, optional, default: "docs/plans")

**Success JSON:**
```json
{
  "plans": ["/path/to/plan1.json", "/path/to/plan2.json"]
}
```

**Empty result:** `{"plans": []}`

### Error Handling

All errors return `mcp.NewToolResultError(message)`. The MCP client receives a structured error response.

### State File

No changes. The `.state.json` sidecar format remains identical. New fields (`review_note`, `git_sha`) are stored as empty strings when not provided.

### Testing

Go tests for:

1. `topologicalSort` — no deps → group 1, deps on group 1 → group 2, mixed waves within group
2. `findPlanRef` — matches plan_line, returns nil for unmatched
3. `resolveDocRefs` / `resolveFpRefs` — full resolution
4. `waveplan_get` — each mode (all, taken, open, complete, task-<id>, agent)
5. `waveplan_deptree` — topological sort output with groups
5. `waveplan_fin` with `git_sha` — stores in completed entry
6. `waveplan_end_review` with `review_note` — stores in taken entry
7. `waveplan_peek` / `waveplan_pop` — full JSON output with plan_ref, doc_refs, fp_refs, notes
8. Error cases — missing task, already completed, unmet deps, active review

## Compatibility

- State file format: **unchanged** (new fields are empty-string defaults)
- Plan file format: **unchanged** (all fields already parsed)
- CLI compatibility: **full** — CLI and MCP read/write the same state file