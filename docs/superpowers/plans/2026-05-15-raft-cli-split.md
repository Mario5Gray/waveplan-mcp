# Raft CLI Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the user-facing SWIM CLI out of `waveplan-cli` into a dedicated Python CLI named `raft`, while keeping `waveplan-mcp` as the only MCP server and preserving the existing `waveplan_swim_*` tool names.

**Spec:** [2026-05-15-raft-cli-split-design.md](/Users/darkbit1001/workspace/waveplan-mcp/docs/superpowers/specs/2026-05-15-raft-cli-split-design.md)

**Architecture:** The boundary changes at the CLI layer, not the core. `waveplan-mcp` remains authoritative for both Waveplan and SWIM. `raft` becomes the canonical SWIM CLI by proxying MCP calls into `waveplan-mcp`. `waveplan-cli` becomes Waveplan-only and keeps `context estimate`. Because installed helper scripts are copied directly into `~/.local/bin` and the repo has no Python packaging layer, this cut should tolerate a small amount of duplicated MCP client boilerplate between `waveplan-cli` and `raft` rather than introducing a new Python packaging/install system.

**Important scope correction:** The current SWIM MCP surface is not at parity with the existing `waveplan-cli swim ...` surface. `raft` cannot be a true MCP proxy until `waveplan-mcp` exposes the missing operations and arguments:

- missing MCP coverage for `compile-plan-json`
- missing MCP coverage for `validate`
- missing MCP coverage for `insert-fix-loop`
- missing `review_schedule` parameters on `waveplan_swim_next`, `waveplan_swim_step`, `waveplan_swim_run`, and `waveplan_swim_journal`
- missing `artifact_root` parameters on `waveplan_swim_step` and `waveplan_swim_run`

The implementation order below reflects that dependency.

**Files:**
- Create: `raft`
- Create: `internal/swim/planjson.go`
- Create: `internal/swim/planjson_test.go`
- Create: `internal/swim/review_sidecar_insert.go`
- Create: `internal/swim/review_sidecar_insert_test.go`
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `waveplan-cli`
- Modify: `Makefile`
- Modify: `README.md`
- Modify: `tests/swim/test_t4_1_cli_scaffold.sh`
- Modify: `tests/swim/test_t4_2_cli_wired.sh`
- Modify: `tests/swim/test_t4_3_docs_present.sh`
- Modify: `tests/swim/test_installed_anydir.sh`
- Modify: `tests/swim/test_t7_2_insert_fix_loop.sh`
- Modify: `tests/swim/test_t7_3_review_schedule_passthrough.sh`
- Modify: `tests/swim/test_compile_plan_json.sh`

---

## Task 1: Bring the SWIM MCP surface to CLI parity

**Why first:** `raft` is supposed to proxy through `waveplan-mcp`, not keep local SWIM execution logic. The server must expose the missing surface before the CLI can be split cleanly.

- [ ] **Step 1: Add coverage tests for SWIM MCP tool registration**

Extend `TestCreateTools_IncludesSwimTools` in `main_test.go` so the expected tool set includes the new parity tools:

```text
waveplan_swim_compile_plan_json
waveplan_swim_validate
waveplan_swim_insert_fix_loop
```

Also assert that the existing tool names remain present:

```text
waveplan_swim_compile
waveplan_swim_next
waveplan_swim_step
waveplan_swim_run
waveplan_swim_journal
waveplan_swim_refine
waveplan_swim_refine_run
```

- [ ] **Step 2: Extend `main.go` tool definitions without renaming existing tools**

Required MCP surface after this step:

- existing tools unchanged by name
- `waveplan_swim_next` accepts `review_schedule`
- `waveplan_swim_step` accepts `review_schedule` and `artifact_root`
- `waveplan_swim_run` accepts `review_schedule` and `artifact_root`
- `waveplan_swim_journal` accepts `review_schedule`
- new `waveplan_swim_compile_plan_json`
- new `waveplan_swim_validate`
- new `waveplan_swim_insert_fix_loop`

Do not remove or rename any existing `waveplan_swim_*` tool.

- [ ] **Step 3: Add handler coverage in `main_test.go`**

Add focused handler tests for:

- compile-plan-json success/failure
- validate `kind=plan|schedule|journal`
- insert-fix-loop success/failure
- review-schedule passthrough on `next`, `step`, `run`, `journal`
- artifact-root passthrough on `step` and `run`

These tests should verify JSON result shape and the relevant failure codes (`2` for argument/contract errors, `3` for runtime/validation errors).

- [ ] **Step 4: Run the server-side test targets**

```bash
cd /Users/darkbit1001/workspace/waveplan-mcp
go test ./... -run 'TestCreateTools_IncludesSwimTools|TestHandleSwim' -count=1
go test ./internal/swim/... -count=1
```

Expected:

- new SWIM MCP tools are registered
- existing SWIM MCP tools remain registered
- handler tests pass before the CLI cutover starts

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat(mcp): bring swim tools to raft parity"
```

---

## Task 2: Move the remaining SWIM-only client logic into Go/core packages

**Why:** `compile-plan-json` and `insert-fix-loop` are currently Python-only behaviors in `waveplan-cli`. If `raft` is to be a real MCP proxy, these operations need core implementations behind MCP handlers.

- [ ] **Step 1: Port plan canonicalization into `internal/swim`**

Create `internal/swim/planjson.go` with a deterministic API along these lines:

```go
func CanonicalizePlanJSON(raw []byte) ([]byte, error)
func ValidatePlanJSON(raw []byte) error
```

Behavior must match the current `waveplan-cli swim compile-plan-json` contract:

- validate required top-level fields
- enforce referential integrity for `tasks`, `units`, `doc_index`, `fp_index`
- preserve all input fields
- emit deterministic key ordering
- return indented JSON identical for byte-identical logical input

- [ ] **Step 2: Port deterministic review-sidecar insertion into `internal/swim`**

Create `internal/swim/review_sidecar_insert.go` with a dedicated API for the current `insert-fix-loop` behavior. The implementation must preserve the current invariants:

- base schedule must be readable and valid
- sidecar base path must match schedule
- inserted IDs remain deterministic
- `fix_rN` and `review_rN+1` rows keep current naming and ordering behavior
- output sidecar remains canonicalized

- [ ] **Step 3: Add regression tests before wiring handlers**

Cover at minimum:

- `compile-plan-json` golden success path
- the current invalid plan cases from `tests/swim/test_compile_plan_json.sh`
- insert-fix-loop happy path
- missing sidecar / wrong base schedule / bad anchor failures
- deterministic round advancement on repeated insertion

Where possible, reuse or mirror the existing shell fixture data instead of inventing new cases.

- [ ] **Step 4: Wire the new core functions into the MCP handlers**

`main.go` handlers for `waveplan_swim_compile_plan_json`, `waveplan_swim_validate`, and `waveplan_swim_insert_fix_loop` should call the Go/core implementations directly. Avoid adding shell-outs or Python subprocess dependencies to the server.

- [ ] **Step 5: Verify the new core behavior**

```bash
go test ./internal/swim -run 'TestCanonicalizePlanJSON|TestInsertFixLoop' -count=1
go test ./... -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/swim main.go main_test.go
git commit -m "feat(swim): move raft-only cli logic into core packages"
```

---

## Task 3: Add `raft` as the canonical SWIM CLI

**Why:** Once MCP parity exists, the SWIM CLI can be separated cleanly without creating a second authority.

- [ ] **Step 1: Create `raft` as a Python MCP proxy**

`raft` should:

- use the same stdio MCP client pattern as `waveplan-cli`
- speak only to `waveplan-mcp`
- expose these top-level subcommands:

```text
compile-plan-json
compile-schedule
next
step
run
journal
insert-fix-loop
validate
refine
refine-run
```

- preserve the current flag spellings and exit-code behavior of `waveplan-cli swim ...`
- keep `--tail-logs` as a client-side convenience after successful `step --apply`

- [ ] **Step 2: Map each `raft` subcommand to MCP**

Expected MCP mapping:

- `compile-plan-json` -> `waveplan_swim_compile_plan_json`
- `compile-schedule` -> `waveplan_swim_compile`
- `next` -> `waveplan_swim_next`
- `step` -> `waveplan_swim_step`
- `run` -> `waveplan_swim_run`
- `journal` -> `waveplan_swim_journal`
- `insert-fix-loop` -> `waveplan_swim_insert_fix_loop`
- `validate` -> `waveplan_swim_validate`
- `refine` -> `waveplan_swim_refine`
- `refine-run` -> `waveplan_swim_refine_run`

Handle path normalization in the client so the user-facing contract remains stable.

- [ ] **Step 3: Add CLI scaffold tests for `raft`**

Update the current SWIM CLI shell tests so they target `raft` as the primary command:

- `tests/swim/test_t4_1_cli_scaffold.sh`
- `tests/swim/test_t4_2_cli_wired.sh`
- `tests/swim/test_t7_2_insert_fix_loop.sh`
- `tests/swim/test_t7_3_review_schedule_passthrough.sh`
- `tests/swim/test_compile_plan_json.sh`
- `tests/swim/test_installed_anydir.sh`

The assertions should check:

- `raft --help` exposes the SWIM surface
- per-command `--help` includes the same flags as before
- JSON outputs and exit codes remain compatible

- [ ] **Step 4: Run the `raft` CLI test matrix**

```bash
bash tests/swim/test_t4_1_cli_scaffold.sh
bash tests/swim/test_t4_2_cli_wired.sh
bash tests/swim/test_t7_2_insert_fix_loop.sh
bash tests/swim/test_t7_3_review_schedule_passthrough.sh
bash tests/swim/test_compile_plan_json.sh
bash tests/swim/test_installed_anydir.sh
```

- [ ] **Step 5: Commit**

```bash
git add raft tests/swim
git commit -m "feat(cli): add raft as the standalone swim cli"
```

---

## Task 4: Remove the `swim` subtree from `waveplan-cli`

**Why:** The separation is not real until `waveplan-cli` no longer owns or advertises SWIM.

- [ ] **Step 1: Remove SWIM parsing and dispatch from `waveplan-cli`**

Delete:

- the `swim` parser subtree
- SWIM-specific dispatch branches
- SWIM-only helper functions that are no longer needed there

Keep:

- coarse Waveplan commands
- `context estimate`
- MCP client setup for Waveplan tools

- [ ] **Step 2: Verify the remaining `waveplan-cli` surface**

Required outcome:

- `waveplan-cli --help` does not mention `swim`
- `waveplan-cli` still supports `peek`, `pop`, `start_review`, `start_fix`, `end_review`, `fin`, `get`, `list_plans`, `deptree`, `version`
- `waveplan-cli context estimate ...` still works

- [ ] **Step 3: Add a negative regression check**

Update or add a shell assertion that `waveplan-cli swim ...` now fails at argument parsing, so the split cannot silently regress back into a double surface.

- [ ] **Step 4: Run focused verification**

```bash
python ./waveplan-cli --help
python ./waveplan-cli context estimate --help
python ./waveplan-cli swim --help
```

Expected:

- first two commands exit `0`
- `waveplan-cli swim --help` exits non-zero and reports unknown/invalid command usage

- [ ] **Step 5: Commit**

```bash
git add waveplan-cli
git commit -m "refactor(cli): remove swim subtree from waveplan-cli"
```

---

## Task 5: Packaging, docs, and cutover

**Why:** The CLI split is not finished until install paths and user-facing docs point to the new command.

- [ ] **Step 1: Update `Makefile` helper installation**

Required changes:

- add `raft` to `HELPER_SCRIPTS`
- ensure `make install` places `raft` in `~/.local/bin`
- ensure `make uninstall-helpers` removes `raft`

- [ ] **Step 2: Rewrite SWIM-facing docs to use `raft`**

Update `README.md` and any nearby CLI references so:

- SWIM examples use `raft ...`
- `waveplan-cli` docs describe only coarse Waveplan commands plus `context estimate`
- MCP docs still describe SWIM as reachable through `waveplan-mcp`
- docs do not imply a second server or a second state authority

- [ ] **Step 3: Update docs-presence tests**

At minimum, update `tests/swim/test_t4_3_docs_present.sh` so the documented CLI surface matches the new split.

- [ ] **Step 4: Run packaging and docs verification**

```bash
make install
raft --help
waveplan-cli --help
bash tests/swim/test_t4_3_docs_present.sh
```

- [ ] **Step 5: Commit**

```bash
git add Makefile README.md tests/swim/test_t4_3_docs_present.sh
git commit -m "docs(cli): cut swim usage over to raft"
```

---

## Final Verification

- [ ] **Step 1: Run the full Go suite**

```bash
cd /Users/darkbit1001/workspace/waveplan-mcp
go test ./... -count=1
```

- [ ] **Step 2: Run the full SWIM shell suite affected by the split**

```bash
bash tests/swim/test_t4_1_cli_scaffold.sh
bash tests/swim/test_t4_2_cli_wired.sh
bash tests/swim/test_t4_3_docs_present.sh
bash tests/swim/test_installed_anydir.sh
bash tests/swim/test_t7_2_insert_fix_loop.sh
bash tests/swim/test_t7_3_review_schedule_passthrough.sh
bash tests/swim/test_t7_4_review_schedule_observer_fixture.sh
bash tests/swim/test_compile_plan_json.sh
```

- [ ] **Step 3: Perform manual surface checks**

```bash
python ./waveplan-cli --help
python ./raft --help
python ./raft next --help
python ./raft step --help
python ./raft run --help
python ./raft journal --help
python ./raft insert-fix-loop --help
```

- [ ] **Step 4: Confirm acceptance criteria**

The split is only complete when all of the following are true:

- `waveplan-mcp` is still the only MCP server
- existing `waveplan_swim_*` tool names still exist
- `raft` is the canonical SWIM CLI
- `waveplan-cli` no longer exposes `swim`
- SWIM examples and tests use `raft`
- no SWIM behavior regresses at the JSON contract level

---

## Execution Options

1. **Subagent-Driven Execution** (recommended)  
   Use `superpowers:subagent-driven-development` to execute the plan in parallel where the write sets are disjoint.

2. **Inline Execution**  
   Execute this plan directly in one session with `superpowers:executing-plans`, updating the checklist as each step lands.
