# waveplan-ps

## Meta
| key | value |
|---|---|
| schema_version | 1 |
| generated_on | 2026-05-12 |
| plan_version | 1 |
| plan_generation | 2026-05-12T00:00:00Z |

## Plan
| plan_id | plan_title | plan_doc_path | spec_doc_path |
|---|---|---|---|
| waveplan-ps | waveplan-ps observer for waveplan execution | docs/superpowers/plans/2026-05-12-waveplan-ps.md | docs/specs/swim-markdown-plan-format-v1.md |

## Doc Index
| ref | path | line | kind |
|---|---|---:|---|
| cli | cmd/waveplan-ps/main.go | 1 | code |
| cli_test | cmd/waveplan-ps/main_test.go | 1 | test |
| config | internal/config/config.go | 1 | code |
| config_example | docs/waveplan-ps-config-example.yaml | 1 | doc |
| config_test | internal/config/config_test.go | 1 | test |
| discovery | internal/discovery/discover.go | 1 | code |
| discovery_test | internal/discovery/discover_test.go | 1 | test |
| go_mod | go.mod | 1 | code |
| integration_test | tests/integration/waveplan_ps_test.go | 1 | test |
| model_log | internal/model/log.go | 1 | code |
| model_note | internal/model/note.go | 1 | code |
| model_plan | internal/model/plan.go | 1 | code |
| model_state | internal/model/state.go | 1 | code |
| plan | docs/superpowers/plans/2026-05-12-waveplan-ps.md | 1 | plan |
| plan_format | docs/specs/swim-markdown-plan-format-v1.md | 1 | spec |
| readme | README.md | 1 | doc |
| schedule_schema | docs/specs/swim-schedule-schema-v2.json | 1 | spec |
| ui | internal/ui/tui.go | 1 | code |
| ui_test | internal/ui/tui_test.go | 1 | test |
| watch | internal/watch/watcher.go | 1 | code |
| watch_test | internal/watch/watcher_test.go | 1 | test |
| waveplan_cli | waveplan-cli | 1 | code |

## FP Index
| fp_ref | fp_id |
|---|---|

## Tasks
| task_id | title | plan_line | doc_refs | files |
|---|---|---:|---|---|
| T1 | Add project dependencies and domain model types | 49 | plan, go_mod, model_plan, model_state, model_note, model_log | go.mod, go.sum, internal/model/plan.go, internal/model/state.go, internal/model/note.go, internal/model/log.go |
| T2 | Add recursive discovery engine for plans, states, notes, and logs | 50 | plan, discovery, discovery_test, model_log | internal/discovery/discover.go, internal/discovery/discover_test.go |
| T3 | Add YAML config loader with pointer-safe display defaults | 51 | plan, config, config_test | internal/config/config.go, internal/config/config_test.go |
| T4 | Add watcher polling and reusable single-shot loading | 52 | plan, watch, watch_test, model_plan, model_state, model_note, model_log | internal/watch/watcher.go, internal/watch/watcher_test.go |
| T5 | Add tview renderer with wave state, tail state, and exact step log matching | 53 | plan, ui, ui_test, schedule_schema | internal/ui/tui.go, internal/ui/tui_test.go |
| T6 | Add cobra CLI entrypoint with --once, --plan, recursive logs, and live TUI refresh | 54 | plan, cli, cli_test, config, discovery, watch, ui | cmd/waveplan-ps/main.go, cmd/waveplan-ps/main_test.go |
| T7 | Add snapshot integration test for compiled binary behavior | 55 | plan, integration_test, cli | tests/integration/waveplan_ps_test.go |
| T8 | Add waveplan-ps usage docs and config example | 56 | plan, readme, config_example | README.md, docs/waveplan-ps-config-example.yaml |
| T9 | Validate canonical plan JSON and compile SWIM schedule | 57 | plan, plan_format, waveplan_cli, schedule_schema | docs/plans/2026-05-12-waveplan-ps-execution-waves.json, docs/plans/2026-05-12-waveplan-ps-execution-schedule.json |

## Units
| unit_id | task_id | title | kind | wave | plan_line | depends_on | fp_refs | doc_refs |
|---|---|---|---|---:|---:|---|---|---|
| T1.1 | T1 | Add Go dependencies and domain model loaders | impl | 1 | 62 | - | - | go_mod, model_plan, model_state, model_note, model_log |
| T2.1 | T2 | Implement deterministic discovery helpers and log validation | impl | 2 | 63 | T1.1 | - | discovery, discovery_test, model_log |
| T3.1 | T3 | Implement config loading with safe boolean pointer defaults | impl | 2 | 64 | T1.1 | - | config, config_test |
| T4.1 | T4 | Implement watcher polling and exported PollOnce helper | impl | 3 | 65 | T1.1, T2.1 | - | watch, watch_test |
| T5.1 | T5 | Implement tview rendering and exact step_id log correlation | impl | 3 | 66 | T1.1 | - | ui, ui_test, schedule_schema |
| T6.1 | T6 | Implement CLI flags, plan filtering, snapshot mode, and page-refreshing TUI | impl | 4 | 67 | T2.1, T3.1, T4.1, T5.1 | - | cli, cli_test, config, discovery, watch, ui |
| T7.1 | T7 | Add integration test for --once snapshot output | test | 5 | 68 | T6.1 | - | integration_test, cli |
| T8.1 | T8 | Document usage, flags, and recursive log directory config | doc | 5 | 69 | T6.1 | - | readme, config_example |
| T9.1 | T9 | Emit and validate execution-waves JSON from canonical source | verify | 6 | 70 | T7.1, T8.1 | - | plan, plan_format, waveplan_cli |
| T9.2 | T9 | Compile SWIM execution schedule from validated execution-waves JSON | verify | 7 | 71 | T9.1 | - | waveplan_cli, schedule_schema |
