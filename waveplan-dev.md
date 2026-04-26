---
Goal
- Build and refine scripts/waveplan CLI for managing execution waves from *-execution-waves.json plans, including a reference skill
- Bring Go MCP service (waveplan-mcp) to feature parity with Python CLI
Constraints & Preferences
- No parallel execution — no re-entrant code needed
- started_at must be set when pop is called (or on fin if pop was skipped)
- Agent name (taken_by) must persist through fin into completed
- get output must show both started and finished timestamps
- Subcommand help text should include argument signatures
- Only one task may be added or removed per write (agnes write-guard, then removed)
Progress
Done
- Added get [mode] with filter modes: all (default), taken, open, task-<id>, <agent>
- Added start_review <task_id> <reviewer> and end_review <task_id> commands
- Review timestamps (review_entered_at, review_ended_at, reviewer) stored in state and displayed by get
- cmd_fin preserves started_at, taken_by, and review info when moving task from taken to completed
- Help text includes inline argument signatures (e.g., pop <agent>, fin <task_id>)
- Created ~/.agents/skills/w ripper/SKILL.md documenting all commands and workflow
- Saved state相对较高 per-task writes with snapshot/diff guard (later removed)
- Created waveplan-mcp feature parity design spec (2026-04-25)
- Created waveplan-mcp implementation plan (2026-04-25)
- Created waveplan-mcp execution waves plan (2026-04-25) — 11 units, 5 waves
- Implemented all 11 tasks across 5 waves (T1.1 → T2.1 → T3.1-T9.1 → T10.1 → T11.1)
- All 15 Go tests pass, build clean
In Progress
- None — all waveplan modifications complete
Blocked
- None
Key Decisions
- Remove write-guard: User confirmed no parallel execution, so snapshot/diff guard was unnecessary
- Review workflow: pop → work → start_review → end_review → fin — review times preserved in completed for audit trail
- get task-<id>: Single lookup mode for full task detail without filtering
- Inversey in completed: fin migrates started_at, taken_by, review_entered_at, review_ended_at, reviewer into completed entry so get all shows full history
- waveplan-mcp: Go MCP service at main.go — JSON output on all tools, deptree mode, review_note on end_review, git_sha on fin
- State file format unchanged — new fields (review_note, git_sha) default to empty strings
- buildTaskEntry extracts shared task-building logic; doDeptree is unlocked helper to avoid sync.Mutex deadlock
- taskInfo emits plan_ref (for peek/pop); buildTaskEntry renames to plan (for get/deptree) to match CLI/spec shape
Next Steps
- None explicitly — waveplan CLI and MCP service are feature-complete
Critical Context
- State file: docs/superpowers/plans/2026-04-22-controlnet-track-3-backend-execution-waves.json.state.json
- Plan file: docs/superpowers/plans/2026-04-22-controlnet-track-3-backend-execution-waves.json
- Skill file: ~/.agents/skills/waveplan/SKILL.md (400 words)
- Plan has 45 units (T1.1–T7.6), various tasks already taken/completed by theta, sigma, psi
- waveplan-mcp: main.go (833 lines), main_test.go (15 tests), go.mod, Makefile.test
- All 15 tests pass: findPlanRef, resolveDocRefs, resolveFpRefs, nilIfEmpty, taskInfo, topologicalSort (3 variants), buildTaskEntry, fin backfill, deterministic ordering, deptree ordering, plan vs plan_ref
Relevant Files
- scripts/waveplan: Main CLI — all changes live here
- ~/.agents/skills/waveplan/SKILL.md: Reference skill for future agents
- docs/superpowers/plans/2026-04-22-controlnet-track-3-backend-execution-waves.json: Execution plan with 45 units
- docs/superpowers/plans/2026-04-25-waveplan-mcp-feature-parity-execution-waves.json: waveplan-mcp execution plan (11 units, 5 waves)
- docs/superpowers/plans/2026-04-25-waveplan-mcp-feature-parity.md: waveplan-mcp implementation plan (11 tasks)
- docs/superpowers/specs/2026-04-25-waveplan-mcp-feature-parity-design.md: waveplan-mcp design spec
- main.go: Go MCP service (833 lines, full rewrite with JSON output)
- main_test.go: 15 unit tests covering helpers, ordering, and parity
---