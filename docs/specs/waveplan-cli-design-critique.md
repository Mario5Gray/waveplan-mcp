### 1. Critical Gaps (max 10)
- Gap: Planned owner does not exist in `waveplan-plan-schema.json`, but drift requires `plannedOwner` comparison. | Why it matters: Owner drift cannot be computed deterministically. | Concrete failure scenario: `owner` drift bucket shows zero while operators manually see reassignment drift.
- Gap: `failed`, `blocked`, and `skipped` require explicit markers, but state schema has no `failed_at`, `blocked_reason`, or `skipped` field. | Why it matters: Status precedence cannot realize half the vocabulary. | Concrete failure scenario: A task operationally failed, dashboard still shows `running` or `waiting`.
- Gap: Artifact drift is specified, but neither schema defines expected/generated artifact structures. | Why it matters: `artifacts` drift bucket is unimplementable. | Concrete failure scenario: Build output missing, dashboard still reports no artifact drift.
- Gap: Verification panel requires checks (`tests/lint/typecheck/build/review/acceptance`), but state schema has no per-check model. | Why it matters: Tile counts and failure lists are synthetic/non-authoritative. | Concrete failure scenario: `28/35` displayed from stale cache while latest run is failing.
- Gap: No snapshot/version contract across plan/state/event feeds. | Why it matters: Cross-file race conditions create mixed-frame truth. | Concrete failure scenario: Plan v2 + state v1 renders false `extra/missing` drift spikes every poll.
- Gap: Timestamp format is non-ISO (`YYYY-MM-DD HH:MM`) with no timezone or monotonic source. | Why it matters: Ordering and freshness are ambiguous across agents/machines. | Concrete failure scenario: Older update appears newer; wrong active task shown.
- Gap: Conflict resolution for unit simultaneously present in `taken` and `completed` is undefined. | Why it matters: Real-world write races are guaranteed at 1–2s polling. | Concrete failure scenario: Completed task flickers `running` because precedence checks `running` before `complete`.
- Gap: “Current active task” is treated as singular, but model allows multiple concurrent `taken` units. | Why it matters: Drawer, summary bar, and “next action” become nondeterministic. | Concrete failure scenario: Two running units; UI arbitrarily picks one and hides the other blocker.
- Gap: No lifecycle policy for unit identity changes (rename, split, merge, deletion) between plan revisions. | Why it matters: Drift explodes without semantic mapping. | Concrete failure scenario: Regenerated plan renumbers units; dashboard reports mass `missing/extra`.
- Gap: Parse-fail behavior pins last-good frame but does not define TTL or “truth degraded” state. | Why it matters: Operators may act on stale-but-plausible data. | Concrete failure scenario: State parsing fails for 10 minutes; UI still looks live except heartbeat dot.

### 2. Contradictions or Tensions
- Conflict: “Density is default” vs “answer 6 questions in 5s.” | What breaks: N1 truncation hides owner/stage/deps; glance target fails at scale. | Suggested resolution direction: Define mandatory always-visible fields for 5-second questions, even in N1.
- Conflict: Real-time updates vs strict layout stability (“no reflow”). | What breaks: Dynamic wave counts, collapse/expand, and edge reroutes force positional churn. | Suggested resolution direction: Freeze coordinates per unit ID within session; only animate explicit user actions.
- Conflict: Verification tiles sorted by severity each tick vs deterministic scanability. | What breaks: Tile order jumps every poll; muscle memory fails. | Suggested resolution direction: Fixed tile order, severity shown as badge/left bar only.
- Conflict: Pipeline strip as status filter vs mapping excludes `blocked/failed/waiting`. | What breaks: Operators click stage tiles and cannot isolate major failure states. | Suggested resolution direction: Add explicit failure/waiting facets or make exclusion visibly explicit in strip.
- Conflict: “Color is state” vs grayscale resilience requirement. | What breaks: Drift/failure urgency collapses when color unavailable; tiny icons insufficient. | Suggested resolution direction: Add redundant shape/pattern encoding at node and edge levels.
- Conflict: Drift as additive overlay vs health pill single-state semantics. | What breaks: `failed + drift` can collapse into ambiguous top health signal. | Suggested resolution direction: Define strict health priority matrix with multi-flag rendering.
- Conflict: Drawer “always visible by default” vs graph-heavy troubleshooting mode. | What breaks: 8-column graph becomes too compressed for orthogonal edge readability. | Suggested resolution direction: Persist per-user drawer state and provide quick temporary peek behavior.
- Conflict: “No modal alerts” vs stale/error/partial data criticality. | What breaks: Severe data integrity issues can be visually subtle during high cognitive load. | Suggested resolution direction: Keep non-modal design but reserve a persistent high-salience degraded-data strip.

### 3. Scalability Limits
- Component that fails first: Orthogonal edge router in dense lanes. | Why: Edge crossings, jog count, and non-overlap constraints grow rapidly with 200–1000 nodes. | Result: Jitter, overlaps, and frame drops during every poll.
- Component that fails first for layout stability: N2/N3 hover/spotlight overlays plus lineage highlighting. | Why: Many concurrent opacity/shadow changes trigger expensive repaints. | Result: Cursor lag and highlight “swim.”
- Component that fails first for cognitive load: Drift buckets + verification panel + ticker competing for urgency. | Why: Three simultaneous priority channels without a global arbitration rule. | Result: Operator misses blocker source despite correct raw data.
- Component that fails first for interaction throughput: Auto-scroll on drift chip click during live updates. | Why: Layout updates invalidate scroll target mid-animation. | Result: Scroll lands on wrong node or oscillates.
- Component that fails first for deterministic reading: Severity-sorted verification tiles. | Why: Reordering every 1–2s at 30-wave scale. | Result: Users reread from scratch each poll.
- Component that fails first for rendering budget: 10-second recent-update glows across many nodes. | Why: Continuous fade effects multiply with update cadence. | Result: GPU overdraw and poor FPS.

### 4. Interaction Risks
- Risk: AND-across / OR-within filter logic is nontrivial in dense state. | User misinterpretation: “I selected owner+wave and tasks vanished; system broken.” | Observed outcome: False bug reports, missed tasks.
- Risk: Mode switching (`Default/Spotlight/Diff`) lacks explicit precedence with active filters. | User misinterpretation: Hidden nodes interpreted as deleted. | Observed outcome: Incorrect operational decisions.
- Risk: Drawer “Next Action” is state-driven but state may be stale/out-of-order. | User misinterpretation: Button implies safe action. | Observed outcome: Premature verify/unblock actions.
- Risk: Hover-based lineage brightening in dense graph. | User misinterpretation: Incidental hover appears as authoritative focus state. | Observed outcome: Attention thrash, wrong dependency reading.
- Risk: Collapsed wave hides unresolved blockers downstream. | User misinterpretation: “Wave is quiet.” | Observed outcome: Blocked work goes unnoticed.
- Risk: Drift chip click forcibly changes viewport/context. | User misinterpretation: Context jump feels like selection loss. | Observed outcome: Operator disorientation during incident response.
- Risk: Single active drawer in multi-running scenarios. | User misinterpretation: Non-shown running task assumed idle. | Observed outcome: Ownership and progress confusion.

### 5. State & Data Model Weaknesses
- Plan/state comparison relies on fields not guaranteed by schemas (`owner`, failure markers, artifact sets).
- Drift computation has no confidence model for partial state; absent data can look like negative drift.
- Status precedence is race-sensitive (`running` before `complete`) and can regress visible status on transient overlaps.
- Timestamp semantics are weak: no timezone, no monotonic sequence, no writer clock authority.
- Out-of-order polls can regress the UI without version checks (older snapshot can overwrite newer view).
- Dependency drift definition is underspecified: “satisfied deps differ from declared deps” conflates plan topology with runtime completion.
- Ownership conflicts are undefined when `taken_by` differs across `taken` and `completed` for same unit.
- Derived metrics (`verificationPassRate`, `dependencyReadiness`) lack denominator rules under missing/extra units.

### 6. Edge Cases Not Covered
- Cyclic dependencies in plan DAG.
- Self-dependency (`T1.2 -> T1.2`).
- Unit exists in `waves[]` but missing from `units` map (and inverse).
- Plan regenerated mid-run with wave renumbering.
- Task deleted from plan while still present in state.
- Multiple active tasks across different waves.
- Unit moved to another wave after being taken.
- Repeated review enter/exit flapping in short intervals.
- State file partially written/truncated during poll read.
- Heartbeat alive but state unchanged (silent orchestrator stall).
- `totalTasks = 0` causing divide-by-zero progress/percent bars.
- Very long labels/paths overflowing N1/N2 cards and collision with badges/icons.

### 7. Visual System Stress Test
- Grayscale: `running` vs `verifying` both rely on blue family plus similar glyph complexity; distinction weak.
- Low contrast: `gray3/gray6` on compact cards plus 13px text risks unreadability under glare/remote screens.
- Colorblindness: red/green edge and status discrimination is fragile without stronger shape/pattern redundancy.
- Small icons (16px, 1.5px stroke): `circle-dashed`, `circle-dashed-check`, and `progress-check` are ambiguous at N1 density.
- Drift badge (12px overlay) is too small under high zoom-out; may disappear in clutter.
- Dashed vs solid edges at high edge density becomes visually noisy; arrowheads become indistinct.
- Stale desaturation (10–15%) can erase already subtle state differences instead of clarifying degradation.

### 8. Behavioral Integrity
- Rapid mode toggling during 1–2s polling can apply transforms to stale node sets, causing highlight/selection ghosts.
- Filters + Spotlight + Diff has no explicit precedence contract; same state can produce conflicting visibility outcomes.
- Coexistence of stale + parse error + partial data has no single truth policy; users can see mixed degradation signals.
- Selection persistence is undefined when selected node is filtered out, moved wave, or disappears from snapshot.
- “Last good frame” with live heartbeat/events can produce incoherent mixed-time UI.
- Pulse-highlight from drift chip can target node no longer on screen after concurrent layout update.

### 9. Implementation Risk Assessment
- Deceptively complex: Orthogonal DAG routing with no node overlap, lane collapses, and stable coordinates under live updates.
- Likely bug source: Snapshot diffing without monotonic revision IDs (plan/state/event race conditions).
- Likely bug source: Status derivation precedence under partial writes and concurrent transitions.
- Likely bug source: Cross-component synchronization of selection, drawer state, filters, and viewport scroll targets.
- Requires strict contract: Unit identity immutability and revision mapping across plan regenerations.
- Requires strict contract: Timestamp format/timezone/ordering guarantees.
- Requires strict contract: Verification result schema, failure taxonomy, and ownership semantics.
- Requires strict contract: Degraded-data rendering states (stale vs error vs partial) with explicit operator-facing truth level.

### 10. What You Would Lock or Change
- Lock harder: Snapshot contract with monotonic `revision_id` and atomic plan/state coherence rules.
- Lock harder: Status derivation truth table (including tie-breakers for `taken+completed`, partial fields, and failure overrides).
- Lock harder: Layout determinism contract (stable node coordinates per unit ID, bounded motion rules).
- Loosen/simplify: Stop severity-based tile reordering; keep fixed tile positions.
- Loosen/simplify: Reduce card tiers to two operational tiers for predictable density/performance.
- Loosen/simplify: Defer fully custom orthogonal router; use proven layout constraints with explicit edge-overlap fallback indicators.