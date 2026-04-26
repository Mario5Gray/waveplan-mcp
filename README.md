# waveplan-mcp

Go MCP service for managing execution waves from `*-execution-waves.json` plan files.

## Overview

`waveplan-mcp` is a standalone Go implementation of the [waveplan CLI](https://github.com/your-org/waveplan) as an MCP (Model Context Protocol) server. It provides the same task management workflow — `peek`, `pop`, `start_review`, `end_review`, `fin`, `get`, `deptree`, `list_plans` — but exposes each command as an MCP tool with structured JSON output.

### Features

- **JSON output** on all tools — no plain-text parsing needed
- **Full feature parity** with the Python CLI: all filter modes, review workflow, dependency tracking
- **`deptree` mode** — topological sort with parallel group numbers
- **`review_note`** on `end_review` and **`git_sha`** on `fin`
- **Deterministic output** — sorted task lists, stable ordering across runs
- **State file compatibility** — reads and writes the same `.state.json` sidecar as the Python CLI
- **15 unit tests** covering helpers, ordering, and parity

## Quick Start

### Build

```bash
go build -o waveplan-mcp
```

### Run

```bash
WAVEPLAN_PLAN=/path/to/your-plan.json ./waveplan-mcp
```

The server listens on stdio for MCP JSON-RPC requests. Set `WAVEPLAN_STATE` to override the state file path (defaults to `<plan>.state.json`).

### MCP Client Config

Configure `waveplan-mcp` in your MCP client config (e.g. Claude Code `claude.json`):

```json
{
  "mcpServers": {
    "waveplan": {
      "command": "./waveplan-mcp",
      "args": ["--plan", "2026-04-25-txt2art-amiga-execution-waves.json"],
      "env": {
        "WAVEPLAN_PLAN": "/Users/darkbit1001/.local/share/waveplan/plans/2026-04-25-txt2art-amiga-execution-waves.json"
      }
    }
  }
}
```

Or with absolute paths:

```json
{
  "mcpServers": {
    "waveplan": {
      "command": "/Users/darkbit1001/.local/bin/waveplan-mcp",
      "args": ["--plan", "/Users/darkbit1001/.local/share/waveplan/plans/2026-04-25-txt2art-amiga-execution-waves.json"]
    }
  }
}
```

### Example: peek

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"client","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"waveplan_peek","arguments":{}}}
```

Response:

```json
{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"{\"task_id\":\"T1.1\",\"task\":\"T1\",\"title\":\"...\",\"kind\":\"impl\",\"wave\":1,\"plan_line\":20,\"plan_ref\":{\"path\":\"...\",\"line\":20},\"depends_on\":[],\"doc_refs\":[],\"fp_refs\":[],\"notes\":[]}"}]}}
```

## Tools

| Tool | Description |
|------|-------------|
| `waveplan_peek` | Show next available task without claiming |
| `waveplan_pop` | Claim the next available task (`agent` required) |
| `waveplan_start_review` | Start review for a taken task (`task_id`, `reviewer` required) |
| `waveplan_end_review` | End active review (`task_id` required, `review_note` optional) |
| `waveplan_fin` | Mark task as completed (`task_id` required, `git_sha` optional) |
| `waveplan_get` | Report tasks filtered by mode (`mode` optional: `all`, `taken`, `open`, `complete`, `task-<id>`, `<agent>`) |
| `waveplan_deptree` | Show tasks in dependency order with parallel groups |
| `waveplan_list_plans` | List available execution wave plans (`plan_dir` optional) |

## Plan File Format

Plans are JSON files matching `*-execution-waves.json` with this structure:

```json
{
  "schema_version": 1,
  "generated_on": "2026-04-25",
  "plan": {
    "id": "my-feature",
    "title": "My Feature"
  },
  "units": {
    "T1.1": {
      "task": "T1",
      "title": "Task title",
      "kind": "impl",
      "wave": 1,
      "plan_line": 20,
      "depends_on": [],
      "doc_refs": ["plan", "spec"],
      "fp_refs": ["FP-123"],
      "notes": ["Note text"],
      "command": "go build"
    }
  },
  "tasks": {
    "T1": {"plan_line": 20}
  },
  "doc_index": {
    "plan": {"path": "docs/plan.md", "line": 20, "kind": "plan"},
    "spec": {"path": "docs/spec.md", "line": 1, "kind": "spec"}
  },
  "fp_index": {
    "FP-123": "https://your-tracker.com/issues/FP-123"
  },
  "waves": [
    {"wave": 1, "units": ["T1.1"]},
    {"wave": 2, "units": ["T2.1"]}
  ]
}
```

## State File

The state file (`<plan>.state.json`) tracks task lifecycle:

```json
{
  "plan": "my-feature-execution-waves.json",
  "taken": {
    "T1.1": {
      "taken_by": "agent-name",
      "started_at": "2026-04-25 10:00",
      "review_entered_at": "2026-04-25 14:00",
      "review_ended_at": "2026-04-25 14:30",
      "reviewer": "reviewer-name",
      "review_note": "Looks good"
    }
  },
  "completed": {
    "T1.1": {
      "started_at": "2026-04-25 10:00",
      "taken_by": "agent-name",
      "review_entered_at": "2026-04-25 14:00",
      "review_ended_at": "2026-04-25 14:30",
      "reviewer": "reviewer-name",
      "review_note": "Looks good",
      "git_sha": "abc1234",
      "finished_at": "2026-04-25 15:00"
    }
  }
}
```

New fields (`review_note`, `git_sha`) default to empty strings when not provided.

## Workflow

```
peek          → see next available task
pop <agent>   → claim it
# ... work ...
start_review <task_id> <reviewer>  → request review
# ... review ...
end_review <task_id> [review_note] → approve review
fin <task_id> [git_sha]            → mark complete
get [mode]                          → check status
```

## Testing

```bash
go test -v ./...
```

15 tests covering:
- Helper functions: `findPlanRef`, `resolveDocRefs`, `resolveFpRefs`, `nilIfEmpty`
- `taskInfo` JSON structure and marshaling
- `topologicalSort`: no-deps, wave ordering, multi-level dependency chains
- `handleFin` started_at backfill (pop vs no-pop)
- Deterministic output ordering for `get` modes
- `deptree` ordering with group numbers
- `plan` vs `plan_ref` key naming (get vs peek)

## Architecture

- **Single binary, single server** — all tools served via stdio MCP
- **`taskInfo()`** — builds full task detail with `plan_ref`, `doc_refs`, `fp_refs` (used by `peek`/`pop`)
- **`buildTaskEntry()`** — extends `taskInfo()` with status/timestamps, renames `plan_ref` → `plan` for `get`/`deptree` (matches CLI shape)
- **`doDeptree()`** — unlocked helper for topological sort with groups; called by both `handleGet(mode="deptree")` and `handleDeptree` to avoid `sync.Mutex` deadlock
- **State file** — read on startup, written after each mutating tool call

## License

MIT