<!-- INDEX -->
- [Validate parameters on cli and mcp](#validate-parameters-on-cli-and-mcp)
- [Real-time tail logs](#real-time-tail-logs)
- [Progression checkpoints](#progression-checkpoints)
- [Blocked recovery workflow](#blocked-recovery-workflow)
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
