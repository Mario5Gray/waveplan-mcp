# Swim Implementation Plan

> For agentic workers: use `superpowers:executing-plans` or `superpowers:subagent-driven-development` while implementing.

## Goal

Turn execution emission into a durable state machine that is:
- resumable after interruption
- auditable step-by-step
- safe against invalid phase transitions

Current state:
- `wp-emit-wave-execution.sh` emits an ordered `execution[]` script.
- `wp-plan-to-agent.sh` executes lifecycle actions.

Target state:
- `execution[]` rows become typed steps with explicit preconditions/postconditions.
- a `swim journal` tracks applied steps and outcomes.
- a swim runner can compute `next unapplied safe step` deterministically.
- step execution is guarded by state-ownership + revalidation so stale reads cannot silently execute.
- optional deterministic refinement layer can split coarse units into fine executable steps under explicit budget constraints.

## Non-Goals

- Do not redesign core waveplan task state machine in this phase.
- Do not replace existing `waveplan-cli` lifecycle commands.
- Do not add auto-remediation logic for failed steps (only detect, record, and stop).
- Do not replicate dagdir abductive planning or dialect generation inside swim refinement.

## Architecture Boundary

Swim compiler:
- input: `*-execution-waves.json` + `waveagents.json`
- output: `*-swim-schedule.json`

Swim runner:
- input: `*-swim-schedule.json` + plan state + journal sidecar
- output: `next-step`, `step-apply`, `run-until`, and append-only journal events

This keeps plan authoring and task lifecycle state in waveplan, while adding a durable execution timeline above it.

## Dagdir Compatibility Guardrails (No Collision)

`dagdir` remains authoritative for:
- abductive intake and scope expansion
- planner loop and dialect log semantics
- canonical DAG emission
- DAG runtime/projection truth

Swim in waveplan must be constrained to:
- consuming already-emitted execution waves
- deciding next safe lifecycle command
- recording swim journal metadata

Hard boundaries:
- no planner logic in swim
- no dialect parsing/emission in swim
- no DAG mutation by swim
- no shadow planner ledger; journal is execution audit only

Source-of-truth mapping:
- `dagdir`: plan generation truth (why/what graph exists)
- `waveplan`: execution lifecycle truth (who is doing step state)
- `swim`: command chronology truth (what command ran, when, and outcome)

Integration contract with dagdir artifacts:
- swim input must be a waveplan execution plan artifact derived from dagdir output
- swim may reference `dag_id`/artifact paths as metadata only
- swim must never reinterpret or re-topologize dagdir DAG semantics

## Data Contracts

### 1) Swim Schedule v2

Every row must be explicitly typed and self-validating.

```json
{
  "schema_version": 2,
  "plan": "docs/plans/2026-05-04-waveplan-tail-state-hotfix-execution-waves.json",
  "generated_on": "2026-05-05T00:00:00Z",
  "execution": [
    {
      "step_id": "S1.T1.1.implement",
      "seq": 1,
      "task_id": "T1.1",
      "action": "implement",
      "requires": {
        "task_status": "available"
      },
      "produces": {
        "task_status": "taken"
      },
      "invoke": {
        "argv": [
          "wp-plan-to-agent.sh",
          "--mode",
          "implement",
          "--target",
          "codex",
          "--plan",
          "...",
          "--agent",
          "sigma"
        ]
      },
      "wp_invoke": "wp-plan-to-agent.sh --mode implement --target codex --plan ... --agent sigma"
    }
  ]
}
```

Required fields per step:
- `step_id` unique, stable, deterministic
- `seq` strictly increasing integer
- `task_id`
- `action` in: `implement|review|end_review|finish`
- `requires` map (preconditions)
- `produces` map (expected postconditions)
- `invoke.argv` executable argv array (no shell parsing)

Optional compatibility fields:
- `wp_invoke` display/debug string only (must not be used as swim runner execution source)

### 2) Swim Journal Sidecar

Path convention:
- `<schedule>.journal.json`

Structure:

```json
{
  "schema_version": 1,
  "schedule_path": ".../swim-schedule.json",
  "cursor": 4,
  "last_event": {
    "event_id": "E0004",
    "step_id": "S1.T1.1.finish",
    "seq": 4,
    "outcome": "applied",
    "completed_on": "2026-05-05T08:14:11Z"
  },
  "events": [
    {
      "event_id": "E0001",
      "step_id": "S1.T1.1.implement",
      "seq": 1,
      "task_id": "T1.1",
      "action": "implement",
      "attempt": 1,
      "started_on": "2026-05-05T08:01:22Z",
      "completed_on": "2026-05-05T08:02:10Z",
      "outcome": "applied",
      "exit_code": 0,
      "stdout_path": "logs/S1.T1.1.implement.stdout.log",
      "stderr_path": "logs/S1.T1.1.implement.stderr.log",
      "state_before": {"task_status": "available"},
      "state_after": {"task_status": "taken"}
    }
  ]
}
```

Journal rules:
- append-only `events`
- `cursor` points to next unapplied `seq`
- canonical `outcome` enum: `applied|failed|blocked|unknown|waived`
- `cursor` advances only when outcome is `applied` or `waived`
- `failed|blocked|unknown` do not advance cursor
- `waived` requires `operator`, `reason`, and `waived_on` fields in the event
- replay uses latest event per `seq`/attempt; earlier attempts remain immutable audit history

### 2a) State Ownership and Race Closure Contract

To close stale-state execution races, step apply must enforce both ownership and revalidation:

- default mode is single-writer execution ownership:
  - acquire `<plan>.swim.lock` before any `--apply`
  - all swim-driven mutating commands must hold this lock
  - direct helper scripts that mutate waveplan state are unsupported while lock is held
  - safety guarantee is valid only when all mutators honor this lock; otherwise swim runs in best-effort detection mode
- apply transaction protocol:
  1. read live waveplan state snapshot A and compute `state_token_A`
  2. evaluate `requires` against snapshot A
  3. immediately re-read snapshot B and require `state_token_B == state_token_A`
  4. execute `invoke.argv`
  5. re-read snapshot C and validate `produces`
  6. append journal event and advance cursor only on successful postcondition
- if token mismatch occurs before invoke, append `blocked` event and abort apply
- if process crashes between invoke and journal append, first recovery pass must append `unknown` for that seq and require operator action

### 3) Swim Refinement Sidecar (Coarse -> Fine)

Refinement is deterministic decomposition of a coarse unit into fine steps with fixed constraints.

Path convention:
- `<plan>.swim.refine.<profile>.json` (example profile: `8k`)

Structure:

```json
{
  "schema_version": 1,
  "coarse_plan": "docs/plans/2026-05-05-swim-execution-waves.json",
  "profile": "8k",
  "generated_on": "2026-05-08T00:00:00Z",
  "units": [
    {
      "parent_unit": "T2.1",
      "step_id": "F2_T2.1_s1",
      "seq": 1,
      "context_budget": "8k",
      "files_scope": ["internal/swim/engine.go", "internal/swim/journal.go"],
      "depends_on": [],
      "requires": {"task_status": "taken"},
      "produces": {"artifact": "engine+journal skeleton"},
      "invoke": {"argv": ["wp-plan-to-agent.sh", "--mode", "implement", "..."]},
      "command_hint": "wp-plan-to-agent.sh --mode implement ..."
    }
  ]
}
```

Refinement rules:
- refinement may split only *within* a parent unit; cross-parent dependencies are immutable
- refinement must not reorder parent-wave topology
- fine `step_id` format is `F{wave}_{parent}_s{n}` and must be unique per refinement artifact
- `context_budget` is mandatory per fine step
- `command_hint` is optional debug text only; runner must execute `invoke.argv` exclusively
- fine-step completion rolls up to parent unit state only when all child steps are applied/waived

### 4) Canonical Action Semantics

- `implement`: requires `available`, produces `taken`
- `review`: requires `taken`, produces `review_taken`
- `end_review`: requires `review_taken`, produces `review_ended`
- `finish`: requires `review_ended`, produces `completed`

Compatibility note:
- if runtime state lacks `review_taken` vs `review_ended` granularity, swim must infer from review timestamps (`review_entered_at`, `review_ended_at`) in state payload.

## CLI and MCP Surface

### CLI (waveplan-cli)

Add `swim` command group:
- `waveplan-cli swim compile --plan <plan.json> --agents <waveagents.json> --out <schedule.json>`
- `waveplan-cli swim next --schedule <schedule.json> [--journal <journal.json>]`
- `waveplan-cli swim step --schedule <schedule.json> [--journal <journal.json>] [--seq N|--step-id ID] [--apply]`
- `waveplan-cli swim run --schedule <schedule.json> [--journal <journal.json>] --until <implement|review|end_review|finish|seq:N|step:ID>`
- `waveplan-cli swim journal --schedule <schedule.json> [--journal <journal.json>] [--tail N]`
- `waveplan-cli swim refine --plan <coarse-waves.json> --profile <8k> --out <refine.json>`
- `waveplan-cli swim refine-run --refine <refine.json> [--journal <journal.json>] [--apply]`

Behavior:
- default is swim mode (read-only, no command execution).
- `--apply` executes `invoke.argv`, persists logs, writes journal event.

### MCP tools (optional in this phase; required in phase 2)

- `waveplan_swim_compile`
- `waveplan_swim_next`
- `waveplan_swim_step`
- `waveplan_swim_run`
- `waveplan_swim_journal`

## Implementation Phases

### Phase 1: Contracts + Compiler Hardening

Files:
- Modify: `wp-emit-wave-execution.sh`
- Create: `internal/swim/contracts.go`
- Create: `internal/swim/contracts_test.go`
- Create: `docs/specs/swim-schedule-schema-v2.json`
- Create: `docs/specs/swim-journal-schema-v1.json`

Tasks:
- [ ] Add `schema_version`, `step_id`, `seq`, `action`, `requires`, `produces` to emitted rows.
- [ ] Make emission deterministic for `step_id` format (`S<wave>.<task_id>.<action>`).
- [ ] Emit `invoke.argv` as the canonical execution payload; keep `wp_invoke` optional for display only.
- [ ] Add strict validators for schedule schema and monotonic `seq`.
- [ ] Fail compile when duplicate `step_id` or non-deterministic ordering appears.

Acceptance:
- `execution[]` always includes typed actions for all rows.
- same inputs produce byte-equivalent schedule output (deterministic JSON formatting).

### Phase 2: Swim Core + Journal Engine

Files:
- Create: `internal/swim/engine.go`
- Create: `internal/swim/journal.go`
- Create: `internal/swim/state_adapter.go`
- Create: `internal/swim/engine_test.go`
- Create: `internal/swim/journal_test.go`

Tasks:
- [ ] Implement `next-step` resolver using schedule + journal + live state.
- [ ] Implement precondition evaluator (`requires`) and postcondition validator (`produces`).
- [ ] Implement append-only journal writer with optimistic lock semantics.
- [ ] Implement apply-time race closure protocol (snapshot token re-read before invoke + post-invoke postcondition validation).
- [ ] Implement resilient resume: if process dies after command exits but before journal write, detect and mark `unknown` requiring operator confirmation.

Acceptance:
- restart after interruption resumes at first unapplied safe step.
- invalid transition never executes command; event logged as blocked.

### Phase 3: Step Application + Logging

Files:
- Create: `internal/swim/runner.go`
- Create: `internal/swim/runner_test.go`
- Modify: `wp-plan-to-agent.sh` (optional compatibility flags only)

Tasks:
- [ ] Execute step command from `invoke.argv` (no shell eval), with captured stdout/stderr to per-step log files.
- [ ] Persist `exit_code`, timings, and log paths in journal.
- [ ] Add `--apply` and `run --until` behavior with stop-on-failure semantics.
- [ ] Add dry-run mode that emits intended command + gating decision without state mutation.

Acceptance:
- each applied step produces a journal event and log artifacts.
- cursor moves exactly one step per successful apply.

### Phase 4: CLI Integration + Documentation

Files:
- Modify: `waveplan-cli`
- Modify: `README.md`
- Create: `docs/specs/2026-05-05-swim-ops.md`

Tasks:
- [ ] Add `swim` command group to CLI.
- [ ] Provide examples for `compile`, `next`, `step --apply`, `run --until`.
- [ ] Document failure recovery and manual override workflow.

Acceptance:
- end-to-end operator flow works from CLI only.

### Phase 5 (Optional): MCP Tooling

Files:
- Modify: `main.go`
- Modify: `main_test.go`

Tasks:
- [ ] Expose swim operations as MCP tools.
- [ ] Ensure parity between CLI and MCP contracts.

Acceptance:
- MCP tools return deterministic JSON, no mixed plain text payloads.

### Phase 6: Coarse -> Fine Refinement Layer

Files:
- Create: `internal/swim/refine.go`
- Create: `internal/swim/refine_test.go`
- Modify: `waveplan-cli`
- Create: `docs/specs/swim-refine-schema-v1.json`

Tasks:
- [ ] Implement deterministic unit refinement (`parent_unit` -> ordered fine steps) with profile-driven context budget.
- [ ] Enforce immutable cross-parent dependencies and no wave-topology mutation.
- [ ] Emit refinement sidecar JSON and validator.
- [ ] Add `swim refine` and `swim refine-run` commands.
- [ ] Add roll-up semantics: parent unit marked complete only when all fine steps are terminal (`applied|waived`).

Acceptance:
- same coarse input + profile produces byte-equivalent refinement output.
- fine execution cannot violate parent DAG dependency ordering.

## Failure Handling and Recovery

Failure classes:
- precondition failure (blocked)
- invoke failure (non-zero exit)
- ambiguous completion (crash between command completion and journal append)
- stale schedule (plan state drift invalidates pending steps)

Required recovery controls:
- `swim next` must explain why a step is blocked.
- `swim journal --tail` must expose last known event and cursor.
- explicit override command (phase 2+): mark event outcome `waived` with operator note.
- `swim step --ack-unknown <step_id>` must convert `unknown` into either retriable `failed` or operator `waived` before cursor can move past that seq.

## Concurrency and Locking

- journal writes must use file lock + compare-and-swap on cursor.
- `--apply` must hold `<plan>.swim.lock` for the full evaluate->invoke->validate->journal transaction.
- forbid parallel `--apply` against same journal unless explicit `--force-concurrent` (out-of-scope for phase 1-4).
- if swim detects external waveplan mutation during apply transaction, it must emit `blocked` and abort.

## Observability

Per applied step capture:
- start/end timestamps (UTC ISO8601)
- duration_ms
- exit_code
- stdout/stderr log path
- state snapshot before/after

Recommended:
- reuse `langsmith.go` trace hooks to annotate `step_id`, `seq`, `action`, `outcome`.

## Testing Strategy

Unit tests:
- schedule validation (missing action, duplicate step_id, bad seq)
- requires/produces evaluator
- journal append and cursor advancement
- resume behavior after simulated crash

Integration tests:
- compile -> next -> apply -> journal progression across 2 tasks
- blocked finish when review end missing
- restart process and resume next unapplied step

Golden tests:
- deterministic emitted schedule for fixed plan/agents fixture
- deterministic journal event shape
- deterministic refinement emission for fixed coarse plan + profile fixture

## Rollout Plan

1. Land schedule v2 contracts and keep legacy fields for compatibility.
2. Introduce swim read-only commands (`next`, `journal`) first.
3. Gate `--apply` behind explicit flag and docs warning.
4. Migrate automation scripts to swim-driven stepping.
5. Deprecate ambiguous `status`-only schedule rows.
6. Add pilot coarse->fine refinement profile (`8k`) on one phase and validate roll-up behavior.

Dagdir-safe rollout checks:
1. verify no `internal/planner`-equivalent concepts appear in swim code
2. verify swim has no DAG write/mutation path
3. verify journal events do not claim planner authority (execution-only vocabulary)

## Remaining Open Decisions

- Should `status` be retained in schedule rows for backward compatibility, or removed once `requires/produces` exists?
- Is `review_taken` a persisted canonical status or derived transiently from timestamps?
- Should `wp_invoke` remain as optional display/debug text, or be dropped entirely after migration completes?
- Where should step logs live by default (`.waveplan/logs/` vs alongside journal)?

## Locked Decisions for T6 Start

- v1 refinement ships one first-class profile: `8k`.
- `8k` hard limits: `max_tokens <= 8000`, `max_files <= 6`, `max_lines <= 400`.
- `16k` and `custom` profiles are deferred until after `8k` pilot validation.
- refinement output includes optional `command_hint` (debug-only), and parser/runner must refuse to execute it.

## Definition of Done

- Operator can run one command to get the next safe step.
- Operator can apply one step and resume exactly where execution stopped.
- Every applied step is auditable with durable metadata and logs.
- Invalid lifecycle transitions are prevented by explicit precondition checks.
