# context-sizer-waveplan-adapter

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
| context-sizer-waveplan-adapter | Context Sizer Waveplan Adapter | docs/superpowers/plans/2026-05-13-context-sizer-waveplan-adapter.md | docs/superpowers/specs/2026-05-13-context-sizer-design.md |

## Doc Index
| ref | path | line | kind |
|---|---|---:|---|
| adapter_waveplan_go | internal/contextsize/adapter/waveplan.go | 1 | code |
| adapter_waveplan_test_go | internal/contextsize/adapter/waveplan_test.go | 1 | test |
| cloudflare_execution_waves | docs/plans/2026-05-13-cloudflare-ai-worker-execution-waves.json | 1 | test |
| plan | docs/superpowers/plans/2026-05-13-context-sizer-waveplan-adapter.md | 1 | plan |
| spec | docs/superpowers/specs/2026-05-13-context-sizer-design.md | 290 | spec |
| waveplan_cli | waveplan-cli | 531 | code |

## FP Index
| fp_ref | fp_id |
|---|---|
| FP-ADAPTER-CLI | FP-pkymdlbv#FP-ljdtcbbi |
| FP-ADAPTER-E2E | FP-pkymdlbv#FP-kaxwxxpx |
| FP-ADAPTER-MAP | FP-pkymdlbv#FP-rrxobrej |

## Tasks
| task_id | title | plan_line | doc_refs | files |
|---|---|---:|---|---|
| T1 | Waveplan adapter mapping | 26 | plan, spec, adapter_waveplan_go, adapter_waveplan_test_go, cloudflare_execution_waves | internal/contextsize/adapter/waveplan.go, internal/contextsize/adapter/waveplan_test.go, docs/plans/2026-05-13-cloudflare-ai-worker-execution-waves.json |
| T2 | waveplan-cli native context estimate selectors | 65 | plan, spec, waveplan_cli | waveplan-cli |
| T3 | End-to-end verification and docs cutover | 112 | plan, spec, waveplan_cli, adapter_waveplan_test_go, cloudflare_execution_waves | internal/contextsize/adapter/waveplan_test.go, waveplan-cli, docs/superpowers/specs/2026-05-13-context-sizer-design.md, docs/plans/2026-05-13-cloudflare-ai-worker-execution-waves.json |

## Units
| unit_id | task_id | title | kind | wave | plan_line | depends_on | fp_refs | doc_refs |
|---|---|---|---|---:|---:|---|---|---|
| T1.1 | T1 | Write adapter tests first | test | 1 | 30 | - | FP-ADAPTER-MAP | plan, spec, adapter_waveplan_test_go, cloudflare_execution_waves |
| T1.2 | T1 | Implement task and unit mapping | impl | 2 | 40 | T1.1 | FP-ADAPTER-MAP | plan, spec, adapter_waveplan_go, cloudflare_execution_waves |
| T1.3 | T1 | Verify adapter package tests | verify | 3 | 55 | T1.2 | FP-ADAPTER-MAP | plan, adapter_waveplan_test_go |
| T2.1 | T2 | Add native selector flags | impl | 4 | 69 | T1.3 | FP-ADAPTER-CLI | plan, spec, waveplan_cli |
| T2.2 | T2 | Enforce selector validation and route through adapter | impl | 5 | 84 | T2.1 | FP-ADAPTER-CLI | plan, spec, waveplan_cli, adapter_waveplan_go |
| T2.3 | T2 | Verify candidate and native plan paths | verify | 6 | 100 | T2.2 | FP-ADAPTER-CLI | plan, waveplan_cli, cloudflare_execution_waves |
| T3.1 | T3 | Add fixture-backed end-to-end tests | test | 7 | 116 | T2.3 | FP-ADAPTER-E2E | plan, adapter_waveplan_test_go, cloudflare_execution_waves |
| T3.2 | T3 | Update docs for normal workflow | doc | 8 | 124 | T3.1 | FP-ADAPTER-E2E | plan, spec |
| T3.3 | T3 | Verify install path and wrapper behavior | verify | 9 | 130 | T3.2 | FP-ADAPTER-E2E | plan, waveplan_cli, cloudflare_execution_waves |
