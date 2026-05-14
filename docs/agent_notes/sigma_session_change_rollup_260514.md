# Session Change Rollup — 2026-05-14

This document summarizes the main changes made in the last couple of hours.

FP status tags:
- `FP:` existing FP issue or issue set already exists
- `NEEDS FP PLAN:` no FP issue set exists yet; a later janitorial process should create one

## 1. SWIM review schedule sidecar

**FP:** `FP-qjzikzfe`  
Children: `FP-byeibgzt`, `FP-aihwddfz`, `FP-akaejfjb`

What changed:
- Defined the review-loop sidecar contract so review findings can materialize into explicit fix/re-review rows.
- Locked the path policy to explicit review-sidecar inputs only:
  - `--review-schedule`
  - `WAVEPLAN_SCHED_REVIEW`
- Rejected guessed filenames and implicit sibling discovery.
- Created canonical SWIM artifacts and compiled plan/schedule for this work.

Artifacts:
- `docs/specs/2026-05-14-swim-review-schedule-sidecar.md`
- `docs/superpowers/plans/2026-05-14-swim-review-schedule-sidecar-swim.json`
- `docs/superpowers/plans/2026-05-14-swim-review-schedule-sidecar-swim.md`
- `docs/plans/2026-05-14-swim-review-schedule-sidecar-execution-waves.json`
- `docs/plans/2026-05-14-swim-review-schedule-sidecar-execution-schedule.json`

## 2. SWIM task-level `--until`

**NEEDS FP PLAN**

What changed:
- Added task-level stop conditions to `swim run`.
- New accepted forms:
  - `--until T2`
  - `--until task:T2`
- Semantics: resolve to the last schedule row for all units under task prefix `T2.*`.
- This is deterministic and fail-fast if the task is not present in the schedule.

Code:
- `internal/swim/apply.go`
- `internal/swim/run.go`
- `internal/swim/run_test.go`
- `waveplan-cli`

Notes:
- Verified with package tests and dry-run against a live schedule.

## 3. waveplan-ps explicit observer input policy

**NEEDS FP PLAN**

What changed:
- Removed recursive scanning for:
  - plans
  - state files
  - journals
  - notes
- `waveplan-ps` now loads those only from:
  - explicit CLI flags: `--plan`, `--state`, `--journal`, `--note`
  - env fallback: `WAVEPLAN_PLAN`, `WAVEPLAN_STATE`, `WAVEPLAN_JOURNAL`
- Legacy YAML scan-root keys now fail fast:
  - `plan_dirs`
  - `state_dirs`
  - `journal_dirs`
  - `note_dirs`
- `log_dirs` remains directory-based because SWIM step logs are fan-out files under `.waveplan`.

Code and docs:
- `waveplan-ps/cmd/waveplan-ps/main.go`
- `waveplan-ps/internal/config/config.go`
- `waveplan-ps/internal/config/config_test.go`
- `waveplan-ps/tests/integration/waveplan_ps_test.go`
- `waveplan-ps/README.md`
- `waveplan-ps/waveps.yml`
- `waveplan-ps/docs/waveplan-ps-config-example.yaml`
- `waveps.yml`

Commit:
- `6c47c1b feat(waveplan-ps): require explicit observer inputs`

## 4. waveplan-ps TUI layout and operator ergonomics

**NEEDS FP PLAN**

What changed:
- Capped visible wave/unit rows and added `--unit-limit` with default `10`.
- Fixed header-row behavior while scrolling.
- Kept the details/log pane visible by bounding the top table.
- Added horizontal rulers plus a current-unit summary strip showing:
  - `log#`
  - `seq#`
  - `status`
  - `actor`
  - `action`
  - `wall`
- Added explicit reviewer denotation in the active actor display:
  - e.g. `sigma [review]`

Code:
- `waveplan-ps/internal/ui/tui.go`
- `waveplan-ps/internal/ui/tui_test.go`
- `waveplan-ps/cmd/waveplan-ps/main.go`
- `waveplan-ps/cmd/waveplan-ps/main_test.go`

## 5. waveplan-ps execution-bundle naming support

**NEEDS FP PLAN**

What changed:
- Extended discovery/state matching so `waveplan-ps` can recognize contained execution bundles using:
  - `*-execution-state.json`
  - `*-execution-journal.json`
- This fixed the observer showing everything as `available` when the bundle no longer used legacy sidecar names.

Code:
- `waveplan-ps/internal/discovery/discover.go`
- `waveplan-ps/internal/discovery/discover_test.go`
- `waveplan-ps/internal/ui/tui.go`

## 6. Contextsize alignment fixes

**FP:** `FP-pkymdlbv`  
Related children already created earlier for the adapter plan.

What changed:
- Brought `internal/contextsize` back in line with the design doc.
- Fixed confidence handling for missing section files and non-ENOENT IO failures.
- Corrected unique local import counting.
- Fixed section sizing to include heading bytes.
- Normalized CLI behavior so documented invocation works.
- Reinstalled the tool and verified `waveplan-cli context estimate` uses the fresh binary.

Code and docs:
- `internal/contextsize/estimate.go`
- `internal/contextsize/estimate_test.go`
- `cmd/contextsize/main.go`
- `docs/superpowers/specs/2026-05-13-context-sizer-design.md`

## 7. Contextsize Waveplan adapter planning artifacts

**FP:** `FP-pkymdlbv`

What changed:
- Wrote the Waveplan adapter follow-up plan and canonical SWIM artifacts.
- Compiled execution-waves and execution-schedule artifacts for the adapter work.

Artifacts:
- `docs/superpowers/plans/2026-05-13-context-sizer-waveplan-adapter.md`
- `docs/superpowers/plans/2026-05-13-context-sizer-waveplan-adapter-swim.json`
- `docs/superpowers/plans/2026-05-13-context-sizer-waveplan-adapter-swim.md`
- `docs/plans/2026-05-13-context-sizer-waveplan-adapter-execution-waves.json`
- `docs/plans/2026-05-13-context-sizer-waveplan-adapter-execution-schedule.json`

## 8. Waveagents provider-object planning

**FP:** `FP-mgwrpwst`

What changed:
- Wrote the provider-catalog SWIM plan and linked FP issue.
- This is the redesign where providers become named objects and agents reference them by key.

Artifacts:
- `docs/superpowers/plans/2026-05-13-waveagents-provider-objects.md`
- `docs/plans/2026-05-13-waveagents-provider-objects-execution-waves.json`

## 9. Cloudflare AI Worker plan canonicalization

**FP:** `FP-sscpykry`

What changed:
- Converted the implementation-style Cloudflare plan into canonical SWIM markdown.
- Compiled it into execution-waves and an execution schedule.
- Left it serialized intentionally because the work is small enough to run linearly.

Artifacts:
- `docs/superpowers/plans/2026-05-13-cloudflare-ai-worker-swim.json`
- `docs/superpowers/plans/2026-05-13-cloudflare-ai-worker-swim.md`
- `docs/plans/2026-05-13-cloudflare-ai-worker-execution-waves.json`
- `docs/plans/2026-05-13-cloudflare-ai-worker-execution-schedule.json`

## 10. Items that likely need later FP cleanup

These changes appear to have no dedicated FP issue set yet and should be picked up by the later janitorial process:

- `waveplan-ps` explicit observer input policy
- `waveplan-ps` TUI and reviewer-status improvements
- `waveplan-ps` execution-bundle naming support
- SWIM task-level `--until Tn`

## 11. SWIM explicit artifact-root override and help-text alignment

**NEEDS FP PLAN**

What changed:
- Added an explicit SWIM runtime artifact root override for `swim run` and `swim step`.
- New operator inputs:
  - `--artifact-root`
  - `WAVEPLAN_SWIM_ARTIFACT_ROOT`
- Help text now documents the default artifact bundle root clearly:
  - `<dirname(schedule)>/.waveplan/swim/<schedule-name>`
- The override applies consistently to:
  - stdout/stderr logs
  - dispatch receipts
  - `swim.lock`
- Updated `env.contextsizr` to pin the context-sizer bundle to an explicit artifact root instead of relying on derivation.

Code:
- `internal/swim/artifacts.go`
- `internal/swim/runner.go`
- `internal/swim/safe_runner.go`
- `internal/swim/dispatch.go`
- `internal/swim/apply.go`
- `internal/swim/run.go`
- `internal/swim/refine_run.go`
- `cmd/swim-run/main.go`
- `cmd/swim-step/main.go`
- `waveplan-cli`
- `env.contextsizr`

Verification:
- `go test ./internal/swim`
- `go test ./cmd/swim-run ./cmd/swim-step`
- `python3 ./waveplan-cli swim run --help`
- `python3 ./waveplan-cli swim step --help`
