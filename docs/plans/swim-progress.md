# SWIM Implementation Progress

Live tracker for the 23-unit / 6-wave SWIM rollout.

- **Spec**: [`docs/specs/2026-05-05-swim-implementation-plan.md`](../specs/2026-05-05-swim-implementation-plan.md)
- **Wave plan**: [`docs/plans/2026-05-05-swim-execution-waves.json`](2026-05-05-swim-execution-waves.json)
- **Live schedule fixture**: [`docs/plans/2026-05-05-swim-execution-schedule.json`](2026-05-05-swim-execution-schedule.json)

Update protocol — when a unit lands, edit its row to `done` with the commit short-SHA + a one-line note. Keep notes telegraphic.

---

## Wave 1 — Contracts + compiler hardening ✅ (5/5)

| unit | kind | deps | status | commit | notes |
|---|---|---|---|---|---|
| T1.1 | impl | — | done | `9edcf1d` | schedule v2 core: schema_version, seq, action, requires, produces |
| T1.2 | impl | T1.1 | done | `11be792` | invoke.argv canonical; wp_invoke derived via shlex.join (parity guaranteed) |
| T1.3 | impl | T1.2 | done | `c4227c1` | sha256 byte-equivalence pinned across runs; step_id uniqueness asserted |
| T1.4 | doc  | T1.3 | done | `c71d400` | published swim-schedule-schema-v2.json + swim-journal-schema-v1.json (Draft 2020-12) |
| T1.5 | test | T1.4 | done | `a2f59f1` | Go validators (santhosh-tekuri) + cmd/swim-validate; golden expected-schedule.json frozen |

## Wave 2 — Engine layer ✅ (5/5)

| unit | kind | deps | status | commit | notes |
|---|---|---|---|---|---|
| T2.1 | impl | T1.5 | done | `f637998` | StateSnapshot + Status enum; review_taken/review_ended derived from timestamps; Token() = sha256 of canonical re-marshal |
| T2.2 | impl | T2.1 | done | `80b4e13`, `bfb66b3` | Evaluate(row, snap) + Predict(row); 12 table-driven cases + 92-row live-schedule dogfood |
| T2.3 | impl | T2.2 | done | `970f77d` | ExecuteNextStep cursor-driven; appends event + advances cursor on success; cmd/swim-next binary |
| T2.4 | impl | T2.3 | done | `58826bb` | ResolveNext read-only; ahead-of-cursor → cursor_drift, behind → blocked_precondition |
| T2.5 | verify | T2.4 | done | `671bef5` (prep), `e4cd2cc` | ExecuteNextStepSafe with locked A/B/C apply transaction; flock primitive; DetectAndMarkUnknown promotes orphan events; InvokeFn injectable |

## Wave 3 — Step runner + logging + dry-run ✅ (3/3)

| unit | kind | deps | status | commit | notes |
|---|---|---|---|---|---|
| T3.1 | impl | T2.5 | done | `a79891f` | direct fork-exec; logs at `.waveplan/swim/<plan>/logs/<step_id>.<attempt>.{stdout,stderr}.log`; attempt derived from journal scan |
| T3.2 | impl | T3.1 | done | `858c9c0` | `Apply()` operator wrapper; ApplyReport with applied/blocked/done/lock_busy/unknown_pending; lock-holder JSON metadata; postcondition_mismatch Reason; MCP-friendly JSON tags |
| T3.3 | verify | T3.2 | done | `291f395` | `Run()` loop with Until parser (action/seq:N/step:ID), MaxSteps cap, dry-run via in-memory journal+state shadow, stop-on-first-non-applied default |

## Wave 4 — CLI + docs ✅ (3/3)

| unit | kind | deps | status | commit | notes |
|---|---|---|---|---|---|
| T4.1 | impl | T3.3 | done | `26cc02f` | swim subparser tree (compile-schedule, next, step, run, journal, validate); stubs return `not_wired_yet`/exit 2; mutex enforced on step args; rg→grep portability fix in test harness |
| T4.2 | impl | T4.1 | done | `d58a023` | shells/Go-binary dispatchers (cmd/swim-next-resolve, swim-step, swim-run, swim-journal); compile-schedule shells to wp-emit-wave-execution.sh; AckUnknown(stepID, outcome) for ack flow; uniform JSON output + 0/2/3/4 exit codes |
| T4.3 | doc  | T4.2 | done | `b91bb3d` | docs/specs/2026-05-05-swim-ops.md (~250 lines) + README ## SWIM section + example fixtures; 3 recovery flows with copy-paste bash; JSON glossary; exit-code reference; smoke test pins doc contract |

## Wave 5 — MCP swim ‖ refine compiler (0/6, parallel branches)

| unit | kind | deps | status | commit | notes |
|---|---|---|---|---|---|
| T5.1 | impl | T4.3 | pending | — | MCP tools: waveplan_swim_compile / next / step / journal |
| T5.2 | test | T5.1 | pending | — | MCP tool: waveplan_swim_run + CLI/MCP parity tests |
| T6.1 | doc  | T4.3 | done | `2adeba2` | swim-refine-schema-v1.json (profile enum locked to `8k`); swim-refine-profile-8k.md (limits + refinement rules + step_id format + debug-only command_hint); sample fixture; smoke test |
| T6.2 | impl | T6.1 | done | `e0b37d8` | Refine(opts) + cmd/swim-refine-compile; profile=8k locked, --targets required, sort+dedup, chunk-by-index at MaxFiles=6, linear sN chain, passthrough s1, byte-identical sha256 across runs; 13 unit tests + shell harness with real exit codes; google/shlex vendored under third_party/ |
| T6.3 | impl | T6.2 | pending | — | refine-run execution + parent roll-up semantics |
| T6.4 | test | T6.3 | pending | — | swim refine/refine-run CLI + determinism tests |

## Wave 6 — MCP refine parity (0/1)

| unit | kind | deps | status | commit | notes |
|---|---|---|---|---|---|
| T7.1 | impl | T5.2, T6.4 | pending | — | MCP tools: waveplan_swim_refine / waveplan_swim_refine_run |

---

## Cross-cutting infrastructure (not unit-scoped)

| commit | scope |
|---|---|
| `2351409` | coarse waves expanded to 22 sub-units; T7 MCP refine parity added |
| `3e0793d` | swim compile-plan-json — preserve all fields, ref-integrity, type/date checks, loosened task-key regex |
| `7955d78` | compile-plan-json regression suite pinning F1–F4 review findings |
| `9cab46e` | tools/render_execution_waves.py — graphviz viz of execution waves |

## Locked decisions (apply globally)

1. `status` field retained in schedule rows during transition; mark deprecated; drop later behind feature flag
2. `review_taken` / `review_ended` derived from timestamps (no state-schema extension)
3. `wp_invoke` is debug-only; `invoke.argv` is the only execution source
4. SWIM artifact layout: `.waveplan/swim/<plan-basename>/{swim.lock, logs/, *.journal.json}`
5. step_id format: `S{wave}_{task_id}_{action}` (underscore separator, avoids period collision with `T1.1`-style task IDs)
6. Refine v1 profile: `8k` only (max_tokens ≤ 8000, max_files ≤ 6, max_lines ≤ 400)
7. Refine fine step_id format: `F{wave}_{parent}_s{n}`
8. Refine v1 ships with `command_hint` as optional debug-only mirror of `invoke.argv`

## Session log

- **2026-05-07** — Wave 1 complete; T2.1, T2.2 land. Compile-plan-json hardened. Live schedule emitted + placed.
- **2026-05-08** — T2.3, T2.4 land. T2.5 prep patch (schema relaxation + resolver unknown_pending). Sigma in flight on T2.5 lock + safe_runner + recovery. **Wave 2 complete** with `e4cd2cc` (T2.5 race-closure). swim-progress.md tracker added (`30fa99a`). gitignore SWIM artifacts (`a51416b`).
- **2026-05-09** — T3.1 lands (`a79891f`): argv runner with direct stdout/stderr capture and journal-derived attempt counter. T3.2 lands (`858c9c0`): Apply() wrapper with ApplyReport status normalization + lock-holder diagnostics. T3.3 lands (`291f395`): Run() loop with Until parser, MaxSteps cap, and dry-run via in-memory shadow. **Wave 3 complete.** T4.1 lands (`26cc02f`): waveplan-cli swim subparser tree with stub handlers; rg→grep portability fix in test harness. T4.2 lands (`d58a023`): handlers wired through new cmd/swim-{next-resolve,step,run,journal} binaries + AckUnknown surface; uniform JSON output + 0/2/3/4 exit codes. T4.3 lands (`b91bb3d`): ops manual + README SWIM section + example fixtures; 3 recovery flows documented with copy-paste bash. **Wave 4 complete.** Wave 5 opens parallel: Sigma takes T5.1→T5.2 (MCP swim tools), Claude takes T6.1→T6.4 (refine compiler). T6.1 lands (`2adeba2`): refine schema v1 + 8k profile contract + sample fixture. T6.2 lands (`e0b37d8`): Refine() compiler + cmd/swim-refine-compile binary; deterministic byte-identical output; google/shlex vendored under third_party/.
