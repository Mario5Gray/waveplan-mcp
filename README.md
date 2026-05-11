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
For RAG indexing guidance across generated plans, schedules, journals, logs, and
notes, see [docs/specs/rag-artifact-indexing-policy.md](docs/specs/rag-artifact-indexing-policy.md).

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

This also installs SWIM helper binaries, installs the schedule/journal schemas
under `~/.local/share/waveplan/specs`, and seeds
`~/.config/waveplan-mcp/waveagents.json` if it does not already exist.

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

SWIM ("Schedule, Work, Invoke, Mark") is the deterministic execution layer above
waveplan. It compiles a plan into typed schedule rows, applies one row at a time
under a lock, records an append-only journal, and refuses to advance when state
or dispatch proof is ambiguous.

Use SWIM when you want the operator loop to be explicit and replayable:

```text
compile schedule -> inspect next row -> apply row -> inspect journal -> continue
```

### Start Using SWIM Now

Install first:

```bash
make install
```

Create or review the agent rotation config:

```bash
$EDITOR ~/.config/waveplan-mcp/waveagents.json
```

Compile a schedule from a plan. `--bootstrap-state` creates
`<plan>.state.json` if it is missing, which is the normal first-run path:

```bash
PLAN=docs/plans/2026-05-05-swim-execution-waves.json
SCHEDULE=/tmp/swim-schedule.json

waveplan-cli swim compile-schedule \
  --plan "$PLAN" \
  --out "$SCHEDULE" \
  --bootstrap-state
```

If you do not want to use the installed default agents file, pass one directly:

```bash
waveplan-cli swim compile-schedule \
  --plan "$PLAN" \
  --agents tests/swim/fixtures/waveagents.json \
  --out "$SCHEDULE" \
  --bootstrap-state
```

Inspect the next step without mutation:

```bash
waveplan-cli swim next --schedule "$SCHEDULE"
```

Apply one step:

```bash
waveplan-cli swim step --apply --schedule "$SCHEDULE"
```

Apply until a boundary:

```bash
waveplan-cli swim run --schedule "$SCHEDULE" --until review
waveplan-cli swim run --schedule "$SCHEDULE" --until fix
waveplan-cli swim run --schedule "$SCHEDULE" --until seq:4
waveplan-cli swim run --schedule "$SCHEDULE" --until step:S1_T1.1_finish
```

Inspect recent journal events:

```bash
waveplan-cli swim journal --schedule "$SCHEDULE" --tail 5
```

Validate artifacts:

```bash
waveplan-cli swim validate --kind schedule --in "$SCHEDULE"
waveplan-cli swim validate --kind journal --in "${SCHEDULE%.json}.journal.json"
```

### SWIM Terminology

| Term | Meaning |
|------|---------|
| `schedule` | v2 JSON list of execution rows. Each row has `seq`, `step_id`, `task_id`, `action`, `requires`, `produces`, and `invoke.argv`. |
| `journal` | append-only v1 JSON event log. The `cursor` points at the next schedule row to execute. |
| `state` | waveplan `<plan>.state.json`; task status is derived as `available`, `taken`, `review_taken`, `review_ended`, or `completed`. |
| `dispatch action` | an action that hands work to an external agent CLI and must produce a dispatch receipt: `implement`, `review`, `fix`. |
| `dispatch receipt` | durable proof that the target CLI accepted the prompt, written under `.waveplan/swim/<schedule-basename>/receipts/`. |
| `incomplete_dispatch` | state advanced but receipt is missing; rerun the same SWIM step to redeliver the prompt. |
| `fix` | dispatch action that transitions `review_taken -> taken` and sends prior review stdout to the implementer. |
| `fix round` | preplanned `fix -> review` pair. Step IDs may use `_rN`, e.g. `S3_T1.1_fix_r1`. |

### Step Lifecycle

The standard emitted lifecycle is:

```text
implement   available    -> taken
review      taken        -> review_taken
end_review  review_taken -> review_ended
finish      review_ended -> completed
```

The runtime also supports preplanned fix cycles:

```text
implement   available    -> taken
review      taken        -> review_taken
fix         review_taken -> taken
review_r2   taken        -> review_taken
end_review  review_taken -> review_ended
finish      review_ended -> completed
```

Current `wp-emit-wave-execution.sh` emits the standard lifecycle. If a schedule
contains `fix` rows, SWIM validates and executes them, and `wp-task-to-agent.sh
--mode fix` attaches the prior review stdout via `SWIM_PRIOR_STDOUT_PATH`.
That launcher path depends on a configured waveplan CLI/MCP that supports
`start_fix <task_id>`.

### Dispatch Safety

For `implement`, `review`, and `fix`, SWIM injects these env vars into the
invoked launcher:

```text
SWIM_DISPATCH_RECEIPT_PATH
SWIM_STEP_ID
SWIM_TASK_ID
SWIM_ACTION
SWIM_PRIOR_STDOUT_PATH   # fix only, when a prior review stdout log exists
```

The step is `applied` only when both conditions hold:

```text
runtime state == row.produces.task_status
dispatch receipt exists
```

If state moved but the receipt is missing, SWIM returns `incomplete_dispatch`.
Do not manually advance the journal; rerun:

```bash
waveplan-cli swim step --apply --schedule "$SCHEDULE"
```

### Boundaries and Inquiries

SWIM observes execution and stops at expected boundaries. It does not approve
downstream prompts or grant target-CLI permissions. `--until review` means
"apply safe rows until the review boundary," not "answer permission prompts on
the way." Permission handling lives in adapters, not in SWIM.

`ApplyReport` and `RunReport` carry four fields for boundary classification
and inquiry decoration:

```text
boundary           until_reached | done | max_steps | blocked
                 | incomplete_dispatch | unknown_pending
inquiry_required   true when a downstream party must answer
inquiry_source     adapter | invoke | timeout | unknown
inquiry_hint       free-text actionable hint
```

`boundary` is set on every non-applied row and rolled up on the run report.
`inquiry_*` decorators populate when the dispatch receipt
(`<step>.<attempt>.dispatch.json`) carries `inquiry_required: true`. Env-var
and stdout heuristics are reserved as future fallbacks. Full contract:
[docs/specs/2026-05-05-swim-ops.md](docs/specs/2026-05-05-swim-ops.md#boundaries-and-inquiries).

### Subcommands

`waveplan-cli swim` supports:

- `compile-schedule`: compile a v2 schedule from a plan and agents config.
- `next`: resolve the next safe row without mutation.
- `step`: inspect, apply, or acknowledge an `unknown` event.
- `run`: apply repeatedly until `implement`, `review`, `fix`, `end_review`, `finish`, `seq:N`, or `step:<id>`.
- `journal`: inspect journal events.
- `validate`: validate schedule, journal, or plan JSON.
- `compile-plan-json`: validate and canonicalize an execution-waves plan.
- `refine` / `refine-run`: split coarse units into fine steps and execute them with parent rollup.

Full reference: [docs/specs/2026-05-05-swim-ops.md](docs/specs/2026-05-05-swim-ops.md). Dispatch receipt and fix-cycle design notes: [docs/specs/2026-05-10-swim-fixes.md](docs/specs/2026-05-10-swim-fixes.md).

Runtime artifact layout:

```text
.waveplan/swim/<schedule-basename>/
  swim.lock
  logs/
    <step_id>.<attempt>.stdout.log
    <step_id>.<attempt>.stderr.log
  receipts/
    <step_id>.<attempt>.dispatch.json
```

The default journal path is next to the schedule:

```text
<schedule-basename>.journal.json
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
