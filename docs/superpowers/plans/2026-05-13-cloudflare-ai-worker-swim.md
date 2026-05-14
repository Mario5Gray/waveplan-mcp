# cloudflare-ai-worker

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
| cloudflare-ai-worker | Cloudflare AI Worker Integration | docs/superpowers/plans/2026-05-13-cloudflare-ai-worker.md | docs/superpowers/plans/2026-05-13-cloudflare-ai-worker.md |

## Doc Index
| ref | path | line | kind |
|---|---|---:|---|
| cloudflare_worker_env_example | cloudflare-worker/.env.example | 1 | code |
| cloudflare_worker_esbuild_config_js | cloudflare-worker/esbuild.config.js | 1 | code |
| cloudflare_worker_metadata_json | cloudflare-worker/metadata.json | 1 | code |
| cloudflare_worker_package_json | cloudflare-worker/package.json | 1 | code |
| cloudflare_worker_scripts_deploy_sh | cloudflare-worker/scripts/deploy.sh | 1 | test |
| cloudflare_worker_scripts_test_sh | cloudflare-worker/scripts/test.sh | 1 | test |
| cloudflare_worker_src_types_ts | cloudflare-worker/src/types.ts | 1 | code |
| cloudflare_worker_src_worker_ts | cloudflare-worker/src/worker.ts | 1 | code |
| cloudflare_worker_tsconfig_json | cloudflare-worker/tsconfig.json | 1 | code |
| internal_aiclient_client_go | internal/aiclient/client.go | 1 | code |
| internal_aiclient_client_test_go | internal/aiclient/client_test.go | 1 | test |
| main_go | main.go | 1 | code |
| plan | docs/superpowers/plans/2026-05-13-cloudflare-ai-worker.md | 1 | plan |
| spec | docs/superpowers/plans/2026-05-13-cloudflare-ai-worker.md | 1 | spec |

## FP Index
| fp_ref | fp_id |
|---|---|
| FP-CLIENT | FP-sscpykry#FP-CLIENT |
| FP-DEPLOY | FP-sscpykry#FP-DEPLOY |
| FP-SECURITY | FP-sscpykry#FP-SECURITY |
| FP-SERVER | FP-sscpykry#FP-SERVER |
| FP-WORKER | FP-sscpykry#FP-WORKER |

## Tasks
| task_id | title | plan_line | doc_refs | files |
|---|---|---:|---|---|
| T1 | Cloudflare Worker | 27 | plan, cloudflare_worker_src_worker_ts, cloudflare_worker_src_types_ts, cloudflare_worker_esbuild_config_js, cloudflare_worker_package_json, cloudflare_worker_tsconfig_json, cloudflare_worker_metadata_json | cloudflare-worker/src/worker.ts, cloudflare-worker/src/types.ts, cloudflare-worker/esbuild.config.js, cloudflare-worker/package.json, cloudflare-worker/tsconfig.json, cloudflare-worker/metadata.json |
| T2 | Go Client Package — `internal/aiclient` | 71 | plan, internal_aiclient_client_go, internal_aiclient_client_test_go | internal/aiclient/client.go, internal/aiclient/client_test.go |
| T3 | Server Integration Scaffolding | 99 | plan, main_go | main.go |
| T4 | Deployment & Tool Integration | 126 | plan, cloudflare_worker_scripts_deploy_sh, cloudflare_worker_scripts_test_sh, cloudflare_worker_env_example | cloudflare-worker/scripts/deploy.sh, cloudflare-worker/scripts/test.sh, cloudflare-worker/.env.example |
| T5 | Security Hardening & Verification | 176 | plan | - |

## Units
| unit_id | task_id | title | kind | wave | plan_line | depends_on | fp_refs | doc_refs |
|---|---|---|---|---:|---:|---|---|---|
| T1.1 | T1 | Write the worker entry point | impl | 1 | 39 | - | FP-WORKER | plan, cloudflare_worker_src_worker_ts, cloudflare_worker_src_types_ts, cloudflare_worker_esbuild_config_js, cloudflare_worker_package_json, cloudflare_worker_tsconfig_json, cloudflare_worker_metadata_json |
| T1.2 | T1 | Write types and build config | impl | 2 | 44 | T1.1 | FP-WORKER | plan, cloudflare_worker_src_worker_ts, cloudflare_worker_src_types_ts, cloudflare_worker_esbuild_config_js, cloudflare_worker_package_json, cloudflare_worker_tsconfig_json, cloudflare_worker_metadata_json |
| T1.3 | T1 | Build and verify the worker bundle | verify | 3 | 61 | T1.2 | FP-WORKER | plan, cloudflare_worker_src_worker_ts, cloudflare_worker_src_types_ts, cloudflare_worker_esbuild_config_js, cloudflare_worker_package_json, cloudflare_worker_tsconfig_json, cloudflare_worker_metadata_json |
| T2.1 | T2 | Write the client implementation | impl | 4 | 79 | T1.3 | FP-CLIENT | plan, internal_aiclient_client_go, internal_aiclient_client_test_go |
| T2.2 | T2 | Write the test file | test | 5 | 84 | T2.1 | FP-CLIENT | plan, internal_aiclient_client_go, internal_aiclient_client_test_go |
| T2.3 | T2 | Build and test | verify | 6 | 89 | T2.2 | FP-CLIENT | plan, internal_aiclient_client_go, internal_aiclient_client_test_go |
| T3.1 | T3 | Add ai field to WaveplanServer | impl | 7 | 106 | T2.3 | FP-SERVER | plan, main_go |
| T3.2 | T3 | Wire NewClient at startup | impl | 8 | 111 | T3.1 | FP-SERVER | plan, main_go |
| T3.3 | T3 | Build to verify compilation | verify | 9 | 116 | T3.2 | FP-SERVER | plan, main_go |
| T4.1 | T4 | Write the deploy script | impl | 10 | 136 | T3.3 | FP-DEPLOY | plan, cloudflare_worker_scripts_deploy_sh, cloudflare_worker_scripts_test_sh, cloudflare_worker_env_example |
| T4.2 | T4 | Write the test script | test | 11 | 141 | T4.1 | FP-DEPLOY | plan, cloudflare_worker_scripts_deploy_sh, cloudflare_worker_scripts_test_sh, cloudflare_worker_env_example |
| T4.3 | T4 | Write .env.example | doc | 12 | 146 | T4.2 | FP-DEPLOY | plan, cloudflare_worker_scripts_deploy_sh, cloudflare_worker_scripts_test_sh, cloudflare_worker_env_example |
| T4.4 | T4 | Integrate AI into handleSwimRefineRun | impl | 13 | 151 | T4.3 | FP-DEPLOY | plan, cloudflare_worker_scripts_deploy_sh, cloudflare_worker_scripts_test_sh, cloudflare_worker_env_example |
| T4.5 | T4 | Integrate AI into handleGet | impl | 14 | 156 | T4.4 | FP-DEPLOY | plan, cloudflare_worker_scripts_deploy_sh, cloudflare_worker_scripts_test_sh, cloudflare_worker_env_example |
| T4.6 | T4 | Implement AI cache (LRU) | impl | 15 | 161 | T4.5 | FP-DEPLOY | plan, cloudflare_worker_scripts_deploy_sh, cloudflare_worker_scripts_test_sh, cloudflare_worker_env_example |
| T4.7 | T4 | Build and test | verify | 16 | 166 | T4.6 | FP-DEPLOY | plan, cloudflare_worker_scripts_deploy_sh, cloudflare_worker_scripts_test_sh, cloudflare_worker_env_example |
| T5.1 | T5 | Add prompt allowlist test | test | 17 | 184 | T4.7 | FP-SECURITY | plan |
| T5.2 | T5 | Implement buildRefinePrompt and buildGetPrompt | impl | 18 | 189 | T5.1 | FP-SECURITY | plan |
| T5.3 | T5 | Run all tests | verify | 19 | 194 | T5.2 | FP-SECURITY | plan |
