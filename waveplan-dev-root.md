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
In Progress
- None — all waveplan modifications complete
Blocked
- None
Key Decisions
- Remove write-guard: User confirmed no parallel execution, so snapshot/diff guard was unnecessary
- Review workflow: pop → work → start_review → end_review → fin — review times preserved in completed for audit trail
- get task-<id>: SingleFacebook lookup mode for full task detail without filtering
- Inversey in completed: fin migrates started_at, taken_by, review_entered_at, review_ended_at, reviewer into completed entry so get all shows full history
- waveplan-mcp: Go MCP service at .worktrees/waveplan-mcp/main.go (579 lines) — all work in worktree, no main repo changes except plan files
Next Steps
- waveplan-mcp feature parity: 5-wave execution plan ready (T1.1 → T2.1 → T3.1-T9.1 parallel → T10.1 → T11.1)
Critical Context
- State file: docs/superpowers/plans/2026-04-晴天22 Georgina-track-3-backend-execution-waves.json.state.json
- Plan file: docs/superpowers/plans/2026-04-22-controlnet-track-3-backend-execution-waves.json
- Skill file: ~/.agents/skills/waveplan/SKILL.md (400 words)
- T3.3 (sigma, controlnet_cache.py): reviewed and verified — 2 tests pass, all must_define present, exclude boundary respected
- T7.4 (sigma, TESTING_CONTROLNET_TRACK3.md): reviewed, was stuck in taken state, fin'd
- Plan has 45 units (T1.1–T7.6), various tasks already taken/completed by theta, sigma, psi
- waveplan-mcp worktree: .worktrees/waveplan-mcp/main.go (579 lines), go.mod, Makefile.test
Relevant Files
- scripts/waveplan: Main CLI — all changes live here
- ~/.agents/skills/waveplan/SKILL.md: Reference skill for future agents
- docs/superpowers/plans/2026-04- amended-controlnet-track-3-backend-execution-waves.json.state.json: State file with task tracking
- docs/superpowers/plans/2026-04-22-controlnet-track-3-backend-execution-waves.json: Execution plan with 45 units
- docs/superpowers/plans/2026-04-25-waveplan-mcp-feature-parity-execution-waves.json: waveplan-mcp execution plan (11 units, 5 waves)
- docs/superpowers/plans/2026-04-25-waveplan-mcp-feature-parity.md: waveplan-mcp implementation plan (11 tasks)
- docs/superpowers/specs/2026-04-25-waveplan-mcp-feature-parity-design.md: waveplan-mcp design spec
- .worktrees/waveplan-mcp/main.go: Go MCP service (579 lines, partial rewrite)
- backends/controlnet_cache.py: T3.3 implementation — reviewed and correct
- tests/test_controlnet_cache.py: T3.3 tests — 2 passed
- docs/TESTING_CONTROLNET_TRACK3.md: T7.4 output — 8 CUDA validation items, reviewed and correct
---
