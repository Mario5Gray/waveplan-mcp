# raft-cli-split

## Meta
| key | value |
|---|---|
| schema_version | 1 |
| generated_on | 2026-05-15 |
| plan_version | 1 |
| plan_generation | 2026-05-15T00:00:00Z |

## Plan
| plan_id | plan_title | plan_doc_path | spec_doc_path |
|---|---|---|---|
| raft-cli-split | Raft CLI Split | docs/superpowers/plans/2026-05-15-raft-cli-split-swim.md | docs/superpowers/specs/2026-05-15-raft-cli-split-design.md |

## Doc Index
| ref | path | line | kind |
|---|---|---:|---|
| main_go | main.go | 1 | code |
| main_test_go | main_test.go | 1 | test |
| makefile | Makefile | 1 | code |
| plan | docs/superpowers/plans/2026-05-15-raft-cli-split-swim.md | 1 | plan |
| raft_cli | raft | 1 | code |
| readme | README.md | 1 | doc |
| spec | docs/superpowers/specs/2026-05-15-raft-cli-split-design.md | 1 | spec |
| swim_cli_scaffold_test | tests/swim/test_t4_1_cli_scaffold.sh | 1 | test |
| swim_cli_wired_test | tests/swim/test_t4_2_cli_wired.sh | 1 | test |
| swim_compile_plan_json_test | tests/swim/test_compile_plan_json.sh | 1 | test |
| swim_docs_test | tests/swim/test_t4_3_docs_present.sh | 1 | test |
| swim_insert_fix_loop_test | tests/swim/test_t7_2_insert_fix_loop.sh | 1 | test |
| swim_installed_anydir_test | tests/swim/test_installed_anydir.sh | 1 | test |
| swim_passthrough_test | tests/swim/test_t7_3_review_schedule_passthrough.sh | 1 | test |
| swim_planjson_go | internal/swim/planjson.go | 1 | code |
| swim_planjson_test_go | internal/swim/planjson_test.go | 1 | test |
| swim_review_sidecar_insert_go | internal/swim/review_sidecar_insert.go | 1 | code |
| swim_review_sidecar_insert_test_go | internal/swim/review_sidecar_insert_test.go | 1 | test |
| waveplan_cli | waveplan-cli | 1 | code |

## FP Index
| fp_ref | fp_id |
|---|---|


## Tasks
| task_id | title | plan_line | doc_refs | files |
|---|---|---:|---|---|
| T1 | Bring the SWIM MCP surface to CLI parity | 42 | plan, spec, main_go, main_test_go, waveplan_cli | main.go, main_test.go |
| T2 | Move the remaining SWIM-only client logic into Go/core packages | 118 | plan, spec, main_go, main_test_go, swim_planjson_go, swim_planjson_test_go, swim_review_sidecar_insert_go, swim_review_sidecar_insert_test_go | internal/swim/planjson.go, internal/swim/planjson_test.go, internal/swim/review_sidecar_insert.go, internal/swim/review_sidecar_insert_test.go, main.go, main_test.go |
| T3 | Add `raft` as the canonical SWIM CLI | 181 | plan, spec, raft_cli, main_go, waveplan_cli, swim_cli_scaffold_test, swim_cli_wired_test, swim_insert_fix_loop_test, swim_passthrough_test, swim_compile_plan_json_test, swim_installed_anydir_test | raft, tests/swim/test_t4_1_cli_scaffold.sh, tests/swim/test_t4_2_cli_wired.sh, tests/swim/test_t7_2_insert_fix_loop.sh, tests/swim/test_t7_3_review_schedule_passthrough.sh, tests/swim/test_compile_plan_json.sh, tests/swim/test_installed_anydir.sh |
| T4 | Remove the `swim` subtree from `waveplan-cli` | 263 | plan, spec, waveplan_cli | waveplan-cli |
| T5 | Packaging, docs, and cutover | 315 | plan, spec, makefile, readme, swim_docs_test | Makefile, README.md, tests/swim/test_t4_3_docs_present.sh |
| T6 | Final verification | 358 | plan, spec, main_go, waveplan_cli, raft_cli, swim_cli_scaffold_test, swim_cli_wired_test, swim_docs_test, swim_installed_anydir_test, swim_insert_fix_loop_test, swim_passthrough_test, swim_compile_plan_json_test | main.go, waveplan-cli, raft, tests/swim/test_t4_1_cli_scaffold.sh, tests/swim/test_t4_2_cli_wired.sh, tests/swim/test_t4_3_docs_present.sh, tests/swim/test_installed_anydir.sh, tests/swim/test_t7_2_insert_fix_loop.sh, tests/swim/test_t7_3_review_schedule_passthrough.sh, tests/swim/test_compile_plan_json.sh |

## Units
| unit_id | task_id | title | kind | wave | plan_line | depends_on | fp_refs | doc_refs |
|---|---|---|---|---:|---:|---|---|---|
| T1.1 | T1 | Expand SWIM MCP tool registration and argument surface | impl | 1 | 46 | - | - | plan, spec, main_go, main_test_go |
| T1.2 | T1 | Add parity handler coverage for new tools and passthrough arguments | test | 2 | 83 | T1.1 | - | plan, spec, main_go, main_test_go |
| T1.3 | T1 | Verify the server-side SWIM MCP suite before cutover | verify | 3 | 95 | T1.2 | - | plan, main_go, main_test_go |
| T2.1 | T2 | Port deterministic plan canonicalization into internal/swim | impl | 4 | 122 | T1.3 | - | plan, spec, swim_planjson_go, swim_planjson_test_go, waveplan_cli |
| T2.2 | T2 | Port deterministic review-sidecar insertion with regression coverage | impl | 5 | 139 | T2.1 | - | plan, spec, swim_review_sidecar_insert_go, swim_review_sidecar_insert_test_go, swim_insert_fix_loop_test |
| T2.3 | T2 | Wire core SWIM conversion functions into MCP handlers and verify | verify | 6 | 161 | T2.2 | - | plan, spec, main_go, main_test_go, swim_planjson_go, swim_review_sidecar_insert_go |
| T3.1 | T3 | Create the raft CLI surface as a Python MCP proxy | impl | 7 | 185 | T2.3 | - | plan, spec, raft_cli, waveplan_cli, main_go |
| T3.2 | T3 | Map raft subcommands to SWIM MCP tools without changing CLI semantics | impl | 8 | 209 | T3.1 | - | plan, spec, raft_cli, waveplan_cli, main_go |
| T3.3 | T3 | Cut the SWIM shell suite over to raft | test | 9 | 226 | T3.2 | - | plan, spec, raft_cli, swim_cli_scaffold_test, swim_cli_wired_test, swim_insert_fix_loop_test, swim_passthrough_test, swim_compile_plan_json_test, swim_installed_anydir_test |
| T4.1 | T4 | Remove the swim parser, helpers, and dispatch path from waveplan-cli | impl | 10 | 267 | T3.3 | - | plan, spec, waveplan_cli |
| T4.2 | T4 | Verify Waveplan-only CLI behavior and add the swim regression guard | verify | 11 | 281 | T4.1 | - | plan, spec, waveplan_cli |
| T5.1 | T5 | Update helper installation and uninstall flows for raft | impl | 12 | 319 | T4.2 | - | plan, spec, makefile |
| T5.2 | T5 | Cut README and docs-presence checks over to raft | doc | 13 | 327 | T5.1 | - | plan, spec, readme, swim_docs_test |
| T5.3 | T5 | Verify install and documentation surfaces after the cutover | verify | 14 | 340 | T5.2 | - | plan, spec, makefile, readme, swim_docs_test |
| T6.1 | T6 | Run the full Go, shell, and manual verification matrix | verify | 15 | 360 | T5.3 | - | plan, spec, main_go, waveplan_cli, raft_cli, swim_cli_scaffold_test, swim_cli_wired_test, swim_docs_test, swim_installed_anydir_test, swim_insert_fix_loop_test, swim_passthrough_test, swim_compile_plan_json_test |
