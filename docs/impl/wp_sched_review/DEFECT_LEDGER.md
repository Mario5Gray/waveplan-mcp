# Defect Ledger: wp_sched_review Feature Review (2026-05-14)

## Table of Contents

- [Summary](#summary)
- [CRITICAL](#critical-3-items)
  - [CRIT-1: CLI wiring dead](#crit-1-cli-wiring-dead----review-schedule-never-passed-to-go-binaries)
  - [CRIT-2: swim journal drops --review-schedule](#crit-2-swim-journal-accepts----review-schedule-flag-but-silently-drops-it)
  - [CRIT-3: --review-schedule hard-required](#crit-3----review-schedule-hard-required-even-when-sidecar-is-absent)
- [HIGH](#high-4-items)
  - [HIGH-1: ResolveNextFromPaths not threaded](#high-1-resolvenextfrompaths-not-threaded-with-reviewschedulepath)
  - [HIGH-2: Verification gap](#high-2-verification-gap----steprunjournal-subcommands-untested-for-fail-fast)
  - [HIGH-3: MCP handlers bypass contract](#high-3-mcp-swim-handlers-bypass-contract-entirely)
  - [HIGH-4: Precondition-conflict fail-fast ownership](#high-4-precondition-conflict-fail-fast-ownership-unclear)
- [MEDIUM](#medium-9-items)
  - [MED-1: Pending-sidecar block ordering](#med-1-pending-sidecar-block-precedes-precondition-block)
  - [MED-2: Sidecar action allowlist](#med-2-sidecar-action-allowlist-not-enforced)
  - [MED-3: Drift/status ordering](#med-3-statusdrift-ordering-interaction-with-pending-sidecar-block)
  - [MED-4: Forward execution scan only](#med-4-pendingsidecarstepfortask-only-consults-forward-execution)
  - [MED-5: swim-journal flag inert](#med-5-swim-journal-flag-advertised-in----help-but-inert)
  - [MED-6: waveplan-ps missing Invoke](#med-6-waveplan-ps-model-misses-invoke-on-insertions)
  - [MED-7: Renderer merged UI](#med-7-renderer-surfaces-sidecar-separately-never-merged-into-unit-table)
  - [MED-8: Untested validator branches](#med-8-untested-validator-branches-in-validatereviewschedulesidecar)
  - [MED-9: Resolver return discarded](#med-9-resolver-return-value-discarded-in-four-places)
- [LOW / MINOR](#low--minor-4-items)
  - [LOW-1: Dead branch in resolver](#low-1-dead-branch-in-resolver)
  - [LOW-2: Control flow safety](#low-2-_resolve_review_schedule_path-control-flow-issue)
  - [LOW-3: seq_hint tie-breaker](#low-3--mergeexecutionwithreviewinsertions-seq_hint-tie-breaker-semantics-unclear)
  - [LOW-4: Seq re-number optimization](#low-4--loadschedules-re-numbers-seq-even-when-no-insertions)
- [Structural Issues](#structural-issues-metadata-cleanup)
- [Verification Gap](#verification-gap)
- [Summary Table](#summary-table)
- [Blocking Chain](#blocking-chain)
- [Notes for Implementer](#notes-for-implementer)
- [Claim Review](#claim-review)

## Summary
Review of wp_sched_review (review-loop scheduling) found 17 unique defects across severity tiers. Consolidated from reviews of T1.3, T2.1, T2.2, T3.2, and T3.3. This ledger deduplicates findings by category and tracks fix status.

---

## CRITICAL (3 items) <a id="critical-3-items"></a>

### CRIT-1: CLI wiring dead — --review-schedule never passed to Go binaries <a id="crit-1-cli-wiring-dead----review-schedule-never-passed-to-go-binaries"></a>
**Severity:** CRITICAL  
**Tasks affected:** T2.1, T2.2, T1.3  
**Evidence:** 
- `waveplan-cli` resolves `--review-schedule` then discards: `_ = _resolve_review_schedule_path(...)` (lines 902, 927, 960, 981)
- Path never appended to `tool_args` for Go subprocess
- `cmd/swim-next/main.go`, `cmd/swim-step/main.go`, `cmd/swim-run/main.go`, `cmd/swim-journal/main.go` define no `--review-schedule` flag
- `mergeExecutionWithReviewInsertions` logic only exercised by unit tests, never by operators

**Fix required:**
- Add `flag.String("review-schedule", "", ...)` to four Go binaries
- Pass into `swim.NextOptions.ReviewSchedulePath` / `RunOptions.ReviewSchedulePath` / `ApplyOptions.ReviewSchedulePath`
- Append `--review-schedule <resolved>` to Python `tool_args` lists in waveplan-cli
- Add end-to-end test asserting merged frame steps appear in `waveplan-cli swim run --dry-run` output

**Status:** **open blocker**

---

### CRIT-2: `swim journal` accepts --review-schedule flag but silently drops it <a id="crit-2-swim-journal-accepts----review-schedule-flag-but-silently-drops-it"></a>
**Severity:** CRITICAL  
**Tasks affected:** T3.2 (M1)  
**Evidence:**
- `cmd/swim-journal/main.go:13` declares flag, line 21 uses `_ = reviewSchedulePath`
- Journal view never merges sidecar rows into output
- Operators see base-only view, ignoring supplemental rows
- Contradicts spec §Recovery: *cursor resolution uses merged order*

**Fix required (choose one):**
1. Implement merge in `ReadJournalView` and thread `reviewSchedulePath` through, OR
2. Remove `--review-schedule` from Go binary + Python wrapper + scaffold test assertions

**Status:** **open blocker**

---

### CRIT-3: --review-schedule hard-required even when sidecar is absent <a id="crit-3----review-schedule-hard-required-even-when-sidecar-is-absent"></a>
**Severity:** CRITICAL  
**Tasks affected:** T3.2 (M2)  
**Evidence:**
- `_resolve_review_schedule_path` defaults `required=True` (line 371)
- All four callers accept default (lines 1399, 1427, 1462, 1485)
- Pre-T3.2 schedules without sidecar now fail: "missing review schedule: set --review-schedule or WAVEPLAN_SCHED_REVIEW"
- Spec: *required only for commands that insert or execute supplemental rows*; `journal` reads only (does not execute)
- Tests patched to inject stub `WAVEPLAN_SCHED_REVIEW` purely to keep flows running (test_installed_anydir.sh:30-34, test_t4_2_cli_wired.sh:10-15)

**Fix required:**
- Change `_resolve_review_schedule_path(..., required=False)` for `next`, `step`, `run`, `journal`
- Keep `required=True` only for `insert-fix-loop`
- Revert test stubs or make them conditional

**Status:** **open blocker**

---

## HIGH (4 items) <a id="high-4-items"></a>

### HIGH-1: ResolveNextFromPaths not threaded with ReviewSchedulePath <a id="high-1-resolvenextfrompaths-not-threaded-with-reviewschedulepath"></a>
**Severity:** HIGH  
**Tasks affected:** T2.2 (HIGH-2)  
**Evidence:**
- `query.go` added `ReviewSchedulePath` field to `NextOptions`
- `cmd/swim-step/main.go:47` calls `ResolveNextFromPaths(swim.NextOptions{SchedulePath, JournalPath, StatePath})` without it
- `swim step` (non-`--apply` path) evaluates against base-only schedule even when sidecar exists
- Pending block is bypassed

**Fix required:**
- Pass `ReviewSchedulePath` into `NextOptions` in `cmd/swim-step/main.go`
- Thread through to `ResolveNextFromPaths`
- Same plumbing as CRIT-1 fix

**Status:** **open blocker** (depends on CRIT-1)

---

### HIGH-2: Verification gap — step/run/journal subcommands untested for fail-fast <a id="high-2-verification-gap----steprunjournal-subcommands-untested-for-fail-fast"></a>
**Severity:** HIGH  
**Tasks affected:** T1.3 (H1)  
**Evidence:**
- `tests/swim/test_t4_1_cli_scaffold.sh:101-149` only exercises fail-fast against `swim next`
- Title says "verify fail-fast behavior … paths" but does not cover step/run/journal
- Risk: future regression silently skips resolver in those subcommands

**Fix required:**
- Extend test to repeat three fail-fast assertions (missing / unreadable / equal-to-schedule) for `swim step`, `swim run`, `swim journal`
- Reuse existing schedule fixture

**Status:** **open blocker**

---

### HIGH-3: MCP swim handlers bypass contract entirely <a id="high-3-mcp-swim-handlers-bypass-contract-entirely"></a>
**Severity:** HIGH  
**Tasks affected:** T1.3 (H2)  
**Evidence:**
- `handleSwimNext/Step/Run/Journal` in main.go:1032-1150 never call review-schedule resolver
- Schema declarations (main.go:316-358) do not declare `review_schedule` parameter
- Spec names exactly these commands as part of operator surface
- CLI enforces fail-fast; MCP does not — contract inconsistency

**Fix required (choose one):**
1. Add `review_schedule` parameter to all four MCP tool definitions, resolve via env fallback, fail-fast with `swimErrorResult(2, ...)` on missing/equal/unreadable, add unit tests in main_test.go
2. Explicitly exempt MCP from requirement and state in spec

**Status:** **defer with explicit spec note**  
**Rationale:** T1's contract must be clarified in spec whether CLI-only or inclusive of MCP. Can be deferred if spec update is explicit.

---

### HIGH-4: Precondition-conflict fail-fast ownership unclear <a id="high-4-precondition-conflict-fail-fast-ownership-unclear"></a>
**Severity:** HIGH  
**Tasks affected:** T2.1 (IMPORTANT)  
**Evidence:**
- Spec §"Fail-Fast Rules" require failure when *sidecar contains rows whose preconditions conflict with merged state*
- `ValidateReviewScheduleSidecar` only checks per-row `requires`/`produces` against `statusByAction`
- Never walks merged execution to verify row's `requires` matches predecessor's `produces`
- Sidecar `fix` row anchored after `implement` (status `taken`) requires `review_taken` — would silently merge but runtime blocks at apply time, not load time

**Fix required:**
- If T2.1 owns: add pass in `loadSchedule`/`ValidateReviewScheduleSidecar` that walks merged execution, rejects rows where `requires.task_status ≠ predecessor.produces.task_status`
- If T2.2 owns: capture explicitly in plan/spec with one-line note in journal

**Status:** **defer with explicit spec note**  
**Rationale:** Ownership is unclear across T2.1 and T2.2 — needs explicit handoff documentation.

---

## MEDIUM (9 items) <a id="medium-9-items"></a>

### MED-1: Pending-sidecar block precedes precondition block <a id="med-1-pending-sidecar-block-precedes-precondition-block"></a>
**Severity:** MEDIUM  
**Tasks affected:** T2.2 (MED-1)  
**Evidence:**
- `resolver.go:71-85` checks pending sidecar before `Evaluate(row, snap)`
- If operator runs `end_review` while state is still `taken` and pending sidecar exists, reason is `pending_sidecar:...` (masks precondition error)
- Spec line 141 implies both must hold

**Fix required:**
- Add comment in `resolver.go:71` explaining intentional ordering (pending-block is more useful guidance)
- Add test for drift-vs-pending-sidecar scenario asserting pending wins over drift-adoption for both `end_review` and `finish`

**Status:** **open blocker**  
**Rationale:** Ordering is intentional but undocumented; needs defensive comment + test.

---

### MED-2: Sidecar action allowlist not enforced <a id="med-2-sidecar-action-allowlist-not-enforced"></a>
**Severity:** MEDIUM  
**Tasks affected:** T2.2 (MED-2)  
**Evidence:**
- `ValidateReviewScheduleSidecar` accepts any action in `statusByAction` (includes `implement`, `end_review`, `finish`)
- Spec §"Review Loop Semantics" only sanctions `fix`/`review` insertions
- Misuse: insertion with `action="end_review"` would land in merged execution; `pendingSidecarStepForTask` could keep original `end_review` blocked indefinitely

**Fix required:**
- Restrict allowed actions in `ValidateReviewScheduleSidecar` to `{fix, review}` only
- Add negative test in `contracts_test.go`

**Status:** **open blocker**

---

### MED-3: Status/drift ordering interaction with pending-sidecar block <a id="med-3-statusdrift-ordering-interaction-with-pending-sidecar-block"></a>
**Severity:** MEDIUM  
**Tasks affected:** T2.2 (MED-3)  
**Evidence:**
- `safe_runner.go:99-119`: `ActionBlocked` for pending-sidecar uses `appendBlockedResult`
- `ActionDrift` flows through receipt/adopt logic
- With pending-sidecar check first, drift on `end_review`/`finish` is now always converted to `Blocked` when sidecar row pends
- Spec lines 173-175 say this is desired (*stale attempts must return blocked*)

**Fix required:**
- Add explicit test: "state already review_ended, sidecar pending, base end_review row" verifies `ResolutionBlockedPendingSidecar` wins over drift-adoption
- Confirm behavior is intentional with spec author

**Status:** **open blocker**

---

### MED-4: `pendingSidecarStepForTask` only consults forward execution <a id="med-4-pendingsidecarstepfortask-only-consults-forward-execution"></a>
**Severity:** MEDIUM  
**Tasks affected:** T3.2 (M4)  
**Evidence:**
- Linear scan from `cursor+1..len(execution)` per `ResolveNext` call
- O(n) per step; not a perf issue at current scale
- Reinforces M3 cursor-drift semantics gap

**Fix required:**
- Document invariant or cache merged schedule across `runWet` iterations if perf becomes concern

**Status:** **defer with explicit spec note**  
**Rationale:** Not a correctness bug; perf is acceptable at current scale.

---

### MED-5: swim-journal flag advertised in --help but inert <a id="med-5-swim-journal-flag-advertised-in----help-but-inert"></a>
**Severity:** MEDIUM  
**Tasks affected:** T3.2 (M5)  
**Evidence:**
- `tests/swim/test_t4_1_cli_scaffold.sh:54` greps for `--review-schedule` in journal help
- Help text says "Review schedule sidecar path" (implies functionality)
- Binary discards the value (CRIT-2)

**Fix required:**
- Couple with CRIT-2 fix (either implement merge or remove flag)

**Status:** **open blocker** (depends on CRIT-2)

---

### MED-6: waveplan-ps model misses `Invoke` on insertions <a id="med-6-waveplan-ps-model-misses-invoke-on-insertions"></a>
**Severity:** MEDIUM  
**Tasks affected:** T3.2 (M6)  
**Evidence:**
- `waveplan-ps/internal/model/review_schedule.go:18-29` defines `ReviewScheduleInsertion` without `Invoke` field
- Runtime sidecar contract carries `invoke` (internal/swim/contracts.go:120 and JSON schema)
- Observer cannot show argv of inserted rows
- Currently unused because renderer (`tui.go:538-555`) only prints insertion count; will bite next consumer

**Fix required:**
- Add `Invoke` field to `ReviewScheduleInsertion` struct in waveplan-ps model

**Status:** **defer with explicit spec note**  
**Rationale:** Currently unused but asymmetry will surface later; recommend fix in T3.3 polish phase.

---

### MED-7: Renderer surfaces sidecar separately, never merged into unit table <a id="med-7-renderer-surfaces-sidecar-separately-never-merged-into-unit-table"></a>
**Severity:** MEDIUM  
**Tasks affected:** T3.2 (M7)  
**Evidence:**
- `renderReviewSchedules` (`tui.go:538-557`) prints separate "Review Schedules" section
- Shows only counts, not inline rows
- Unit-detail pane (`renderUnitDetails`) doesn't tag rows or show inserted steps inline
- Minimal spec compliance; observation/execution will not visibly match when inserts present

**Fix required:**
- Either inline sidecar rows in unit table or surface `[sidecar]` annotation in unit detail pane
- Update T3.3 scope

**Status:** **defer with explicit spec note**  
**Rationale:** Minimal compliance achieved; recommend defer to T3.3 UI polish.

---

### MED-8: Untested validator branches in `ValidateReviewScheduleSidecar` <a id="med-8-untested-validator-branches-in-validatereviewschedulesidecar"></a>
**Severity:** MEDIUM  
**Tasks affected:** T1.3 (M1)  
**Evidence:**
- Within-sidecar duplicate `step_id` path (contracts.go:293-295) — no test
- `base == nil` guard (contracts.go:238-240) — no test
- Schema-level rejections (e.g. `schema_version: 2`, missing `source_event_id`) — no test
- Self-loop and 3+ node cycle — cycle test only covers 2-cycle

**Fix required:**
- Add test cases to `TestValidateReviewScheduleSidecar_Cases`:
  - within-sidecar duplicate `step_id`
  - `base == nil` returns explicit error
  - schema-level rejection
  - self-loop cycle (X1.AfterStepID = "X1")

**Status:** **open blocker**

---

### MED-9: Resolver return value discarded in four places <a id="med-9-resolver-return-value-discarded-in-four-places"></a>
**Severity:** MEDIUM  
**Tasks affected:** T1.3 (M2)  
**Evidence:**
- `_ = _resolve_review_schedule_path(...)` at waveplan-cli:902, 927, 960, 981
- Resolved path never threaded into downstream `tool_args`
- Related to CRIT-1

**Fix required:**
- Capture and append to `tool_args` per CRIT-1 fix

**Status:** **open blocker** (depends on CRIT-1)

---

## LOW / MINOR (4 items) <a id="low--minor-4-items"></a>

### LOW-1: Dead branch in resolver <a id="low-1-dead-branch-in-resolver"></a>
**Severity:** LOW  
**Tasks affected:** T1.3 (L1)  
**Evidence:**
- `waveplan-cli:368-376`: `_abs_user_path` only returns falsy for falsy input
- `candidate` is non-empty in this branch — `if not resolved:` is unreachable

**Fix required:**
- Drop the dead branch or replace with single-line comment explaining invariant

**Status:** **already fixed** (dead code, safe to remove)

---

### LOW-2: _resolve_review_schedule_path control flow issue <a id="low-2-_resolve_review_schedule_path-control-flow-issue"></a>
**Severity:** LOW  
**Tasks affected:** T2.1 (MINOR)  
**Evidence:**
- Lines 386–397 in `waveplan-cli`: after `_emit_json(..., 2)` for "review schedule must differ from --schedule"
- Function continues and may emit second JSON dict
- Currently safe because `_emit_json` calls `sys.exit`; structure invites regression

**Fix required:**
- Add explicit `return` after each `_emit_json` call or guard with `elif` chain

**Status:** **open blocker**  
**Rationale:** Defensive; prevents future regression if `sys.exit` behavior changes.

---

### LOW-3: `mergeExecutionWithReviewInsertions` seq_hint tie-breaker semantics unclear <a id="low-3--mergeexecutionwithreviewinsertions-seq_hint-tie-breaker-semantics-unclear"></a>
**Severity:** LOW  
**Tasks affected:** T2.1 (MINOR)  
**Evidence:**
- Sorts ties via `rows[i].ID < rows[j].ID`
- Schema pattern `^X[0-9]+$` causes `X10` to sort before `X2` (lexical, not numeric)
- Authors relying on numeric order see surprising merges when `seq_hint` collides

**Fix required (choose one):**
1. Document "ties broken lexically — pad your IDs with zeros"
2. Parse numeric suffix and sort numerically

**Status:** **defer with explicit spec note**  
**Rationale:** Works as-is; needs clarification in spec/schema docs.

---

### LOW-4: `loadSchedule` re-numbers `Seq` even when no insertions <a id="low-4--loadschedules-re-numbers-seq-even-when-no-insertions"></a>
**Severity:** LOW  
**Tasks affected:** T2.1 (MINOR)  
**Evidence:**
- In `len(insertions) == 0` branch, code still walks slice and reassigns `Seq = i+1`
- Harmless because base `Seq` already matches index+1
- Adds allocation+pass for common (no sidecar) case

**Fix required:**
- Early-return original slice unchanged when `len(insertions) == 0`

**Status:** **defer with explicit spec note**  
**Rationale:** Perf optimization; low priority.

---

## STRUCTURAL ISSUES (metadata cleanup) <a id="structural-issues-metadata-cleanup"></a>

### STR-1: waveps.yml and waveplan-ps binary in working tree
**Evidence:**
- `waveps.yml` swaps log_dirs (unrelated to task scope)
- `waveplan-ps/waveplan-ps` built binary appears in `git status`

**Status:** **already fixed** or **clean up before commit**

---

## VERIFICATION GAP <a id="verification-gap"></a>

### VGAP-1: T3.3 tests not executed
**Severity:** BLOCKER  
**Tasks affected:** T3.3  
**Evidence:**
- Test files written: `test_t7_2_insert_fix_loop.sh`, `test_t7_3_review_schedule_passthrough.sh`, `test_t7_4_review_schedule_observer_fixture.sh`
- Fixtures created and valid
- No execution evidence provided (blocked in review sandbox)

**Fix required:**
- Run full test suite: `bash tests/swim/test_t7_*.sh` and `go test ./internal/swim/...`
- Verify all assertions pass before claiming T3.3 complete

**Status:** **open blocker**

---

## SUMMARY TABLE <a id="summary-table"></a>

| ID | Severity | Title | Status | Dependencies |
|----|----------|-------|--------|--------------|
| CRIT-1 | CRITICAL | CLI wiring dead | open blocker | — |
| CRIT-2 | CRITICAL | swim journal drops --review-schedule | open blocker | — |
| CRIT-3 | CRITICAL | --review-schedule hard-required | open blocker | — |
| HIGH-1 | HIGH | ResolveNextFromPaths not threaded | open blocker | CRIT-1 |
| HIGH-2 | HIGH | Verification gap: step/run/journal | open blocker | — |
| HIGH-3 | HIGH | MCP handlers bypass contract | defer | spec clarification |
| HIGH-4 | HIGH | Precondition-conflict ownership | defer | spec clarification |
| MED-1 | MEDIUM | Pending-sidecar block ordering | open blocker | — |
| MED-2 | MEDIUM | Sidecar action allowlist | open blocker | — |
| MED-3 | MEDIUM | Drift/status ordering | open blocker | — |
| MED-4 | MEDIUM | Forward execution scan only | defer | perf optimization |
| MED-5 | MEDIUM | swim-journal flag inert | open blocker | CRIT-2 |
| MED-6 | MEDIUM | waveplan-ps missing Invoke | defer | T3.3 polish |
| MED-7 | MEDIUM | Renderer merged UI | defer | T3.3 polish |
| MED-8 | MEDIUM | Untested validator branches | open blocker | — |
| MED-9 | MEDIUM | Resolver return discarded | open blocker | CRIT-1 |
| LOW-1 | LOW | Dead branch in resolver | already fixed | — |
| LOW-2 | LOW | Control flow safety | open blocker | — |
| LOW-3 | LOW | seq_hint tie-breaker | defer | spec clarification |
| LOW-4 | LOW | Seq re-number optimization | defer | perf optimization |
| VGAP-1 | BLOCKER | T3.3 tests not executed | open blocker | — |

---

## BLOCKING CHAIN <a id="blocking-chain"></a>

**Critical path to unblock:**
1. CRIT-1 (CLI wiring) — blocks HIGH-1, MED-9
2. CRIT-2 (swim journal merge) — blocks MED-5
3. CRIT-3 (--review-schedule required=False) — unblocks existing flows
4. MED-1, MED-2, MED-3 (resolver correctness) — independent of above
5. MED-8 (test coverage) — independent, pre-close cleanup
6. HIGH-2 (verification gaps) — independent, test coverage
7. VGAP-1 (T3.3 test execution) — final blocker

**Recommended fix order:** CRIT-1 → CRIT-3 → CRIT-2 → MED-{1,2,3,8} → HIGH-2 → VGAP-1

---

## NOTES FOR IMPLEMENTER <a id="notes-for-implementer"></a>

- **Sigma's evidence:** CRIT-3 and CRIT-2 reproduce in current HEAD; existing T7.3 test does not catch swim journal bug.
- **env.wp_sched_review:** Contains all artifacts for test reproduction.
- **Spec anchoring:** High-3 and High-4 require spec clarification from author — flag in plan before starting fixes.
- **Test debt:** MED-8 is cheap cleanup; recommend tackle before final T1.3 handoff.
- **Observer polish:** MED-6, MED-7 can ship in T3.3 if time permits; documented as follow-ups.

---

## CLAIM REVIEW <a id="claim-review"></a>

Reviewed 2026-05-14 by code inspection against HEAD.

### Confirmed Accurate

- **CRIT-2: `swim journal` drops --review-schedule** — CONFIRMED. `cmd/swim-journal/main.go:21`: `_ = reviewSchedulePath`. Flag declared but discarded, never passed to `ReadJournalView`.
- **CRIT-3: --review-schedule hard-required** — CONFIRMED. `waveplan-cli:371` defaults `required=True`. All four callers (lines 1399, 1427, 1462, 1485) use default. Pre-T3.2 schedules without sidecar fail with exit 2.
- **HIGH-3: MCP handlers bypass contract** — CONFIRMED. `main.go:1042-1046` (`handleSwimNext`), `main.go:1081-1085` (`handleSwimStep`), `handleSwimRun`, `handleSwimJournal` all call `ResolveNextFromPaths` without `ReviewSchedulePath`. Schema (lines 316-358) does not declare `review_schedule` parameter.
- **MED-1: Pending-sidecar block precedes precondition** — CONFIRMED. `resolver.go:71` checks `pendingSidecarStepForTask` before `Evaluate(row, snap)` at line 86.
- **MED-2: Sidecar action allowlist not enforced** — CONFIRMED. `statusByAction` (lines 41-45) includes `implement`, `end_review`, `finish`. `ValidateReviewScheduleSidecar` at line 293 checks against `statusByAction`, accepting any of these. Spec only sanctions `fix`/`review`.
- **MED-8: Untested validator branches** — CONFIRMED. No test for `base == nil` guard (line 244), within-sidecar duplicate `step_id` (line 282), schema-level rejections (line 252), or self-loop cycle (only 2-cycle tested).
- **VGAP-1: T7.3 doesn't catch journal bug** — PARTIALLY CORRECT. T7.3 exists and passes, but only tests passthrough (flag reaches Go binary), not functionality (journal actually merges sidecar rows).

### Confirmed Stale / Overstated

- **CRIT-1: CLI wiring dead** — OVERSTATED. Python CLI now passes `--review-schedule` to all four Go binaries: `next` (lines 1409-1410), `step` (lines 1433-1434), `run` (lines 1473-1474), `journal` (lines 1488-1489). Wiring is dead only for `swim-journal` binary which accepts but discards the flag (CRIT-2).
- **HIGH-1: ResolveNextFromPaths not threaded for step** — FIXED. `cmd/swim-step/main.go` now passes `ReviewSchedulePath` at lines 50 and 70.
- **LOW-1: Dead branch "already fixed"** — MISLEADING. The dead branch at `waveplan-cli:386` (`if not resolved:`) is still present. Should say "still present, safe to remove."

### Cannot Verify Without Spec Author

- **HIGH-4: Precondition-conflict ownership** — Requires spec review to determine T2.1 vs T2.2 ownership.
- **MED-3: Drift/status ordering** — Code behavior matches ledger description; spec author confirmation needed.
- **MED-4, MED-6, MED-7, LOW-3, LOW-4** — Defer items, low priority.

### Recommended Fix Order (Revised)

1. **CRIT-3** (required=False for next/step/run/journal) — unblocks existing flows, cheap
2. **CRIT-2** (implement journal merge or remove flag) — core functional gap
3. **MED-2** (restrict action allowlist to fix/review) — prevents invalid insertions
4. **MED-8** (fill test coverage gaps) — cheap cleanup before close
5. **HIGH-3** (MCP handlers) — requires spec clarification first
6. **MED-1** (add comment + test for pending-sidecar ordering) — defensive

### Status of Dependent Items

- CRIT-1 is no longer a blocker for next/step/run; only relevant to journal (CRIT-2).
- HIGH-1 is unblocked (already fixed).
- MED-9 (resolver return discarded) is no longer accurate — paths are now captured and appended to `tool_args`.
