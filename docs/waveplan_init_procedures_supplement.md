Supplement to `docs/waveplan_init_procedures.json`.
This file corrects ambiguities, resolves conflicts, and adds implementation contracts.

## 1) Scope and Compatibility
- Keep existing `waveplan_*` tools working (`peek/pop/start_review/end_review/fin/get/deptree/list_plans/version`).
- New plan-authoring surface (`wp plan ...` + MCP authoring tools) is additive.
- If supplement conflicts with base file, this supplement wins.

Compatibility baseline (current server):
- Derived runtime states today: `available`, `taken`, `completed`.
- Review lifecycle today uses timestamps in `taken` entry (`review_entered_at`, `review_ended_at`).
- Plan/status schema sources:
  - `docs/specs/waveplan-plan-schema.json`
  - `docs/specs/waveplan-state-schema.json`

## 2) Canonical Lifecycle (Plan)
Plan lifecycle states:
- `draft`: created, editable.
- `active`: execution started (first `pop` or explicit activate).
- `archived`: read-only, retained.
- `retired`: tombstoned (metadata kept), not hard-deleted in MVP.

Corrections:
- Do **not** hard-delete on retire in MVP.
- `retired` must preserve audit trail and plan metadata for traceability.

CLI additions:
1. `wp plan archive <plan_id>`
2. `wp plan reactivate <plan_id>`
3. `wp plan retire <plan_id> [--force]` (tombstone by default)

MCP additions:
1. `plan_archive`
2. `plan_reactivate`
3. `plan_retire`

## 3) Canonical Lifecycle (Task)
Future canonical task status model:
- `available`, `taken`, `in_review`, `completed`, `skipped`, `blocked`

Valid transitions:
- `available -> taken`
- `taken -> in_review`
- `in_review -> taken` (rework)
- `taken -> completed`
- `in_review -> completed`
- `available -> skipped`
- `taken -> skipped`
- `available -> blocked`
- `blocked -> available`

Rules:
- `completed` is terminal in MVP (no reopen).
- `blocked` task is not dispatchable.
- If downstream depends on blocked task, downstream remains not-dispatchable.

Compatibility bridge (required):
- Existing state file has no explicit `status` field.
- Bridge derive logic:
  - in `completed` map => `completed`
  - in `taken` map + `review_entered_at` set + `review_ended_at` empty => `in_review`
  - in `taken` map => `taken`
  - else => `available`

## 4) Concurrency Contract
All mutating ops require optimistic concurrency:
- Input: `expected_revision` (required for mutating MCP/CLI in v2 tools).
- Server behavior:
  - match => apply mutation, revision +1.
  - mismatch => reject with `REVISION_MISMATCH`.

Locking contract:
- Per-plan file lock.
- Retry backoff: 100ms, 250ms, 500ms, 1s, 2s.
- Max wait default: 5s.
- Exceed wait => `LOCK_TIMEOUT`.

Atomic write contract:
- Write temp file + fsync + rename.
- Never partial-write plan/state/audit files.

## 5) Idempotency Contract
All mutating MCP calls accept optional `idempotency_key` (UUIDv4 recommended).

Required semantics:
- Scope key by `(plan_id, operation_name, idempotency_key)`.
- Same key + same normalized payload => return cached prior result.
- Same key + different payload => reject `IDEMPOTENCY_KEY_REUSED`.
- Cache TTL default: 24h.

Storage:
- In-memory LRU for hot path.
- Optional persisted sidecar: `<plan>.idempotency.json`.

## 6) Error Contract (Unified)
Use one canonical domain error envelope:

```json
{
  "ok": false,
  "error": {
    "code": "REVISION_MISMATCH",
    "message": "expected revision 3, got 5",
    "retryable": true,
    "details": {
      "plan_id": "P-2026-05-04-foo",
      "expected_revision": 3,
      "actual_revision": 5,
      "latest_revision": 5
    }
  }
}
```

Corrections:
- Remove duplicate meaning split (`CONFLICT` vs `REVISION_MISMATCH`). Keep `REVISION_MISMATCH` only.
- Keep machine-parseable `code` stable across CLI and MCP.

Minimum error codes:
- `PLAN_NOT_FOUND`
- `PLAN_LOCKED`
- `TASK_NOT_FOUND`
- `REVISION_MISMATCH`
- `LOCK_TIMEOUT`
- `CYCLE_DETECTED`
- `DEP_VIOLATION`
- `INVALID_STATE_TRANSITION`
- `TASK_HAS_DEPENDENTS`
- `NO_ACTIVE_REVIEW`
- `ALREADY_COMPLETED`
- `IDEMPOTENCY_KEY_REUSED`
- `INVALID_WAVE`
- `SCHEMA_VERSION_UNSUPPORTED`

## 7) Audit Contract
Corrections:
- Avoid `0444` audit file mode claim. Append-only writer needs write permissions.
- Use logical immutability, not filesystem immutability, in MVP.

Format:
- JSONL file `<plan>.audit.jsonl` (one event per line, append-only).

Event shape:
```json
{
  "revision": 7,
  "op": "plan_add_tasks",
  "by": "sigma",
  "at": "2026-05-04T18:25:10Z",
  "plan_id": "P-2026-05-04-foo",
  "task_ids": ["T3.1"],
  "before_hash": "sha256:...",
  "after_hash": "sha256:...",
  "idempotency_key": "8f6f8e0a-..."
}
```

Tooling:
- `wp plan audit <plan_id> [--tail N]`
- `plan_audit`

## 8) Wave / Depth Computation
Correction:
- Formula `wave = max(dep wave)+1` requires topological pass, not plain BFS.

Algorithm:
1. Validate DAG (no cycles, all deps exist).
2. Topological sort.
3. For each node in topo order:
- no deps => `wave=1`, `depth=0`
- else `wave = 1 + max(wave(dep))`
- `depth = 1 + max(depth(dep))`

Validation:
- If user sets explicit wave, enforce `wave >= 1 + max(dep wave)`.
- Violations => `INVALID_WAVE`.

Optional tool:
- `wp plan recompute_waves <plan_id>`

## 9) File Layout and Naming
Canonical dirs:
- plans: `~/.local/share/waveplan/plans/`
- archive: `~/.local/share/waveplan/archive/`

Canonical files:
- plan: `{YYYY-MM-DD}-{slug}-execution-waves.json`
- state: `<plan>.state.json`
- audit: `<plan>.audit.jsonl`
- idempotency (optional): `<plan>.idempotency.json`

Notes:
- Keep name prefix `waveplan` (not `waveplanner`).
- Expand `~` before use.

## 10) MCP Tool Surface (Authoring Additions)
Base additions:
1. `plan_create`
2. `plan_init`
3. `plan_add_tasks`
4. `plan_remove_task`
5. `plan_get`
6. `plan_validate`
7. `plan_suggest_questions`
8. `plan_list`

Optional later additions:
- lifecycle (`plan_archive`, `plan_reactivate`, `plan_retire`)
- state ops (`plan_skip_task`, `plan_block_task`, `plan_reopen_task`)
- reporting (`plan_dashboard`)

`plan_suggest_questions` contract:
- deterministic sort: required fields first, then optional, stable field-name order.
- no already-filled fields.
- enum fields must include full valid options list.

## 11) MCP Response Envelope Guidance
Transport shape may vary by MCP library.
Application payload must be stable:

Success payload:
```json
{ "ok": true, "data": { ... } }
```

Error payload:
```json
{ "ok": false, "error": { "code": "...", "message": "...", "retryable": false, "details": {} } }
```

Rule:
- Do not rely on free-form text parsing for domain outcomes.

## 12) Security and Permissions
Recommended file modes:
- plan/state/idempotency: `0600` or `0640` (same-user/same-group usage).
- audit jsonl: `0600` or `0640`.

Write guards:
- no edits on archived/retired plan unless reactivated.
- no remove with dependents unless explicit mode (`cascade` or `relink`).
- no mutate completed task in MVP.

## 13) Testing Matrix (Required)
1. CRUD: create/init/add/remove/get/validate
2. DAG validation: cycle, missing dep, self-dep
3. revision control: correct expected revision, mismatch behavior
4. lock behavior: contention + timeout
5. idempotency: replay same payload, reject reused key with different payload
6. state transitions: valid/invalid transition table
7. wave/depth compute correctness
8. audit append correctness + hash chain consistency
9. parity: Python CLI vs Go MCP JSON (normalized timestamps)

Recommended targets:
- `make test`
- `make test-parity`
- `make test-golden`

## 14) Rollout Plan
Phase 1 (minimum safe):
- error contract
- expected revision
- atomic write + lock
- audit jsonl

Phase 2:
- authoring tools (`create/init/add/remove/get/validate/suggest_questions/list`)
- idempotency
- wave recompute

Phase 3:
- lifecycle tools
- richer task status model
- dashboard/reporting

## 15) Priority (Corrected)
1. Error + revision contracts
2. Locking + atomic writes
3. Audit jsonl
4. Idempotency
5. Core authoring tools
6. Wave/depth recompute
7. Lifecycle and reporting
8. Full parity suite
