# waveagents-provider-objects

## Meta
| key | value |
|---|---|
| schema_version | 1 |
| generated_on | 2026-05-13 |
| plan_version | 1 |
| plan_generation | 2026-05-13T00:00:00Z |

## Plan
| plan_id | plan_title | plan_doc_path | spec_doc_path |
|---|---|---|---|
| waveagents-provider-objects | waveagents provider catalog and command-provider schedule emission | docs/superpowers/plans/2026-05-13-waveagents-provider-objects.md | docs/specs/2026-05-05-swim-ops.md |

## Doc Index
| ref | path | line | kind |
|---|---|---:|---|
| agent_config_example | docs/waveplan-agent-config-example.json | 1 | doc |
| cli_wired_test | tests/swim/test_t4_2_cli_wired.sh | 1 | test |
| emit_script | wp-emit-wave-execution.sh | 1 | code |
| fixture_expected_schedule | tests/swim/fixtures/expected-schedule.json | 1 | test |
| fixture_waveagents | tests/swim/fixtures/waveagents.json | 1 | test |
| main_test | main_test.go | 1 | test |
| plan | docs/superpowers/plans/2026-05-13-waveagents-provider-objects.md | 1 | plan |
| readme | README.md | 1 | doc |
| swim_ops_example | docs/specs/swim-ops-examples/waveagents.json | 1 | doc |
| swim_ops_spec | docs/specs/2026-05-05-swim-ops.md | 1 | spec |
| waveplan_cli | waveplan-cli | 1 | code |

## FP Index
| fp_ref | fp_id |
|---|---|


## Tasks
| task_id | title | plan_line | doc_refs | files |
|---|---|---:|---|---|
| T1 | Define provider catalog contract and migrate fixture/config shapes | 48 | plan, readme, swim_ops_spec, swim_ops_example, fixture_waveagents, agent_config_example | README.md, docs/specs/2026-05-05-swim-ops.md, docs/specs/swim-ops-examples/waveagents.json, tests/swim/fixtures/waveagents.json, docs/waveplan-agent-config-example.json |
| T2 | Parse provider catalogs and agent provider keys in the schedule compiler | 49 | plan, emit_script, main_test | wp-emit-wave-execution.sh, main_test.go |
| T3 | Emit provider command argv for agent-dispatch rows | 50 | plan, emit_script, fixture_expected_schedule | wp-emit-wave-execution.sh, tests/swim/fixtures/expected-schedule.json |
| T4 | Refresh compile-schedule coverage for provider-object configs | 51 | plan, main_test, cli_wired_test, fixture_waveagents | main_test.go, tests/swim/test_t4_2_cli_wired.sh, tests/swim/fixtures/waveagents.json |
| T5 | Document migration and validate provider-object schedule compilation | 52 | plan, readme, swim_ops_spec, swim_ops_example, agent_config_example, waveplan_cli | README.md, docs/specs/2026-05-05-swim-ops.md, docs/specs/swim-ops-examples/waveagents.json, docs/waveplan-agent-config-example.json, waveplan-cli |

## Units
| unit_id | task_id | title | kind | wave | plan_line | depends_on | fp_refs | doc_refs |
|---|---|---|---|---:|---:|---|---|---|
| T1.1 | T1 | Replace legacy flat provider strings with a `providers` object and provider-key agent refs in examples and fixtures | impl | 1 | 59 | - | - | readme, swim_ops_spec, swim_ops_example, fixture_waveagents, agent_config_example |
| T1.2 | T1 | Add negative fixture expectations for legacy `provider` strings and missing provider references | test | 2 | 60 | T1.1 | - | fixture_waveagents, main_test |
| T2.1 | T2 | Load the `providers` catalog and resolve each agent's provider key in `wp-emit-wave-execution.sh` | impl | 3 | 61 | T1.2 | - | emit_script |
| T2.2 | T2 | Reject missing providers, duplicate provider keys, unsupported stereotypes, and legacy flat provider strings deterministically | test | 4 | 62 | T2.1 | - | emit_script, main_test |
| T3.1 | T3 | Build implement and review `invoke.argv` by prepending `provider.command.cmd` and `provider.command.args` | impl | 5 | 63 | T2.2 | - | emit_script, fixture_expected_schedule |
| T3.2 | T3 | Keep `end_review` and `finish` on the generic lifecycle invoker and refresh expected schedule fixtures | verify | 6 | 64 | T3.1 | - | emit_script, fixture_expected_schedule |
| T4.1 | T4 | Update `main_test.go` compile-schedule coverage for provider catalog configs | test | 7 | 65 | T3.2 | - | main_test, fixture_waveagents |
| T4.2 | T4 | Refresh shell smoke tests to write provider-catalog `waveagents.json` fixtures | test | 8 | 66 | T4.1 | - | cli_wired_test, fixture_waveagents |
| T5.1 | T5 | Document the provider catalog schema, command stereotype, and migration from flat provider strings | doc | 9 | 67 | T4.2 | - | readme, swim_ops_spec, swim_ops_example, agent_config_example |
| T5.2 | T5 | Validate canonical plan JSON and compile a schedule using provider-object fixtures | verify | 10 | 68 | T5.1 | - | plan, waveplan_cli, fixture_waveagents |
