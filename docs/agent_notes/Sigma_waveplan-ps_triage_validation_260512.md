<!-- INDEX -->
- [Validate parameters on cli and mcp](#validate-parameters-on-cli-and-mcp)
- [Real-time tail logs](#real-time-tail-logs)
- [Progression checkpoints](#progression-checkpoints)
- [Blocked recovery workflow](#blocked-recovery-workflow)
- [go.mod replace directive masked real tview with no-op shim](#gomod-replace-directive-masked-real-tview-with-no-op-shim)
- [cursor_drift from review agent calling end_review](#cursordrift-from-review-agent-calling-endreview)
- [swim-adopt-drift.sh races with swim run, skips cursor steps](#swim-adopt-driftsh-races-with-swim-run-skips-cursor-steps)
- [journal cursor advances past step with no event when state-file write is skipped](#journal-cursor-advances-past-step-with-no-event-when-state-file-write-is-skipped)
<!-- /INDEX -->

## SWIM > Validation > Validate parameters on cli and mcp
Requirement:
Validate SWIM CLI and MCP parameters before execution.

Scope:
- `--seq ${seq}` must be checked against the compiled schedule/waves before use. Reject missing or non-existent sequence numbers with a deterministic error.
- `--until` values must be validated before run/resume. For any node, task, unit, step id, or sequence target, confirm the target exists in the schedule/wave graph.
- The same validation must be enforced through both CLI and MCP entrypoints so behavior is identical regardless of caller.

Rationale:
Interrupted or mistyped run commands can otherwise create confusing blocked/unknown recovery states or run with a target that is not reachable in the generated schedule.

Implementation note:
Prefer validating against the schedule execution rows and waveplan units before invoking runner state mutation. Return a fail-fast error that names the bad parameter and lists the accepted target forms.


## SWIM > Logging > Real-time tail logs
Observation:
`--tail-logs` currently appears to emit logs only after the invoked step exits. It is useful for post-run inspection but does not provide real-time visibility while a long-running agent step is active.

Problem:
During long executions, operators cannot see streaming stdout/stderr through the SWIM wrapper, which makes interruption/recovery decisions harder and hides early failures until completion.

Follow-up:
Revisit log transport for CLI and MCP execution paths. Consider streaming child stdout/stderr to the parent process while also preserving file-backed logs for journal replay. The implementation should define whether live logs are opt-in, how stderr/stdout interleave is represented, and how MCP tool responses expose incremental output.


## SWIM > Observability > Progression checkpoints
Observation:
SWIM execution needs progression checkpoints so operators can gauge where a sequence is inside its workflow, not just whether the current step has finished.

Problem:
For long steps or multi-action sequences, the journal cursor and final step outcome are too coarse. Operators need intermediate markers that show current phase, active subprocess, dispatch status, log availability, and recovery boundary.

Follow-up:
Add checkpoint events or progress records that are emitted during sequence execution. Checkpoints should be visible through CLI and MCP status/journal views, and should distinguish scheduler progress, precondition checks, subprocess launch, agent dispatch, review/end-review/finish transitions, and log capture milestones.


## Blocked recovery workflow
Observation:
Blocked SWIM steps are recoverable, but the current operator workflow is too manual and too easy to misapply. In particular, cursor drift and failed dispatches require the operator to infer whether to retry the same seq, advance, or repair the schedule/state mismatch.

Current recovery guidance:
- `cursor_drift`: inspect `swim next`; do not assume `cursor + 1` is correct.
- If `state_after` did not advance, retrying the same seq is usually correct after fixing the cause.
- If state did advance, do not rerun blindly; check `swim next`.
- If the schedule itself is wrong, patch or recompile the schedule before continuing.

Gap:
There is no single `recover` command that classifies the block reason, inspects the last journal event, suggests the exact next command, and refuses unsafe replays. Recovery is currently manual and error-prone for operators mid-wave.

Follow-up:
Add a deterministic recovery helper for blocked and drift states. It should: classify `reason`, inspect the last event, compare journal cursor to live state, recommend the next safe command, and refuse replays when the state transition already advanced.


## waveplan-ps > Build > go.mod replace directive masked real tview with no-op shim
Symptom: `waveplan-ps -config ./waveps.yml` exited immediately with no output and exit code 0. The `-once` flag worked correctly.

Root cause: `go.mod` contained `replace github.com/rivo/tview => ./internal/tviewshim`. The shim's `Application.Run()` returned `nil` immediately instead of blocking on the TUI event loop. This meant the live-refresh mode started, rendered nothing, and returned as if the user had quit.

The shim lives in `internal/tviewshim/` and exists for tests that cannot drive a real terminal. It was accidentally left as the active replacement in the production module, so every build since has shipped the stub instead of the real library.

Diagnosis path:
- `-once` worked → data loading and rendering logic were fine; only `runLive` was broken.
- `strings` on the binary showed `./internal/tviewshim` in the symbol table, confirming the shim was linked.
- Reading `go.mod` revealed the `replace` directive on line 11.

Fix:
1. Remove the `replace` line from `go.mod`.
2. Run `go get github.com/rivo/tview@v0.42.0` to populate `go.sum`.
3. Fix the one API mismatch: real tview's `Pages.AddAndSwitchToPage` takes 3 args, not 4.
4. Rebuild and verify `strings` no longer shows `tviewshim`.

Lesson: a `replace` directive that points at an internal test stub will silently produce a working but non-interactive binary. Symptoms look like a TTY or terminal capability problem, not a linker substitution. Always check `go.mod` replace directives and `strings <binary> | grep shim` when a TUI tool exits immediately.


## SWIM > Cursor > cursor_drift from review agent calling end_review
Symptom: `swim run` blocks repeatedly on `cursor_drift: action=review requires=taken actual=review_ended (state ahead)`. The step shows `postcondition_unmet` on first attempt, then `cursor_drift` on every retry.

Root cause (two-layer):

1. AGENT OVERSHOT: The review prompt in `wp-task-to-agent.sh` (lines 534–541) was missing the write-action prohibition present in both implement and fix modes. The reviewer agent (opencode/phi) called `end_review` via waveplan-mcp after finishing its review, advancing state `taken → review_taken → review_ended` in one shot. SWIM expected `review_taken` as the postcondition of the review step; seeing `review_ended` it recorded `blocked` and did not advance the cursor.

2. DRIFT HANDLER BLIND SPOT: `safe_runner.go:102` only auto-adopts when `actual == Predict(row)`. For seq 38 (review), `Predict = review_taken` but `actual = review_ended` — one step past the expected postcondition. Adoption fails; every retry records another blocked event without advancing. The cursor is permanently stuck until manually repaired.

Evidence trail: E0042 shows `outcome: blocked`, `reason: postcondition_unmet: action=review produces=review_taken actual=review_ended`, `state_after: review_ended`. The state file confirms `review_entered_at` and `review_ended_at` both set at the same timestamp as the dispatch receipt.

Fix applied:
1. Added write-action prohibition to review PREAMBLE in `wp-task-to-agent.sh` (same three lines as implement/fix modes).
2. Manual cursor advance: added idempotent_adopt event E0044 for seq 38 to journal, set cursor to 38. Next `swim run` drift-adopts seq 39 (end_review, Predict=review_ended==actual) then runs seq 40 (finish) normally.

Recovery procedure for recurrence (cursor stuck on cursor_drift for review step):
1. Back up journal: `cp <journal>.json <journal>.json.bak-$(date +%Y%m%d-%H%M%S)`
2. Check actual state: `swim-step --schedule ... --journal ... --state ...` — confirm action=drift for the end_review row following the stuck review row.
3. Append adopt event and advance cursor by 1 (Python one-liner against the journal JSON). Set `outcome: applied`, `reason: idempotent_adopt`, `cursor += 1`.
4. Re-run `swim-step` — should now show drift on end_review with Predict==actual, confirming auto-adopt on next run.
5. Resume: `swim run --schedule ... --until seq:N`

Systemic gap: drift handler does not handle overshoot (actual > Predict by more than one status step). If an agent overshoots by two steps, manual cursor surgery is always required.


## SWIM > Cursor > swim-adopt-drift.sh races with swim run, skips cursor steps
Symptom: After `swim-adopt-drift.sh` ran and advanced cursor, a subsequent journal inspection showed cursor had jumped past seq 55 (S11_T4.2_end_review) with no journal event recorded for that step. State was left at `review_taken`; seq 56 (finish) blocked on `precondition_unmet: requires=review_ended actual=review_taken`.

Root cause: `swim-adopt-drift.sh` does not acquire the SWIM lock before reading and writing the journal. If `swim run` is executing concurrently (i.e., the review dispatch is still in flight), both processes reload and save the journal independently. The in-flight safe_runner reloads the journal after the invoke finishes and increments cursor; adopt-drift also increments cursor. The two increments result in cursor advancing by 2 instead of 1, silently skipping the next scheduled step.

Evidence: journal had E0063 (seq 54, review, applied) followed immediately by E0064 (adopt, seq 54, applied) with cursor=54, then E0065 (seq 56, finish, blocked) — seq 55 (end_review) never appeared.

Fix applied: called `waveplan-cli end_review T4.2` directly to set `review_ended_at` in the state file. Since journal cursor was already past seq 55, SWIM did not need to execute end_review; the state just needed to catch up. After direct call, `swim-step` returned `ready` for seq 56 (finish).

Required follow-up: `swim-adopt-drift.sh` must acquire the swim lock (read lock path from schedule path: `<schedule>.lock`) before reading the journal and release it after saving. Use the same lock file and PID protocol as the Go safe_runner so concurrent adopt-drift + swim run cannot interleave.


## SWIM > Cursor > journal cursor advances past step with no event when state-file write is skipped
Symptom: cursor=N+1 in journal but no event recorded for seq N, and the corresponding waveplan state mutation never happened. Manifests as `blocked_precondition` on the step after the skipped one.

Observed instance: seq 55 (S11_T4.2_end_review) was skipped. cursor jumped from 54 to 55 with no end_review event. State showed `review_entered_at` set but `review_ended_at` empty. seq 56 (finish) blocked on `requires=review_ended actual=review_taken`.

Root cause: the concurrent adopt-drift race (see related entry). A secondary contributing factor is that the adopt-drift script does not validate whether the step being adopted has already been recorded as applied — it adopted seq 54 (review) a second time even though E0063 already existed showing seq 54 applied. This redundant adopt incremented cursor an extra time.

Recovery procedure when cursor is past the skipped step:
1. Identify the skipped step from the journal gap (missing seq between last applied event and current cursor).
2. Determine what state mutation that step would have produced.
3. Apply that mutation directly via waveplan-cli (e.g. `waveplan-cli end_review <task-id>`).
4. Re-run `swim-step` to confirm next step is now `ready`.
5. Resume `swim run`.

Do NOT roll back cursor — if cursor is past the step, rolling back risks re-executing steps that already ran and double-mutating state.

Required follow-up: `swim-adopt-drift.sh` should check whether the current cursor step already has an applied event before adopting. If an applied event exists for the cursor seq, the cursor is stale but the step is done — skip the adopt and just verify cursor matches event count.
