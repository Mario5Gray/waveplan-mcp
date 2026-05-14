# swim-review-schedule-sidecar

## Meta
| key | value |
|---|---|
| schema_version | 1 |
| generated_on | 2026-05-14 |
| plan_version | 1 |
| plan_generation | 2026-05-14T00:00:00Z |

## Plan
| plan_id | plan_title | plan_doc_path | spec_doc_path |
|---|---|---|---|
| swim-review-schedule-sidecar | SWIM Review Schedule Sidecar | docs/superpowers/plans/2026-05-14-swim-review-schedule-sidecar-swim.md | docs/specs/2026-05-14-swim-review-schedule-sidecar.md |

## Doc Index
| ref | path | line | kind |
|---|---|---:|---|
| plan | docs/superpowers/plans/2026-05-14-swim-review-schedule-sidecar-swim.md | 1 | plan |
| spec | docs/specs/2026-05-14-swim-review-schedule-sidecar.md | 1 | spec |
| swim_contracts_go | internal/swim/contracts.go | 1 | code |
| swim_fixes_spec | docs/specs/2026-05-10-swim-fixes.md | 1 | spec |
| swim_resolver_test_go | internal/swim/resolver_test.go | 1 | test |
| swim_run_go | internal/swim/run.go | 1 | code |
| waveplan_cli | waveplan-cli | 1 | code |
| waveplan_ps_main_go | waveplan-ps/cmd/waveplan-ps/main.go | 1 | code |

## FP Index
| fp_ref | fp_id |
|---|---|
| FP-CLI | FP-akaejfjb |
| FP-CONTRACT | FP-byeibgzt |
| FP-RUNTIME | FP-aihwddfz |

## Tasks
| task_id | title | plan_line | doc_refs | files |
|---|---|---:|---|---|
| T1 | Define review sidecar contract and explicit path rules | 1 | plan, spec, swim_fixes_spec, waveplan_cli, swim_contracts_go | waveplan-cli, internal/swim/contracts.go, docs/specs/2026-05-14-swim-review-schedule-sidecar.md |
| T2 | Merge base and review schedules at runtime | 1 | plan, spec, swim_run_go, swim_contracts_go, swim_resolver_test_go | internal/swim/run.go, internal/swim/contracts.go, internal/swim/resolver_test.go |
| T3 | Insert fix loops and align observer behavior | 1 | plan, spec, waveplan_cli, waveplan_ps_main_go | waveplan-cli, waveplan-ps/cmd/waveplan-ps/main.go |

## Units
| unit_id | task_id | title | kind | wave | plan_line | depends_on | fp_refs | doc_refs |
|---|---|---|---|---:|---:|---|---|---|
| T1.1 | T1 | Define review sidecar schema and anchor validation rules | impl | 1 | 1 | - | FP-CONTRACT | plan, spec, swim_fixes_spec, swim_contracts_go |
| T1.2 | T1 | Add explicit review-schedule flag and env resolution contract | impl | 2 | 1 | T1.1 | FP-CONTRACT | plan, spec, waveplan_cli |
| T1.3 | T1 | Verify fail-fast behavior for missing or invalid review-schedule paths | verify | 3 | 1 | T1.2 | FP-CONTRACT | plan, spec, waveplan_cli |
| T2.1 | T2 | Merge anchored sidecar rows into the executable runtime frame | impl | 4 | 1 | T1.3 | FP-RUNTIME | plan, spec, swim_run_go, swim_contracts_go |
| T2.2 | T2 | Block end_review and finish while pending sidecar rows exist | impl | 5 | 1 | T2.1 | FP-RUNTIME | plan, spec, swim_run_go, swim_contracts_go |
| T2.3 | T2 | Add resolver and runner tests for merged review loops | test | 6 | 1 | T2.2 | FP-RUNTIME | plan, spec, swim_resolver_test_go |
| T3.1 | T3 | Add deterministic insert-fix-loop sidecar writer command | impl | 7 | 1 | T2.3 | FP-CLI | plan, spec, waveplan_cli |
| T3.2 | T3 | Thread review-schedule through swim execution and waveplan-ps observation | impl | 8 | 1 | T3.1 | FP-CLI | plan, spec, waveplan_cli, waveplan_ps_main_go |
| T3.3 | T3 | Refresh docs and fixture-backed verification for sidecar review loops | verify | 9 | 1 | T3.2 | FP-CLI | plan, spec, waveplan_cli, waveplan_ps_main_go |
