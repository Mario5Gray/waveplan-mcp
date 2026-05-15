# 2026-05-10 SWIM Fixes

## Problem

`implement` and `review` steps currently bundle two effects:

1. mutate waveplan state (`pop`, `start_review`)
2. deliver a handoff prompt to an external agent CLI

If state mutation succeeds and prompt delivery fails, SWIM sees the produced
state but has no durable proof that the handoff actually happened. That creates
the "truth problem": cursor/state can be reconciled while the real work never
reached the agent.

## Locked Decisions

- Primary recovery model: durable dispatch receipt, not blind journal rewind.
- New operator-facing status: `incomplete_dispatch`.
- Retry behavior:
  - `produced state + receipt present` => idempotent adopt, cursor may advance
  - `produced state + receipt missing` => retry dispatch, do not silently adopt
  - `state not at produced value` => existing blocked/failed handling
- Manual `--stepback` is useful but deferred. It is not the primary answer to
  partial external side effects.

## Receipt Model

For dispatching actions (`implement`, `review`), SWIM will derive a receipt path
under the schedule artifact tree:

`.waveplan/swim/<schedule-basename>/receipts/<step_id>.<attempt>.dispatch.json`

The prompt-delivery helper (`wp-agent-dispatch.sh`) writes this file only after
the target CLI accepts the prompt successfully. Legacy direct flows through
`wp-task-to-agent.sh` still end at the same helper.

Receipt payload v1:

- `step_id`
- `task_id`
- `action`
- `target`
- `agent`
- `mode`
- `task_source`
- `delivered_on`
- `tool`
- `ok: true`

## Runtime Semantics

### Normal dispatch

1. SWIM writes the in-flight journal event.
2. SWIM invokes the launcher with `SWIM_DISPATCH_RECEIPT_PATH=<derived path>`.
3. Launcher writes receipt after successful prompt delivery.
4. SWIM validates:
   - state postcondition satisfied
   - receipt exists for dispatch steps
5. Only then is the event terminal `applied` and the cursor advanced.

### Recovery / continue

If a retry starts on a dispatch step and runtime state already equals the
step's `produces.task_status`:

- if a receipt already exists for the step => adopt as applied
- if no receipt exists => re-dispatch the prompt against the already-claimed
  task, then require receipt creation before advancing

This supports "continue after partial failure" without repeating `pop` or
pretending the work was delivered when it was not.

## Implementation Scope

1. Add receipt helpers to SWIM runtime.
2. Mark dispatch rows (`implement`, `review`) as receipt-bearing actions.
3. Pass receipt path into launcher environment.
4. Require receipt presence on success for dispatch actions.
5. On drift where actual state already equals produced state:
   - dispatch action + receipt missing => retry dispatch
   - non-dispatch or receipt present => idempotent adopt
6. Surface `incomplete_dispatch` from `Apply()` / CLI when delivery is still
   unproven after an attempt.

## Fix-Cycle Action

### Motivation

The state machine is strictly forward:

```text
available → implement → taken → review → review_taken → end_review → review_ended → finish → completed
```

When a review finds issues that the reviewer cannot fix in-band during its own
turn, there is no path back to the implementer. The task is stranded at
`review_taken` with `end_review` blocked.

### Decision

Add a `fix` action as a dispatch step that transitions `review_taken → taken`.
This returns the task to the implementer with reviewer findings attached, and
allows the cycle to repeat.

New state transition:

```text
review_taken → fix → taken
```

`fix` is a receipt-bearing dispatch action (same model as `implement` and
`review`). Cursor does not advance until a dispatch receipt is written.

### Schedule Shape

Fix rounds are pre-planned by the schedule compiler, not discovered at runtime.
A task with one fix round has this row sequence:

```text
seq 1: implement   available    → taken
seq 2: review      taken        → review_taken
seq 3: fix         review_taken → taken
seq 4: review      taken        → review_taken
seq 5: end_review  review_taken → review_ended
seq 6: finish      review_ended → completed
```

Multiple fix rounds stack additional `fix → review` pairs. The compiler decides
the number of pre-planned rounds based on task complexity or a flag; the operator
is not expected to edit the schedule by hand.

Step IDs must be unique per row even when the same action appears multiple times,
e.g. `S2_T1.1_review_r1`, `S4_T1.1_review_r2`.

### Dispatch Path

Canonical scheduled execution uses:

```text
wp-plan-step.sh -> wp-agent-dispatch.sh
```

`wp-task-to-agent.sh` remains as a compatibility wrapper for older direct
implement/review/fix entrypoints. It is no longer the canonical scheduled-step
launcher and must not receive `--task-id`.

A `--mode fix` is added alongside `implement` and `review`:

1. `wp-plan-step.sh` validates the selected task and runs `start_fix <task_id>`.
2. Attaches reviewer findings from the prior `review` step's stdout log path
   (passed via `SWIM_PRIOR_STDOUT_PATH` env var set by the runner).
3. `wp-agent-dispatch.sh` builds the fix prompt and delivers it to the target CLI.
4. `wp-agent-dispatch.sh` writes a dispatch receipt after confirmed delivery
   (same path convention as `implement` and `review`).

### Runner Changes

- `statusByAction` in `contracts.go` gains:

  ```go
  "fix": {requires: "review_taken", produces: "taken"},
  ```

- `isDispatchAction` in `dispatch.go` gains `"fix"`.
- `safe_runner.go` sets `SWIM_PRIOR_STDOUT_PATH` in `extraEnv` for `fix` steps,
  derived from the most recent `review` event's `stdout_path` in the journal.

### Fix Receipt Convention

No changes to receipt path convention. Fix receipts follow the same pattern:

`.waveplan/swim/<schedule-basename>/receipts/<step_id>.<attempt>.dispatch.json`

`step_id` uniqueness per row means receipts for different fix rounds do not
collide.

### Out of Scope

- Runtime detection of "fixes required" from reviewer output — the schedule
  compiler pre-plans rounds; SWIM does not parse review output to decide
  branching.
- Dynamic round insertion at runtime — the schedule is immutable after compile.
- `end_review` semantics are unchanged; if no fix rounds are scheduled and the
  reviewer finds issues, the operator must recompile the schedule with fix rounds
  added, or manually rewind state with `--stepback` (still deferred).

## Deferred

- `swim step --stepback` guarded triage tool
- richer receipt/session identifiers per target CLI
- receipt-aware refine-run parity if refine steps begin dispatching through the
  same launcher path
- dynamic fix-round insertion (runtime branching) — requires schedule mutability
  and is explicitly out of scope here
