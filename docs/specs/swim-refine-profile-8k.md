# SWIM Refinement Profile — `8k`

Profile contract for the v1 refinement compiler. v1 ships **only** this profile; later phases may add `16k`, `custom`, etc.

The compiler MUST refuse to emit a fine step that violates any of these limits.

## Hard limits

| limit | value | enforced by |
|---|---|---|
| `max_tokens` | ≤ 8000 | compiler estimates token count of `files_scope` content + `invoke.argv` joined; rejects over budget |
| `max_files` | ≤ 6 | length of `files_scope` array |
| `max_lines` | ≤ 400 | sum of source-line counts across `files_scope` files (when files exist on disk at compile time; otherwise compiler emits a warning and proceeds) |

## Refinement rules (re-stated from spec §3)

1. Refinement may split *only within* a parent unit. Each fine step's `parent_unit` is exactly one coarse unit ID.
2. Cross-parent dependencies are **immutable**. The coarse plan owns inter-task ordering; refinement must not introduce, remove, or reorder cross-parent edges. Fine-step `depends_on` entries must reference siblings under the same `parent_unit`.
3. Parent-wave topology must not be reordered. If coarse plan says T2.1 is wave 2, every fine step under T2.1 inherits wave 2.
4. `context_budget` is mandatory per fine step. v1: always `"8k"`.
5. Parent unit is marked complete in the journal **only** when every fine step under it reaches a terminal outcome (`applied` or `waived`).

## step_id format

```
F{wave}_{parent_unit}_s{n}
```

- `F` prefix distinguishes from coarse `S{wave}_{...}` step_ids
- `wave` is the parent_unit's wave (inherited)
- `parent_unit` is the literal coarse unit ID (e.g. `T2.1`)
- `n` is 1-indexed sequential ordinal within the parent

Examples:
- `F2_T2.1_s1` — first fine step under coarse unit T2.1 in wave 2
- `F2_T2.1_s2` — second fine step under T2.1
- `F3_T3.2_s1` — first fine step under T3.2 in wave 3

The format is parser-trivial (single split on `_` with the parent_unit's `.` left intact via positional decoding) and avoids the period-collision ambiguity coarse step_ids fixed in T1.3.

## `command_hint`

Optional. If present, MUST be a shell-quoted string equivalent to `invoke.argv` (compiler emits via `shlex.join` for parity). The runner MUST refuse to execute `command_hint` directly; it is debug-only.

## Frame contract back to coarse plan

The refinement sidecar's `plan_ref` (when present) MUST match the coarse plan's `plan_version` and `plan_generation`. The runner refuses to apply a refinement whose `plan_ref` does not pair with the loaded coarse plan, mirroring the schedule v2 frame contract.

## Out of scope (v1)

- `16k`, `32k`, custom profiles
- Token estimator backed by a real tokenizer (v1 uses a heuristic: ~4 chars per token over `files_scope` content)
- Cross-parent dependency rewiring
- Per-profile invoke transformation (e.g. tool selection by budget)
