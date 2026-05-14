# 2026-05-14 SWIM Review Schedule Sidecar

## Problem

The SWIM runtime already understands a `fix` transition (`review_taken -> taken`), but the normal schedule pipeline is still linear:

```text
implement -> review -> end_review -> finish
```

When a reviewer posts findings, the runtime has no deterministic way to materialize those findings into executable follow-up rows. As a result, review comments are observational, while control flow stays bound to the original compiled schedule.

## Locked Decisions

- The base schedule passed by `--schedule` is immutable after compile.
- Review-driven fix rounds are carried in a separate supplemental schedule sidecar.
- The supplemental sidecar is executable schedule state, not commentary.
- The supplemental sidecar path is explicit only: CLI flag first, env fallback second, never guessed.
- `end_review` and `finish` are blocked while unresolved supplemental rows exist for the same unit.

## Artifact Model

### Base Schedule

The base schedule remains the canonical compiled output from `*-execution-waves.json`:

- immutable
- replayable
- diffable
- suitable for byte-stable rebuilds

### Review Schedule Sidecar

A second JSON file carries review-inserted rows. It is ordered, deterministic, and anchored to rows in the base schedule.

Conceptually:

```text
base schedule
  S1 implement
  S2 review
  S3 end_review
  S4 finish

review sidecar
  X1 after S2 -> fix_r1
  X2 after X1 -> review_r2
```

Effective execution order becomes:

```text
S1 implement
S2 review
X1 fix_r1
X2 review_r2
S3 end_review
S4 finish
```

The base file is never rewritten to embed `X*` rows.

## CLI and Environment Contract

The review sidecar location is required for any command that inserts or executes supplemental review-loop rows.

Accepted sources, in order:

1. `--review-schedule <path>`
2. `WAVEPLAN_SCHED_REVIEW`

No other discovery is permitted.

### Fail-Fast Rules

The command must fail with a deterministic error when:

- neither `--review-schedule` nor `WAVEPLAN_SCHED_REVIEW` is set
- the resolved path equals `--schedule`
- the resolved sidecar path is unreadable for commands that require existing content
- the sidecar references anchors not present in the base schedule
- the sidecar contains rows whose preconditions conflict with the base/overlay merged state

## Sidecar Schema Shape

The sidecar should be its own explicit JSON contract, not an overloaded copy of the full base schedule.

Minimum fields:

- `schema_version`
- `base_schedule_path`
- `insertions[]`

Each insertion row needs:

- `id` - stable sidecar row id (`X1`, `X2`, ...)
- `after_step_id` - anchor row in base or previously inserted sidecar row
- `step_id`
- `seq_hint` - deterministic local ordering key inside the sidecar
- `task_id`
- `action`
- `requires.task_status`
- `produces.task_status`
- `invoke`
- `reason`
- `source_event_id` or equivalent audit pointer to the triggering review event

The sidecar is append-only from the operator/runtime point of view.

## Merge Semantics

At load time, SWIM resolves execution against:

1. base schedule rows
2. ordered sidecar insertions

Deterministic merge rules:

- rows are inserted immediately after their anchor
- if multiple rows target the same anchor, order by `seq_hint`, then lexical `id`
- a sidecar row may anchor to another sidecar row in the same file
- cycles in anchor chains are invalid
- merged output is ephemeral runtime state; the base schedule file is unchanged

## Review Loop Semantics

A reviewer finding that requires implementer follow-up materializes a fix loop by appending at least:

```text
fix_r1     review_taken -> taken
review_r2  taken        -> review_taken
```

If a second review also finds issues, another pair can be appended:

```text
fix_r2     review_taken -> taken
review_r3  taken        -> review_taken
```

`end_review` may run only when no pending sidecar rows remain for the unit and the current merged status is `review_taken`.

`finish` may run only after `end_review` has applied in the merged schedule.

## Operator / Tooling Surface

Recommended commands:

```text
waveplan-cli swim insert-fix-loop
waveplan-cli swim run --review-schedule ...
waveplan-cli swim next --review-schedule ...
waveplan-cli swim step --review-schedule ...
waveplan-cli swim journal --review-schedule ...
```

`insert-fix-loop` should require:

- `--schedule`
- `--review-schedule`
- `--task`
- `--after-step`
- optional `--round`

The command writes explicit sidecar rows; it does not mutate the base schedule.

## Recovery

Journal/state recovery must treat the merged schedule as the executable frame.

That means:

- cursor resolution uses merged order
- sidecar rows participate in drift adoption and replay checks
- stale attempts to run base `end_review` or `finish` while pending sidecar rows exist must return `blocked`

## waveplan-ps

`waveplan-ps` should accept the same explicit sidecar path so observation matches execution:

```text
waveplan-ps --review-schedule <path>
```

The UI should distinguish base rows from supplemental review rows.

## Non-Goals

- guessed sidecar filenames
- parsing free-form review prose to invent fixes without an explicit insertion action
- mutating the compiled base schedule in place
- implicit cursor jumps that skip sidecar rows
