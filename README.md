# waveplan-mcp

A MCP (implemented in Go-lang) service for managing execution waves from `*-execution-waves.json` plan files, which are human-optimized, strucutred objects containing coarse or (but not likely) fine-grain tasks or units of work to accomplish in some (not any) order.

Motivation: mostly because I am learning agentic workflows. I found that orchestrating multiple agents means having to keep state on who what and where at ALL phases of plan->execution, even small deviations can SERIOUSLY derail your project! Waveplan does not attempt to genreate a plan document; it simply executes it. The emergent inner-loop: pop task-> execute task-> review execution -> sign_off_review -> finish is what waveplan is based on.

Notice there is no 'return' loop semantic for re-writes - thats intentional - as I want to remain in control of the deeper inner-processes. Currently, this is coarse-graned as all in-between tasks get rolled up to a waveplan phase (pop, exec, review, sign_off, fin), meaning waveplan is HUMAN optimized.

I set out by makeing this VERY simple. Currently, the main orchestrator IS YOU! There are not enough checks and gates to in waveplan to make full automation happen.
To provide full automation, a seperate project is being worked on now and it's private ATM [dagdir](https://github.com/mario5gray/neuraloops).

## Superpowers

[Superpowers](https://github.com/obra/superpowers) is a framework of agent skills that enhance coding agents with structured workflows — brainstorming, planning, debugging, test-driven development, code review, and more. Skills are invoked contextually to guide agents through disciplined development practices.

## Fiberplane & Drift

[Fiberplane](https://fiberplane.com) provides developer tooling for observability, API development, and agent workflows. It includes [Drift](https://fiberplane.com), a tool for managing documentation to code state drift.

## The Optimal Stack

The recommended toolchain combines these four components into a cohesive agent-first development workflow:

1. **Superpowers** — Agent skills for disciplined development practices.
2. **Fiberplane** — Agent Task/Project Management.
3. **Drift** — Bind Specs/Document <-> Code management.
4. **Waveplan** — Implementation step execution.
5. **txtstore** — Sectioned markdown file store for artifacts.

Together they form a complete stack: Superpowers keeps Agents disciplined and predictable. Plan with Fibreplane; break problem into issues and collaborate. Drift for Documentation Spec to Code Integrity. Waveplan turns specs and plans into code steps for execution with DAG ordering. txtstore provides sectioned markdown storage for implementation notes and review artifacts.  

For a detailed guide on how to use this stack together, see [planstack.md](planstack.md).

## Overview

`waveplan-mcp` is a standalone Go implementation of a 'waveplan' as an MCP (Model Context Protocol) server. It provides a code-implementation management workflow — `peek`, `pop`, `start_review`, `end_review`, `fin`, `get`, `deptree`, `list_plans` — but exposes each command as an MCP tool with structured JSON output.

### Features

- **JSON output** on all tools — no plain-text parsing needed
- **`deptree`** — topological sort with parallel group numbers
- **`review_note`** on `end_review` and **`git_sha`** on `fin`
- **Deterministic output** — sorted task lists, stable ordering across runs
- **State file compatibility** — reads and writes the same `.state.json` sidecar as the Python CLI
- safe parallel read/write ordering via a Queue with lock.
- **unit tests** covering helpers, ordering, and parity

## Quick Start

### Install Artifacts

Install the MCP server and helper scripts into `~/.local/bin`:

```bash
make install
```

Install only one side:

```bash
make install-bin
make install-helpers
```

Remove helper scripts:

```bash
make uninstall-helpers
```

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
        "WAVEPLAN_PLAN": "~/.local/share/waveplan/plans/2026-04-25-txt2art-amiga-execution-waves.json"
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
      "command": "~/.local/bin/waveplan-mcp",
      "args": ["--plan", "~/.local/share/waveplan/plans/2026-04-25-txt2art-amiga-execution-waves.json"]
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

## CLI Interface

`waveplan-cli` is a Python CLI that proxies commands to the `waveplan-mcp` server over stdio. It provides the same interface as the original Python CLI while all logic is delegated to the Go-based MCP server.

```bash
waveplan-cli peek
waveplan-cli pop <agent>
waveplan-cli start_review <task_id> <reviewer>
waveplan-cli end_review <task_id> [review_note]
waveplan-cli fin <task_id> [git_sha]
waveplan-cli get [all|taken|open|complete|deptree|task-<id>|<agent>] [--json]
waveplan-cli deptree
waveplan-cli list_plans [--plan-dir <dir>]
```

Configure the server binary path via `--mcp-bin`, `WAVEPLAN_MCP_BIN` env var, or it auto-detects `~/.local/bin/waveplan-mcp`. Set `--plan` or `WAVEPLAN_PLAN` to specify the plan file; otherwise it auto-detects from `docs/superpowers/plans/`.

### Helper: `wp-emit-wave-execution.sh`

`wp-emit-wave-execution.sh` emits (does not execute) a sequential workflow JSON for plan tasks:

```json
{
  "execution": [
    { "task_id": "T1.1", "wp_invoke": "wp-plan-to-agent.sh ...", "status": "available" },
    { "task_id": "T1.1", "wp_invoke": "wp-plan-to-agent.sh ...", "status": "taken" },
    { "task_id": "T1.1", "wp_invoke": "wp-plan-to-agent.sh ...", "status": "taken" },
    { "task_id": "T1.1", "wp_invoke": "wp-plan-to-agent.sh ...", "status": "completed" }
  ]
}
```

```bash
# emit all tasks from plan order (default)
wp-emit-wave-execution.sh --plan <plan.json> --agents waveagents.json

# emit only currently open tasks
wp-emit-wave-execution.sh --plan <plan.json> --agents waveagents.json --task-scope open

# write output JSON to file
wp-emit-wave-execution.sh --plan <plan.json> --agents waveagents.json --out tmp.json

# override emitted command path (optional)
wp-emit-wave-execution.sh --plan <plan.json> --agents waveagents.json --invoker /opt/tools/wp-plan-to-agent.sh
```

`waveagents.json` supports:
- `agents`: list of `{name, provider}` where provider is `codex|claude|opencode`
- `schedule` (optional): explicit rotation order

Task scope:
- `all` (default): includes every unit/task in `(wave, task_id)` order
- `open`: includes only claimable tasks from `waveplan-cli get open`

### Helper: `wp-plan-to-agent.sh`

`wp-plan-to-agent.sh` is a unified wrapper for agent-dispatch + lifecycle commands.

```bash
# 1) Dispatch implementation task to an agent target
wp-plan-to-agent.sh --mode implement --target codex --plan <plan.json> --agent sigma

# 2) Dispatch review task (owner agent + reviewer agent)
wp-plan-to-agent.sh --mode review --target claude --plan <plan.json> --agent sigma --reviewer psi

# 3) End review with optional note
wp-plan-to-agent.sh --mode review_end --plan <plan.json> --task-id T1.1 --review-note "looks good"

# 4) Mark complete with optional git sha (or DEFERRED)
wp-plan-to-agent.sh --mode fin --plan <plan.json> --task-id T1.1 --git-sha DEFERRED
```

Supported targets for `--target`: `codex`, `claude`, `opencode`.

Use `--dry-run` to print the generated command/prompt without mutating waveplan state.

Portability overrides:
- `WAVEPLAN_CLI_BIN`: path to `waveplan-cli` if not installed in PATH
- `WP_TASK_TO_AGENT_BIN`: path to `wp-task-to-agent.sh`
- `WP_PLAN_TO_AGENT_BIN`: command/path used by `wp-emit-wave-execution.sh`

## SWIM

SWIM ("Schedule, Work, Invoke, Mark") is a deterministic execution layer above waveplan that turns a plan into a typed, journaled, race-safe step sequence. Operators drive it via `waveplan-cli swim`.

Quick start (3 commands):

```bash
waveplan-cli swim compile-schedule \
  --plan docs/specs/swim-ops-examples/plan.json \
  --agents docs/specs/swim-ops-examples/waveagents.json \
  --out /tmp/schedule.json

waveplan-cli swim next --schedule /tmp/schedule.json
waveplan-cli swim step --apply --schedule /tmp/schedule.json
```

Subcommands: `compile-schedule`, `next`, `step` (with `--apply` and `--ack-unknown`), `run` (with `--until` / `--dry-run` / `--max-steps`), `journal`, `validate`, `compile-plan-json`.

Full reference: [docs/specs/2026-05-05-swim-ops.md](docs/specs/2026-05-05-swim-ops.md). Recovery flows for stuck `unknown` events, `lock_busy`, and cursor drift are documented there with copy-paste bash.

Runtime artifact layout (per plan):

```text
.waveplan/swim/<plan-basename>/
  swim.lock                       # advisory flock; content = JSON {pid, started_at, hostname}
  <schedule>.journal.json         # append-only event journal with cursor
  logs/
    <step_id>.<attempt>.stdout.log
    <step_id>.<attempt>.stderr.log
```

## txtstore

`txtstore` is a sectioned markdown file store for waveplan artifacts (implementation notes, review notes, etc.). It uses an MCP server architecture: `txtstore-mcp` (Go MCP server) + `txtstore` (Go CLI proxy).

### Build & Install

```bash
make build-mcp          # builds txtstore-mcp and txtstore
make install-mcp        # installs both to ~/.local/bin/
```

### CLI Usage

```bash
# Append a new section (auto-renames duplicates with -2, -3, ...)
txtstore append notes.md "Section Title" "content here"

# Append with unit/section hierarchy
txtstore append notes.md "OAuth" "notes" --unit "Auth" --section "Security"

# Edit an existing section (replaces content)
txtstore edit notes.md "Section Title" "updated content"

# Help
txtstore --help
```

Environment override: `TXTSTORE_MCP_BIN=/path/to/txtstore-mcp`

### MCP Tools

| Tool | Description |
|------|-------------|
| `txtstore_append` | Append a new section to a markdown file. Auto-renames duplicates with `-2`, `-3`, etc. |
| `txtstore_edit` | Replace an existing section with new content. Creates the section if it doesn't exist. |

### File Format

Sections are stored in markdown with an embedded TOC index between `<!-- INDEX -->` markers:

```markdown
<!-- INDEX -->
- [Section Title](#section-title)
- [Section Title-2](#section-title-2)
<!-- /INDEX -->

## Section Title
Content...

## Section Title-2
More content...
```

Headings use `##` for top-level sections. Optional `--unit` and `--section` create nested hierarchy: `## Auth > Security > OAuth`. The index anchors use the last heading component (after ` > `).

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

Tests covering:
- waveplan: helper functions, taskInfo, topologicalSort, handleFin, deterministic output, deptree ordering
- txtstore: anchor generation, heading hierarchy, index building, append/edit operations, parent directory creation
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
