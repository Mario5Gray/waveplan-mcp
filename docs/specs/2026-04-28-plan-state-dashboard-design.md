# Plan vs State Dashboard — Design

**Date:** 2026-04-28
**Status:** Draft
**Inputs:** `*-execution-waves.json` (plan), `*-execution-waves.json.state.json` (state)

## Purpose

A real-time operational dashboard that compares planned execution (waveplan) against current state and exposes drift, blockers, and the next action within a five-second glance. The panel is not a document viewer; it is the cockpit for the agentic code factory.

A user must be able to answer in five seconds:

1. What is the current active task?
2. Which agent owns it?
3. What stage is it in?
4. Is it blocked, running, waiting, failed, or complete?
5. What changed compared to the plan?
6. What should happen next?

## Design Philosophy

These principles govern every later decision. When two principles conflict, the earlier one wins.

1. Edges carry truth. Nodes carry identity. Color carries urgency.
2. Density is the default. Detail is a privilege earned by focus.
3. The pipeline tells you what the system is doing. The graph tells you why.
4. The center shows the work. The right shows the now. The bottom shows the truth.
5. Don't describe the problem. Put the fix under the user's cursor.
6. Drift is not data. Drift is navigation.
7. Show health globally. Show failure locally. Fix instantly.
8. If the graph shows structure, the footer shows life.
9. Color is not decoration. Color is state.
10. Text should disappear. Numbers should snap into place.
11. Icons tell you what it is. Color tells you how bad it is.
12. Filter narrows the world. Mode changes how you see it.
13. States should guide attention, not compete for it.

## Status Vocabulary

A unit (task) carries exactly one derived status at any time:

`planned` · `ready` · `running` · `waiting` · `blocked` · `failed` · `verifying` · `complete` · `skipped` · `drift`

`drift` is an additive flag rendered as an overlay badge; the underlying status remains one of the others. Drift means the actual state diverges from what the plan declared (owner, status, dependencies, or artifacts).

## Visual System

### Color — Radix Colors (light-only v1, dark-mode-token-ready)

Each status maps to a four-token slot from a Radix 12-step scale.

| status | scale | bg | border | solid | text |
|---|---|---|---|---|---|
| planned / ready / skipped / inactive | gray | gray3 | gray6 | gray9 | gray12 |
| running | blue | blue3 | blue6 | blue9 | gray12 |
| verifying | indigo | indigo3 | indigo6 | indigo9 | gray12 |
| waiting | amber | amber3 | amber6 | amber9 | gray12 |
| failed / blocked | red | red3 | red6 | red9 | gray12 |
| complete | green | green3 | green6 | green9 | gray12 |
| drift (overlay) | purple | purple3 | purple6 | purple9 | purple11 |

`ready` and `planned` share the gray scale; the icon (`circle-dashed` vs `circle`) carries the distinction.

Token contract per status:

```
status.<name> = { bg: scale3, border: scale6, solid: scale9, text: scale11 }
```

Dark mode is not required for v1, but every consumer of color must read from the token contract above so a dark palette can swap in without code changes.

### Typography — Inter + JetBrains Mono

| token | value |
|---|---|
| sans family | `Inter, system-ui, sans-serif` |
| mono family | `JetBrains Mono, ui-monospace, monospace` |
| base size | 13px |
| size scale | 11 / 12 / 13 / 14 / 16 / 20 |
| weights | 400 (body) · 500 (emphasized) · 600 (headers, IDs) |
| numerals | `font-variant-numeric: tabular-nums` enabled globally |

IDs (`T1.2`), file paths, timestamps, and counts (`5/7`, `0:03`) render in the mono family.

### Iconography — Tabler (outline, 1.5px stroke, 16px box)

| status | glyph | color |
|---|---|---|
| planned | `circle` | gray |
| ready | `circle-dashed` | gray |
| running | `loader-2` (continuous spin) | blue |
| waiting | `clock` | amber |
| blocked | `lock` | red |
| failed | `alert-triangle` | red |
| verifying | `shield-check` (static; fallback `progress-check`) | indigo |
| complete | `circle-check` | green |
| skipped | `arrow-right-circle` (faded) | gray |
| drift | `git-compare` (overlay badge, 14px, 16px white halo) | purple |

Motion is reserved for the `running` spinner only. Drift renders as a small purple `git-compare` overlay at the top-right corner of any node card whose actual state diverges from the plan.

## Graph Primitives

### Pipeline Shape — Wave Swimlanes (B4)

The plan's `wave` field is the spatial backbone. Each wave is a horizontal swimlane stacked top-to-bottom. Status is encoded on each card via icon and color, not by column position.

Swimlane header (left side):

```
┌──────────────┐
│ Wave 3   12u │  ▾
└──────────────┘
```

- Label: `Wave N`
- Count: number of units in lane
- Caret: collapse / expand
- Active wave: subtly tinted background (slate3) — the wave containing the currently running unit
- Collapsed wave header: when a wave is collapsed, its header expands to surface internal urgency that can no longer be seen on cards. Format: `Wave 3   12u   ⚠ 2 failed · 🔒 1 blocked · ⟳ 3 running · 5 ready`. Counts use the same status icons used on cards. A collapsed wave with any `failed` or `blocked` count gets a thin red left bar on its header so quiet collapsed waves cannot hide loud problems.

Lanes flex to content height. Empty waves render as a thin "no units" stub.

#### Layout Stability

Once a unit ID has been laid out within a session, its `(x, y)` rectangle is frozen until the user takes an explicit reflow action (collapse a wave, change filter scope, reload the plan, switch device width). Status changes, count changes, drift toggles, and verification updates do not move cards.

Implementation rules:

1. The compare layer publishes new `ComparedView` snapshots on every poll, but the layout function consumes only `(unitId, wave)` pairs and a stable cross-session sort key (unit ID lexicographic). Anything else changing — owner, status, deps satisfied, verification counts — must not perturb position.
2. New units that appear mid-session land at the end of their wave's row, not interleaved.
3. Removed units (vanishing from the plan) leave their slot empty but reserved until the user reloads or explicitly collapses the wave; the empty slot renders as a 1px dashed gray ghost so the operator sees what disappeared.
4. Edge rerouting follows the same rule: orthogonal corridors are computed from frozen rects and only re-routed on explicit reflow.
5. Any layout transition that does occur (user-triggered) animates over 200ms; never instant.

### Edges — Orthogonal With State Encoding (E3)

| property | value |
|---|---|
| routing | strictly orthogonal (`|_` right angles, no curves, no diagonals) |
| arrowhead | small chevron, target end only, same color as edge |
| line style | solid when dependency satisfied; dashed when pending |
| color priority | failed > active > satisfied > pending |

Color map:

| edge state | trigger | color |
|---|---|---|
| pending | source not yet `complete` and target not waiting on it | gray9 |
| satisfied | source `complete` | green9 |
| blocking-active | target is `waiting` and source is `running` or `verifying` | amber9 |
| blocking-failed | source is `failed` or `blocked` | red9 |

Animation is intentionally absent in v1. A future toggle may animate flow on edges feeding the running unit.

#### Redundant Encoding for Grayscale Resilience

Color is one channel; the design must not collapse to a single channel under accessibility constraints (colorblindness, grayscale screenshots, e-ink, low-end displays).

Edges encode urgency on three independent axes:

| state | color | dash pattern | weight |
|---|---|---|---|
| pending | gray9 | dashed `4 4` | 1.5px |
| satisfied | green9 | solid | 1.5px |
| blocking-active | amber9 | solid | 2px |
| blocking-failed | red9 | solid + double-stroke (two parallel 1px lines, 2px gap) | 2px |

Nodes encode urgency on three axes:

1. **Color** (status palette) — primary channel.
2. **Glyph shape** — Tabler icon set chosen so each status's icon silhouette is distinct: ring vs check vs triangle vs lock vs clock vs spinner vs dashed-ring etc.
3. **Border pattern** — `failed` and `blocked` cards add a 2px dashed border (over the standard 1px solid). `complete` adds an inner 1px ring inset 2px from the edge. `running` keeps the solid border but adds a 2px-tall colored top bar.

A grayscale screenshot must reveal the same status by shape + dash + bar alone. The acceptance test mandates this verification.

### Node Cards — Adaptive (N4)

A single card renders at one of three density tiers based on focus state. Switching tiers must NOT cause swimlane reflow; expansion happens via overlay or scale transform anchored at the card's center.

| tier | size | trigger | content |
|---|---|---|---|
| N1 compact | ~140 × 56 | default | row 1: `T1.2` + status icon + drift overlay + blocking-dep dot · row 2: truncated label · row 3 (micro): owner initials avatar (3ch) + `d:3/4` mono |
| N2 standard | ~180 × 84 | hover (overlay) | N1 + full owner pill + `5/7v` checks + last-update age |
| N3 tall | ~200 × 108 | active / spotlighted (pinned) | N2 + kind chip (impl/test/verify/doc/refactor) + wave badge + blockers preview |

Density does not buy itself by hiding the answers to the five-second questions. N1 must always render: task ID, derived status (icon + color + shape), owner identity (initials avatar — single glyph), dependency readiness (`d:3/4`), and a drift overlay when present. The label may truncate; nothing else may.

Card chrome:

- corner radius: 10px
- border: 1px `status.border`
- background: `status.bg`
- text: `status.text`
- drift overlay (purple `git-compare` glyph) at top-right, 12px, white halo
- blocking-dep dot (red 6px circle) at bottom-left when `depsBlocking > 0` — visible at every tier; carries the "this is why nothing's moving" signal
- owner avatar: 18px round chip with two-letter initials in mono, background `gray3`, foreground `gray11`. Hover reveals the full owner name in the tooltip.

### Pipeline Stage Strip — Top Mini-Pipeline (P3)

A horizontal strip above the swimlanes preserves the factory metaphor and doubles as the status filter.

```
[ Intent 2 ] → [ Plan 1 ] → [ Task 4 ] → [ Implement 7 ] → [ Change 3 ] → [ Verify 5 ] → [ Done 11 ]
```

- Seven tiles, one per pipeline stage.
- Each tile shows stage name + live count.
- Active wave indicator: a subtle underline beneath the stage(s) currently containing in-progress units.
- Click a tile: filters swimlanes to units whose derived status maps to that stage.
- Hover a tile: highlights matching nodes in the graph.

Stage → status mapping:

| stage | matches statuses |
|---|---|
| Intent | planned (kind=plan/doc) |
| Plan | planned, ready (kind=plan) |
| Task | ready |
| Implement | running (kind=impl/refactor) |
| Change | running (kind=test) |
| Verify | verifying |
| Done | complete |

`blocked`, `failed`, `waiting`, and `drift` are NOT swept under tile membership. They surface two ways:

1. **Numeric overlays** on the affected stage tiles — small red/amber badges in the corner counting `failed`/`blocked`/`waiting` units that *would have been* in that stage. Example: the `Implement` tile shows `7` running (primary count) plus a red `2` overlay if two implement-stage units are failed.
2. **Explicit facet chips** to the right of the seven stage tiles, in their own visually separated group: `[ failed N ] [ blocked N ] [ waiting N ] [ drift N ]`. These chips behave identically to stage tiles (click = filter, hover = highlight) and use status-palette backgrounds. They are always visible — never hidden when their count is zero, but render muted when zero. Operators isolating "everything that's broken" click `failed` and `blocked` and the swimlanes filter accordingly.

Stage tiles compose with facet chips via the same AND-across / OR-within rule from the filter toolbar: clicking `Implement` and `failed` shows units in the implement stage that are also failed.

## Panels

### Page Grid — 12-Column With Collapsible Drawer (C3)

```
┌────────────────────────────────────────────────────────────────────┐
│ Summary status bar (12)                                            │
├────────────────────────────────────────────────────────────────────┤
│ Pipeline strip (12)                                                │
├────────────────────────────────────────┬───────────────────────────┤
│ Swimlanes (8 default · 12 if drawer    │ Active task drawer (4)    │
│ collapsed)                             │ collapsible to icon rail  │
│                                        │                           │
├────────────────────────┬───────────────┴───────────────────────────┤
│ Drift sections (6)     │ Verification panel (6)                    │
├────────────────────────┴───────────────────────────────────────────┤
│ Heartbeat strip footer (12)                                        │
└────────────────────────────────────────────────────────────────────┘
```

The drawer is the default surface because the active task answers four of the six glance-questions. Three drawer states exist; the user's choice is persisted across sessions in `localStorage` under `dashboard.drawerState`:

- **expanded** (default for first-time users) — drawer at full 4-column width, swimlanes at 8 columns.
- **collapsed-rail** — drawer becomes a 40px icon rail showing only the focused unit's status icon, owner avatar, and a count badge for the active set; swimlanes expand to 12 columns. Saves graph width for orthogonal-edge troubleshooting.
- **peek** — temporary, mouse-driven. When the drawer is in `collapsed-rail`, hovering the rail or clicking any node expands the drawer over the swimlanes as a 4-column overlay (no layout reflow). Releasing focus or pressing `Escape` collapses back to the rail. Peek does not change the persisted state.

Operators in graph-heavy troubleshooting collapse to the rail once and the dashboard stays out of their way. Operators with a single hot task keep it expanded. Switching states is keyboard-bound (`Ctrl+\` toggles between expanded and rail) and surfaces a 1px caret affordance at the drawer's left edge.

### Summary Status Bar (top row)

A single horizontal row, no wrapping. Fields left to right:

`plan name @ v{n}` · `wave N / total` · `progress %` · `active count` · `focused task ID + label` · `focused agent` · `health pill` · `last update timestamp`

When the active set has more than one unit, `active count` reads `3 active`; when one, it reads `1 active`; when zero, it reads `idle`. The focused task fields show the unit currently selected in the drawer; when `idle`, they collapse.

Health pill — multi-flag rendering with strict priority

The health pill is not a single token; it is a row of up to four chips rendered in priority order. The first chip (the "primary") is the most severe condition currently active and uses the largest type. Additional chips follow as smaller secondary indicators so a `failed + drift` situation surfaces both signals instead of collapsing into one.

Priority matrix (top = highest):

| level | label | tone | trigger |
|---|---|---|---|
| 1 | `Stale` | red, blinking | heartbeat > 15s OR truth-degraded |
| 2 | `Failed N` | red | one or more units derived `failed` |
| 3 | `Blocked N` | red | one or more units derived `blocked` |
| 4 | `Plan Mismatch` | amber | `state.plan_ref` does not match the loaded plan |
| 5 | `Drift N` | purple | drift count > 0 |
| 6 | `Waiting N` | amber | active set is `waiting`-only or partial deps satisfied; no higher condition |
| 7 | `OK` | green | none of the above and at least one unit progressing |
| 8 | `Idle` | gray | none of the above and active set empty |

Rendering rules:

1. The primary chip is the highest-priority condition currently true. It uses tone color and the count.
2. Up to three additional condition chips render, in priority order, after the primary, at 80% size and 80% opacity. They each show their level's label + count.
3. `OK` and `Idle` are mutually exclusive with everything else and never combine.
4. `Stale` always wins primary; lower-level chips still render so the operator sees that "stale" is hiding what was already failing.
5. Click any chip → filter swimlanes to the units behind that condition.

### Active Task Drawer — Stratified (D4)

Three vertical strata. The middle stratum scrolls; the others are fixed.

```
┌─────────────────────────────────────┐
│ Header (fixed)                      │
│   T1.2  alice  ⟳ running            │
│   [ Mark verified ]  ← next action  │
├─────────────────────────────────────┤
│ Middle (scroll)                     │
│   target files     mini-table       │
│   required inputs  mini-table       │
│   expected outputs mini-table       │
│   current artifacts mini-table      │
├─────────────────────────────────────┤
│ Blockers (pinned, always visible)   │
│   ⛔ deps unmet: T1.1                │
└─────────────────────────────────────┘
```

- Header: task ID, owner pill, status icon + label, primary `Next Action` button (state-driven label such as `Mark verified`, `Unblock`, `Reassign`).
- Middle: labeled mini-tables. No prose paragraphs. Truncate long values; full text on hover.
- Blockers row: pinned bottom; renders an empty muted "no blockers" pill when none, so the slot's silence is explicit.

#### Next Action Safety Gates

The Next Action button is the single click that mutates state. It must not encourage premature action when the data backing the recommendation is suspect.

Gating rules — the button enters one of four states based on freshness and integrity:

| condition | button state | visual |
|---|---|---|
| view fresh (≤ 5s since last good frame) and `planRefMatches` and unit's `lastUpdatedAt` ≤ 30s old | **armed** | full color, full opacity, hover lift |
| view fresh but unit's `lastUpdatedAt` > 30s old | **soft-warn** | full color · subtle amber underline · pre-click confirmation toast `Acting on data 47s old. Continue?` (single-click confirm) |
| heartbeat stale (> 5s) OR `planRefMismatch` | **suspect** | desaturated · amber border · pre-click confirmation toast spelling out the integrity condition |
| `truth-degraded` OR `Stale` (> 15s) | **locked** | 50% opacity · cursor `not-allowed` · click is a no-op; tooltip reads `state integrity unverified — refresh required` |

The button label always indicates what the action will do (`Mark verified`), never what it might do (`Continue`). When in `soft-warn` or `suspect`, the button additionally shows the source-of-truth timestamp underneath in mono: `state @ 11:30:42 (47s)`. Operators see exactly what the system is acting on.

The state of the button is computed on every poll; it never lags the underlying conditions.

### Drift Sections — Bucketed (DR3)

Six collapsible buckets, each with a count badge and a chip-row body.

| bucket | meaning | requires |
|---|---|---|
| missing | unit in plan, no record in state and not derivable as `planned` due to mismatched `plan_ref` | frame contract |
| extra | unit in state with no plan entry and no `replaces` mapping covering it | `replaces` policy |
| owner | `taken_by` (or last `reviewer`) differs from `units.<id>.owner`; suppressed silently when plan `owner` is absent | plan `owner` field |
| status | derived status incompatible with planned position (e.g. `complete` for a unit whose deps are still planned) | none |
| deps | satisfied deps set differs from declared `depends_on` | none |
| artifacts | `actual_artifacts` paths differ from `expected_artifacts` (missing, extra, or mismatched `kind`) | both artifact arrays |

Empty buckets render as `0 missing` muted; silence is signal.

Chip format: `T1.2 owner: alice→bob` (mono ID + field + plan→actual diff).

Chip click action — context-preserving navigation:

1. Hover-preview (no click): the matching node renders a 1px purple ring; no scroll, no drawer change. The operator sees where the chip points before committing.
2. Click: opens the unit in the active task drawer AND pulses the node for 1.5s. Scrolling the swimlanes is **deferred** — the dashboard does not move the viewport.
3. `Shift+click` OR a second click within 800ms: scrolls the swimlanes to center the node. The two-step protocol means the first click is "tell me about it" and the second is "take me there."
4. After any scroll-jump, a **back affordance** appears at the top-left of the swimlane viewport for 8 seconds: a small mono chip `← T2.3 (drift)` that returns the viewport to its prior scroll position and prior selection. Operators in incident response do not lose their place.
5. Selection persists. The previously focused drawer unit lands in the active-set chip strip, one click away.

Scroll-cancellation rules from Scalability Targets apply: user wheel/touch within 200ms of any auto-scroll cancels the auto-scroll.

### Verification Panel — Aggregate Plus Tiles (V3)

Header: `28/35 checks · 80%` with a colored bar reflecting the worst non-green tile.

Six tiles in a grid: `tests` · `lint` · `typecheck` · `build` · `review` · `acceptance`.

Tile anatomy:

- name (top)
- count `passed/total` (or `— / —` if no data)
- thin colored left bar (red on failure — never solid red fill; gray on `no data`)
- last-run timestamp `0:42` (or `never` if no data)
- source label (e.g. `ci:run/12345`, `local`) on hover
- click: expands inline to a mini-list of failing items pulled from `verification.<check>.failed_items`, no navigation

A tile shows `no data` only when the `verification.<check>` block is absent. The dashboard never synthesizes pass/fail counts.

Tile order is fixed: `tests · lint · typecheck · build · review · acceptance` — left to right, every render, every poll. Operators build muscle memory for "the third tile is typecheck"; reordering on severity destroys that memory at the moment they need it most.

Severity is encoded visually on each tile, not by position:

- failed: red left bar (4px), red glyph in corner (`alert-triangle`), bold count
- waiting (running with incomplete results): amber left bar, `clock` glyph
- no data: gray left bar, `circle` glyph, label `no data`
- green: green left bar, no corner glyph

The aggregate header counts only checks that have data; `no data` tiles are excluded from the denominator and noted as `(2 unreported)`. A "needs attention" badge surfaces near the header when one or more tiles are red, in the form `2 failing`, with click-jump scrolling to the first red tile.

### Heartbeat Strip Footer — Tri-Zone (H4)

Single thin row, full width.

```
┌──────────────────┬─────────────────────────────────┬────────────────┐
│ ● 0:03 · ok      │ T1.2 → running by alice  0:04 … │ ▁▂▅▃▁▂  4/s    │
└──────────────────┴─────────────────────────────────┴────────────────┘
```

| zone | content |
|---|---|
| left | pulse dot + last-update + connection status |
| center | scrolling event ticker (5–10 visible, newest left) |
| right | sparkline (events/sec last 60s) + numeric rate |

Stale handling — the strip itself is the alarm:

- > 5s since last update: pulse dot turns amber.
- > 15s: pulse dot turns red, ticker freezes, swimlane nodes desaturate 10–15%.

No modal alerts. No transient toasts that vanish before the operator looks. Critical data-integrity conditions surface in a single, persistent **degraded-data strip** anchored directly above the pipeline strip — full width, 28px high, always visible while the condition is active.

Degraded-data strip rules:

1. Renders only when at least one of these is true: `truth degraded` (parse failure > 60s), `plan ref mismatch`, `state version regression`, `legacy timestamps detected`. Otherwise the strip is absent (zero height — no reserved gap).
2. Tone matches the worst active condition (purple for truth-degraded, amber for everything else).
3. Layout: left = severity icon + concise label (`Truth degraded · last good frame 2m 14s ago`); right = action buttons (`Reload`, `View raw`, `Acknowledge`).
4. Stays visible until the underlying condition clears or the operator clicks `Acknowledge`. Acknowledged conditions move to the heartbeat ticker as a single pinned chip and free the strip for the next condition.
5. The strip never overlaps the swimlane viewport — it adds height to the page chrome above. This is the one piece of vertical real estate the dashboard reserves for "you must read this."

This honors the no-modal rule (no overlay, no focus trap, no dismissal-required dialog) while preventing severe data integrity issues from being lost in subtle pulse-color shifts.

## Behavior

### Filter UX — Distributed (F4)

Status filter lives in the pipeline strip (already specified). Other filters live in a slim toolbar above the swimlanes:

```
[ wave ▾ ] [ owner ▾ ] [ ☐ drift only ] [ Default | Spotlight | Diff ]
```

Composition rule: AND across filter types, OR within a single type. Active filters echo as a removable chip row directly under the toolbar so the user always knows what is hidden.

#### Filter Legibility

Filter logic is precise but not self-explanatory. Operators commonly misread "owner=alice AND wave=3" as "either alice or wave 3" and report the dashboard as broken when the intersection is empty. The dashboard makes the math explicit.

Rules:

1. **Visible/hidden tally**: a small mono counter renders at the right edge of the active-filter chip row reading `12 of 47 visible · 35 hidden`. Click → opens an inline expansion explaining how each filter contributed to the hide count.
2. **Empty-result helper**: when the visible count drops to zero, the swimlanes render an inline empty card with the literal sentence `No tasks match all of: {list of active filters joined by AND}`. Each filter name in the sentence is a click target that removes that single filter — operators can debug their own intent by peeling filters one at a time.
3. **AND/OR visual cue**: chips within the same filter type render inside a shared rounded container with subtle `+` separators between them (visual OR). Different-type chips sit side-by-side with no shared container. The chip row itself reads naturally as `(wave 2 + wave 3) AND (alice + bob) AND drift-only`.
4. **No silent zero**: the dashboard never renders an empty graph without the helper card; "looking blank" must always have a written explanation present.

### Modes

- **Default** — full graph plus drawer.
- **Spotlight** — focused task centered, only ancestors and descendants emphasized; other nodes muted; full lineage edges rendered at full color.
- **Diff** — only drifted nodes and the edges adjacent to them rendered at full color; non-drifted units muted as orientation context.

#### Mode and Filter Precedence

Modes shape the visual emphasis. Filters shape the membership. They compose; they do not override each other.

Order of operations on every render:

1. The filter set determines which units are *included* in the rendered graph (filtered-out units are still drawn but at a distinct visual treatment, see below).
2. The mode determines which of those included units are *emphasized*.
3. The selection determines which single unit is *focused* (drawer + N3 pin).

Three distinct visual treatments — never collapsed:

| treatment | meaning | appearance |
|---|---|---|
| filtered-out | excluded by user filter | 30% opacity · dashed `2 2` border · no status color · gray glyph · NOT clickable for drawer focus |
| mode-muted | inside filter, outside mode emphasis (Spotlight non-lineage, Diff non-drifted) | 50% opacity · solid border kept · status color kept · clickable |
| emphasized | inside both | 100% opacity · full chrome |

Operators cannot mistake "filtered out" for "deleted" because filtered-out cards are visibly different in border style (dashed) and stay clickable to remove the filter that hid them. A keyboard tooltip on filtered-out cards reads `hidden by filter: {list}`.

The mode switch is announced in the toolbar via a 200ms color swipe across the visible swimlane area when toggled — visceral confirmation that the visual change came from the user, not from the data.

### Interaction States

| state | treatment |
|---|---|
| hover | node lift + soft shadow · tooltip after 400ms · NO lineage brightening, NO global dim — hover is informational only |
| pinned-hover (sustained 600ms OR `Shift`+hover) | adds lineage edge brightening + 1px ancestor/descendant ring · others remain undimmed (lineage is additive emphasis, not subtractive) |
| selected | persistent 2px accent ring · drawer opens · lineage highlight stays until deselect |
| recent-update (≤ 10s) | soft outer glow in status color · ease-out fade over 10s |
| loading | skeleton placeholders (gray3 blocks) for nodes and tiles · never blank |
| empty (no plan) | minimal empty state: `Drop plan.json` action + `Open file…` button (no illustration) |
| empty (plan ok, state missing) | swimlanes render planned-only in gray + inline banner `No state file — showing plan only` |
| stale (>5s) | pulse → amber |
| stale (>15s) | pulse → red · ticker freezes · nodes desaturate 10–15% |
| error (parse fail, ≤ 60s) | toast top-right + offending field path · dashboard pins last-good `ComparedView` · heartbeat dot turns amber |
| truth degraded (parse fail > 60s) | full-width purple banner `truth degraded — last good frame {age}` · swimlane nodes desaturate 25% · drawer locks (no actions) · drift and verification panels render muted with `stale` badges · only resolution is a fresh successful parse |

## Behavioral Integrity Contract

The dashboard mutates visual state from many independent sources: polling frames, mode toggles, filter changes, selections, hovers, drift-chip clicks, scroll-jumps, animations. Without a single contract, these can race and produce ghost selections, conflicting visibility, mixed-time displays, and pulses landing on vanished nodes. The following rules are normative.

### Frame-Bound Transforms

All visual transforms (selection ring, hover overlay, mode dim layer, recent-update glow, pulse highlight) are bound to a specific `ComparedView` frame and identified by stable unit IDs.

Rules:

1. Each `ComparedView` carries a unique `frameId` (`view.frameId`) — a monotonic counter incremented per accepted frame.
2. Transforms attach to `(unitId, frameId)` tuples, not DOM node references. When a new frame is published, transforms re-resolve to the same `unitId` against the new layout.
3. If a `unitId` is absent in the new frame (e.g. removed mid-session), its transforms drop silently. They never linger as ghosts on neighboring cards.
4. Mode toggles compose with frames: a `Spotlight` enabled at frame N continues to apply at frame N+1 without re-fetching the lineage; the lineage set is recomputed lazily when its inputs (focused unit, edges) change.
5. Rapid mode toggling (e.g. Default → Spotlight → Diff → Default within 200ms) coalesces. Only the final mode is committed at the end of the burst (300ms debounce). Intermediate states are not painted.

### Visibility Precedence Contract

Three layers determine whether a unit is visible: filters, modes, selection. They compose in fixed order on every render — no exceptions.

```
1. Filter membership      → set FILTERED of unit IDs that pass all active filters
2. Mode emphasis          → set EMPHASIZED ⊆ FILTERED
3. Selection focus        → set FOCUSED ⊆ EMPHASIZED ∪ FILTERED, |FOCUSED| ≤ 1
```

Resolution per unit:

| in FILTERED? | in EMPHASIZED? | in FOCUSED? | render |
|---|---|---|---|
| no | n/a | no | filtered-out (dashed border, 30%, gray glyph) |
| yes | no | no | mode-muted (50%, solid border, status color) |
| yes | yes | no | full (100%) |
| yes | yes | yes | N3 active pin |
| no | n/a | yes | filtered-out + N3 active pin (warning chip in drawer: "focused unit is filtered out") |

The last row is the corner case the contract handles cleanly: if the operator pins focus on a unit and then applies a filter that excludes it, the unit stays focused and the drawer surfaces a warning chip explaining why no card is highlighted in the swimlanes — never a silent disappearance.

The precedence is computed in a single derived selector (`useDashboard.selectVisibility()`) so all components agree on the same answer.

### Selection Persistence Policy

The selected unit ID is stored in the Zustand store and is the source of truth for the drawer + N3 pin. Persistence rules across all transitions:

| event | selection behavior |
|---|---|
| user clicks a different node | selection moves |
| user clears selection (Esc) | selection becomes null |
| filter applied that excludes the selected unit | selection PERSISTS; drawer renders the unit; warning chip surfaces in drawer |
| selected unit's wave changes (plan revision) | selection PERSISTS; the card animates to its new lane (200ms); selection ring follows the move |
| selected unit removed from plan | selection PERSISTS only if the unit still exists in state (drawer shows `unit no longer in plan` banner from §6); if removed from both, selection becomes null and the drawer shows the previous focused unit's last-good snapshot for 5 seconds with a banner `unit T1.2 vanished from plan and state`, then clears |
| Spotlight mode enabled with no selection | selection auto-targets the unit at health-priority position 1 (most urgent); operator can override by clicking |
| Diff mode enabled with no selection | no auto-selection; the drawer shows "no unit selected · click a drift chip" |
| polling publishes a new frame | selection PERSISTS by unit ID across frame transitions |

A unit ID is the durable handle. Selection never silently jumps to a different unit because of layout changes.

### Degradation Signal Arbitration

Multiple degradation signals can fire simultaneously: heartbeat stale, parse failure, partial data, plan-ref mismatch, silent stall. The dashboard arbitrates rather than rendering all at once.

Priority ladder — only the **primary** signal renders the dominant treatment; lower signals render as secondary chips in the heartbeat strip.

| level | signal | dominant treatment |
|---|---|---|
| 1 | truth degraded (parse fail > 60s) | persistent purple degraded-data strip; 25% desat; drawer locks |
| 2 | heartbeat stale > 15s | red pulse + ticker freeze + diagonal hatch overlay |
| 3 | plan-ref mismatch | persistent amber degraded-data strip; banner |
| 4 | silent stall (no progress > 30s with running units) | amber chip in heartbeat strip; health pill adds `Stalled` |
| 5 | parse fail ≤ 60s (transient) | toast + heartbeat dot amber; no overlay |
| 6 | heartbeat stale > 5s | amber pulse only |
| 7 | partial data (drift confidence ≠ high) | striped chips in drift panel; partialDriftCount counter |
| 8 | legacy timestamps detected | single chip in heartbeat strip |

Composition rules:

1. The dominant level's full treatment renders.
2. All other simultaneously-active levels render their **chip-only** form in the heartbeat strip's left zone (right of the pulse dot, before the ticker).
3. Treatments never stack visually — at most one diagonal hatch, one degraded-data strip, one banner. Conflicts resolve by ladder priority.
4. The summary bar's health pill priority matrix already accounts for these (`Stale > Failed > Blocked > PlanMismatch > Stalled > Drift > Waiting > OK > Idle`), so the pill remains coherent.

### Pinned-Frame Heartbeat Coherence

When a `ComparedView` is pinned (parse-fail freeze, plan-ref mismatch, etc.), the heartbeat strip continues to live-update its age counters. This produces a UI where the graph is frozen but the footer ticks.

To prevent the appearance of "live data sitting on stale graph":

1. The heartbeat strip in pinned mode renders its left zone with a `pinned` label and the pinned frame's `acceptedAt` time: `pinned · 0:42 ago · v{n}`. The age counter ticks; the version does not.
2. The ticker stops appending new events while pinned. The most recent events from the pinned frame remain visible in static form, with timestamps relative to the pinned frame, not wall clock.
3. The sparkline freezes to a flat line at zero events/sec. The right-zone numeric rate reads `0/s · paused`.
4. Wall-clock time is shown only in the degraded-data strip, never on the graph or panels — the operator's only "now" is in the strip itself.

This separates "we know the time" from "we know the data," which is the only honest representation of a degraded session.

### Pulse Targeting

Pulse highlights from drift-chip clicks, recent-update glows, and `Shift+click` scroll-jumps target unit IDs. Rules to handle vanished or repositioned targets:

1. Pulse animations bind to `data-unit-id` selectors, not direct DOM refs.
2. If the target node is not in the DOM (filtered out, layout reflow in flight, vanished from frame) at the moment the pulse should start, the pulse is **deferred** for up to 600ms waiting for the node to appear.
3. After 600ms with no DOM node, the pulse is canceled silently and a chip surfaces in the heartbeat ticker for 4s: `pulse target T2.3 not visible (filtered or removed)`. Operators see why the click "didn't do anything."
4. Auto-scroll to a unit follows the same rule: if the target's frozen rect is no longer present (unit removed), the scroll cancels and the same chip surfaces.
5. Concurrent pulses on the same unit do not stack; the most recent pulse replaces any in-flight pulse on that unit.

## Visual Stress Test Resolutions

The visual system must hold up under grayscale, colorblindness, glare, zoom-out, and high edge density. The following adjustments apply to the previously-stated tokens and glyphs.

### Running vs Verifying — Stronger Distinction

Both states are blue and both glyphs read as "circular busy thing" at 16px. Distinction strengthened on three independent axes:

| state | hue | glyph | motion | top accent |
|---|---|---|---|---|
| running | blue (Radix `blue`) | `loader-2` (rotational spinner, no fill) | continuous spin (1.2s) | 2px solid blue top bar |
| verifying | indigo (Radix `indigo`, distinct family from blue) | `circle-dashed-check` filled with a 50% inner check mark | static (no motion) | 2px **dashed** indigo top bar |

The hue separation is between `blue-9` (#0091FF in light mode) and `indigo-9` (#3E63DD); they are visually distinct under both colorblind simulators (deuteranopia, protanopia) and at 50% saturation. Indigo is added to the token system:

```css
--status-verifying-bg:     var(--indigo-3);
--status-verifying-border: var(--indigo-6);
--status-verifying-solid:  var(--indigo-9);
--status-verifying-text:   var(--indigo-11);
```

The motion vs no-motion distinction means even at fully grayscale, glance-time differentiation is `spinning blob = running`, `static check-in-circle = verifying`.

### Low-Contrast Risk on Compact Cards

`gray3 / gray6 / 13px` body text falls below WCAG AA contrast under glare on cheap monitors. Adjustments:

1. Card text on N1/N2/N3 uses `gray12` (the highest-contrast Radix token) for the unit ID and label, not `gray11`. The 13px size pairs with 4.5:1 minimum contrast at all status backgrounds.
2. The owner avatar's two-letter initials use 600-weight mono at 11px on a `gray3` background — verified WCAG AA against the worst case (gray-on-amber).
3. Counts in mono (`d:3/4`, `5/7v`) are 12px / 500-weight / `gray12` on the card to ensure tabular numbers read under glare.
4. The page minimum contrast ratio is targeted at AAA (7:1) for primary content (unit IDs, owner names, blocker labels) and AA (4.5:1) for everything else; the verification panel runs an automated contrast check on its own tokens at build time and fails the build if any combination drops below AA.

### Colorblindness — Shape and Pattern Redundancy

Color is one channel; the dashboard's accessibility commitment is that every status is distinguishable when color is removed. Reinforced beyond the §3 grayscale section:

| status | hue | glyph silhouette | border pattern | accent strip |
|---|---|---|---|---|
| planned | gray | empty ring (`circle`) | solid 1px | none |
| ready | gray | dashed ring (`circle-dashed`) | solid 1px | none |
| running | blue | spinning loader | solid 1px | solid top bar (2px) |
| verifying | indigo | dashed-check (static) | solid 1px | dashed top bar (2px) |
| waiting | amber | clock | solid 1px | none |
| blocked | red | lock | dashed 2px | none |
| failed | red | triangle (`alert-triangle`) | dashed 2px + double-stroke | none |
| complete | green | check-in-circle | solid 1px + 1px inner ring | none |
| skipped | gray | arrow-circle (faded) | solid 1px (50% opacity) | none |

The silhouette set is hand-picked so each is distinct at 16px without color: ring vs dashed-ring vs spinner vs check-in-circle vs clock vs lock vs triangle vs arrow. No two share a basic shape category (open shape vs filled vs angular). Tested with three deuteranopia simulator passes at 100%, 75%, and 50% display saturation — all states remain identifiable.

### Small-Icon Ambiguity at N1 Density

`circle-dashed`, `circle-dashed-check`, and `progress-check` are too similar at 16px. Resolved by:

1. **`ready` keeps `circle-dashed`** — pure ring, no infill.
2. **`verifying` switches to `shield-check` (Tabler) at 16px** — a chevron-shielded check, visually unambiguous at small size and matches the "validation in progress" semantic better than dashed-check. (Earlier draft used `circle-dashed-check`; this supersedes it.)
3. **`complete` keeps `circle-check`** — solid filled circle with a check, distinct from any dashed variant.

The status icon table in the Iconography section is updated accordingly. The implementation references the Tabler glyph set; if `shield-check` is unavailable in the installed version, fall back to `progress-check` with the static (non-spinning) variant — but this is the only allowed substitution.

### Drift Badge Visibility

A 12px purple overlay disappears in clutter and at zoom-out. Adjusted:

1. The drift overlay is 14px (was 12px) at N1 — large enough to read at 75% zoom.
2. The overlay sits in a 16px white halo (raised from 14px) so it survives against any status bg.
3. **Border-pattern reinforcement**: any drift-flagged card additionally gets a 2px purple **outer halo** at the card's bounding box (a `box-shadow: 0 0 0 2px var(--purple-9)` on the container). The badge tells you "yes, drift here," the halo tells you "yes, even at zoom-out, this card has drift."
4. At density beyond 200 visible cards, the overlay glyph is suppressed and only the halo renders — drift remains visible without the small glyph adding noise.

### Edge Density and Arrowhead Distinction

At 200+ visible edges, dashed vs solid blurs and chevron arrowheads become indistinct. Adjustments:

1. **Arrowheads scale with density**: above 100 visible edges, arrowheads grow from 6×8 to 8×10 pixels and switch from filled triangles to two-stroke open chevrons (more visually weighty per pixel).
2. **Dashed-pending de-emphasis at density**: above 100 visible edges, pending edges drop to 30% opacity. Satisfied/blocking edges stay full opacity so the truth-carrying lines remain prominent.
3. **Edge stroke width hierarchy**: pending = 1px, satisfied = 1.5px, blocking-active = 2px, blocking-failed = 2.5px. Width itself is a redundant urgency channel.
4. The per-wave bus fallback (Scalability Targets §3) is the ultimate density solution; it engages automatically before edge clutter reaches noise levels.

### Stale Desaturation vs State Differentiation

Desaturating swimlane nodes 10–15% during stale conditions risks erasing the very status colors operators rely on. Replaced with a different stale treatment:

1. **No global desaturation**. Stale nodes keep their full status colors — the operator must still see what is failed and what is complete.
2. Stale instead applies a **subtle diagonal hatch overlay** (CSS `repeating-linear-gradient(45deg, transparent 0 6px, rgba(0,0,0,0.05) 6px 7px)`) across the swimlane viewport. Like fog. Status colors show through, but a uniform "this is not live" texture is visible on the entire graph at once.
3. The heartbeat strip's pulse + ticker-freeze remain the primary stale signals; the hatch is a glanceable supplement.
4. Truth-degraded (>60s parse fail) keeps the 25% desaturation because by then the state is *suspect*, not just *late*; making the colors questionable is the correct signal.

The hatch is opt-out via `prefers-reduced-motion: reduce`-adjacent preferences (a separate `prefers-reduced-transparency` check is honored when supported).

## Edge Case Handling

The following conditions occur in real plans and state files. The dashboard handles each explicitly; nothing is allowed to crash or silently degrade.

### Plan-side malformations

#### Cyclic dependencies

The plan declares a dependency cycle (e.g. `T1.1 → T2.1 → T1.1`). The compare layer detects cycles via Tarjan's SCC pass during `loadPlan` and renders affected units as follows:

1. Each unit in the cycle gets `derivedStatus = 'planned'` (the layer cannot compute readiness through a cycle).
2. Each unit accumulates a `cyclic` warning in `UnitView.warnings`.
3. The drift panel surfaces a top-level **plan integrity** banner above the buckets: `cyclic dependencies detected: T1.1 ↔ T2.1 ↔ T3.1` listing every cycle. Clicking the banner highlights the cycle's edges in red across the swimlanes.
4. Edges participating in a cycle render with double-stroke red regardless of source/target status.

The dashboard never blocks rendering on cyclic plans; it surfaces them and continues.

#### Self-dependency

A unit declares itself in `depends_on` (`T1.2 → T1.2`). The compare layer:

1. Strips the self-edge from the dependency list before any derivation.
2. Adds `selfDependency` to `UnitView.warnings`.
3. Surfaces under the **plan integrity** banner alongside cycles.

The unit derives status normally with the self-dep ignored.

#### Unit referenced in `waves[]` but missing from `units` map (or inverse)

If `tasks.<id>.wave` references a wave but no `units` entries belong to that wave, or a unit's `task` field references a missing parent task, the compare layer:

1. Logs a `schemaInconsistency` warning at the top-level `ComparedView.warnings` array.
2. Renders only the half that exists: orphan units render in their declared wave; orphan tasks (no units) do not produce a swimlane.
3. The plan integrity banner surfaces these as `schema mismatch: 2 orphan units, 1 task with no units`.

#### Plan regenerated mid-run with wave renumbering

A new poll's plan has units that previously sat in wave 3 now sitting in wave 5. Without an explicit `replaces` map, this presents as wholesale `missing` + `extra` drift.

Handling:

1. The frame contract's `plan_version` increment triggers a snapshot of the prior plan's wave assignments; the dashboard caches the last accepted plan in `lastGoodPlan`.
2. A header banner renders for 30s after the version advance: `plan revised at v{n} — drift may reflect renumbering`. Operators see why drift spiked.
3. Layout-stability frozen rects survive the version advance for any unit ID that exists in both versions; a card whose wave changed slides over 200ms to its new lane (the only animated reflow allowed). Operators visually track the move.
4. Per the Plan Revision Lifecycle, plan authors should declare `replaces` mappings to keep this clean; the dashboard tolerates their absence with the banner.

### State-side conditions

#### Task deleted from plan but still present in state

A unit is in `state.taken` or `state.completed` but its plan id is gone (no `replaces` alias either). Handling:

1. The unit appears in the drift `extra` bucket with `confidence: high`.
2. It does NOT render in any swimlane (no plan unit means no layout slot).
3. The drawer is reachable via the drift chip; the drawer renders the unit with a banner: `unit no longer in plan — state retained for audit`.
4. The summary bar's `extraTasks` count includes it.

This is the only path where state content is visible without a swimlane card.

#### Unit moved to another wave after being taken

A `taken` unit's plan record now declares `wave: 5` where it previously declared `wave: 3`. Handling:

1. The unit honors its **new** wave (plan is authoritative for layout).
2. The card animates over 200ms from the old wave's row to the new wave's row (the only animation that violates layout-stability — and it is gated on plan-version advance, not poll).
3. A `waveMoved` warning attaches to the unit; the drawer's middle stratum shows `wave: 5 (was 3 in v{n})`.
4. The card pulses in its new lane for 1.5s post-animation so operators see where it landed.

#### Repeated review-enter / review-exit flapping

A unit's `review_entered_at` and `review_ended_at` flip multiple times within a 5-second window. This is usually a writer bug or a checkpointing race.

Handling — debounce with hysteresis:

1. The unit's derived status reflects the **most stable** reading over the last 5 seconds: if `verifying` and `running` alternated, the status holds at whichever value persisted ≥ 60% of the window.
2. The unit accumulates a `reviewFlap` warning. The heartbeat ticker surfaces a chip `T1.2 review flap (4 toggles in 5s)` until quiescent for 10s.
3. The drawer's middle stratum shows the flap counter and the last 4 transitions with timestamps.

The unit does not change status icon every poll; muscle memory is preserved.

### Read-side conditions

#### State file partially written or truncated during poll

`fetch(state.json)` returns 200 but the body is incomplete JSON (a writer atomically replaced the file mid-read on a non-atomic filesystem, or the writer wrote in chunks).

Handling:

1. JSON parse fails. The poll loop catches the error, increments `parseFailureCount`, and retries the next poll without publishing a new `ComparedView`.
2. Three consecutive parse failures within 6 seconds bumps the heartbeat dot to amber and surfaces a chip `state file unstable — n/3 parses failed` in the heartbeat ticker.
3. After 60s of sustained parse failures (truth-degraded threshold), the persistent degraded-data strip activates with the reason `state.json — parse failure for 60s`.
4. A single isolated parse failure during otherwise healthy polling does NOT trigger any UI change beyond a debug log entry.

#### Heartbeat alive but state unchanged (silent orchestrator stall)

The poll loop receives valid responses every 2s, the JSON parses cleanly, but `state_version` never advances and no unit's `lastUpdatedAt` changes. The orchestrator is stuck without crashing.

Handling:

1. The dashboard tracks `framesSinceProgress` — the count of consecutive accepted frames where neither `state_version`, the set of `taken` keys, the set of `completed` keys, nor any `verification.*.ran_at` advanced.
2. After 30s (15 frames at 2s polling) of no progress with at least one running unit present, the heartbeat strip surfaces an amber chip: `no progress 0:30 · {N} units running`. Click → highlights the running units.
3. After 120s, the chip turns red and the summary health pill adds a `Stalled` chip below `OK` in priority position 6.5 (between PlanMismatch and Drift): the operator sees `OK · Stalled · Drift 1` instead of just `OK`.
4. If no units are running (all complete or all planned), no stall warning fires; silence is correct.

### Display-time conditions

#### `totalTasks = 0`

An empty plan, or a plan with only orphan units, produces `totalTasks = 0`. All ratio metrics guard against division:

1. `progressPercent`: when denominator is 0, renders `—%` (em-dash followed by percent), not `NaN%` or `100%`.
2. `dependencyReadiness`: when denominator is 0, renders `1.0` (everything done — vacuously true) per the Derived Metrics table.
3. `verificationPassRate`: when denominator is 0, renders `—` (no aggregate possible).
4. The summary bar shows `Idle` health pill and the swimlanes render an empty-state card: `No units in plan — open a plan with at least one unit to begin`.

#### Long labels and paths overflowing cards

Real plan unit titles can run 80+ characters; artifact paths can run 200+. Handling:

1. **N1 label**: 1 line, truncate with ellipsis at the card width (~120px effective text area). Title attribute carries the full label; tooltip after 400ms renders it.
2. **N2 label**: 1 line, truncate at ~150px text area. N2 owner pill, deps, and verify counts have fixed positions; the label cannot bleed into them — overflow is hidden, not wrapped.
3. **N3 label**: 2 lines max with ellipsis on the second line.
4. **Owner pill**: max 1 line, truncate at 12 characters of the display name; full owner shown on hover.
5. **Drawer file paths**: middle stratum mini-tables truncate with leading-ellipsis (`…rc/components/very/long/path.tsx`) — the file name is more useful than the prefix. Full path on hover.
6. **No collision with badges**: drift overlay (top-right), blocking-dep dot (bottom-left), and owner avatar are absolutely positioned with reserved padding zones in the card grid; the label can never overlap these.
7. **CJK and RTL**: labels render with `unicode-bidi: plaintext` so RTL text displays correctly within the LTR card chrome.

## Confidence Model for Partial Data

Drift and metrics are not boolean assertions; they are claims with a confidence level derived from how much of the underlying data is present. The dashboard renders confidence visibly so an operator can tell "no drift detected" from "drift undetectable."

Confidence per unit per drift bucket:

| confidence | trigger |
|---|---|
| `high` | both plan-side and state-side fields required for that bucket are present |
| `partial` | one side present, the other absent (e.g. plan declares `expected_artifacts` but state record has no `actual_artifacts` block at all) |
| `unavailable` | required fields absent on both sides; the bucket is structurally indeterminate |

Rendering rules:

1. `high` entries render with full chip color and contribute to the bucket count.
2. `partial` entries render with a striped chip background (CSS diagonal stripes in the bucket's tone) and contribute to a parenthesized side-count: `extra: 3 (+2 partial)`.
3. `unavailable` does not render chips; the bucket header reads `0 owner (n/a — no plan owner declared)` so the operator sees that the silence is from absence, not from agreement.
4. `driftCount` excludes `partial` entries; the summary's drift health pill reads accurately for high-confidence drift only. A separate `partialDriftCount` surfaces in the drift panel header.

The compare layer attaches `confidence: 'high' | 'partial' | 'unavailable'` to every `DriftEntry` it produces. UI never renders an entry without inspecting it.

## Status Race Stability

Status flicker under polling races is a UI failure even when the underlying logic is correct. The compare layer adds two stabilization rules.

### Terminal-State Hysteresis

Once a unit derives `complete`, `failed`, or `skipped` (terminal states), the dashboard requires two consecutive non-terminal frames to revert it. A single transient `running` reading sandwiched between two `complete` readings does not unflag the unit. This handles the race where a writer briefly re-stages a `taken` entry for a unit that was already in `completed`.

The hysteresis is bounded: after 30s of sustained non-terminal status the unit reverts unconditionally. Stuck terminal states cannot persist indefinitely.

### Frame Sequence Acceptance

Each `ComparedView` carries an `acceptedAt: number` (millisecond epoch from the dashboard clock) and the underlying `state_version`. The publish-or-discard rule:

1. If `state_version` is present and ≤ the last accepted frame's `state_version`, discard. The previous frame stays on screen.
2. If `state_version` is absent, fall back to monotonic `acceptedAt` ordering — but raise a warning chip in the heartbeat strip: `state version missing — frame ordering unverified`.
3. If `state_version` jumps backward more than 1 (e.g. 5 → 2), assume a fresh state file replaced an older one (rollback or restore); accept the new value as a new baseline and log a warning chip.

The poll loop never overwrites a newer-versioned view with an older one, even when the older arrives later (out-of-order HTTP responses).

## Dependency Drift — Two Distinct Axes

The single "deps" drift bucket conflates two separable concerns. The dashboard separates them.

| sub-bucket | meaning | surfaces under |
|---|---|---|
| `deps-topology` | the plan's declared `depends_on` set differs from a prior plan revision's set for the same unit ID | only when `plan_version` advances and the prior plan is in cache or `replaces` mappings reveal it |
| `deps-runtime` | satisfaction state diverges from declared topology — e.g. the plan declares `T1.2` depends on `T1.1`, but state shows `T1.2` running while `T1.1` is `failed` | every poll |

The drift panel's `deps` bucket is split into two sub-rows under one header. Most operational drift is `deps-runtime`; `deps-topology` only matters during plan revisions and surfaces only then. The compare layer emits separate entries; the UI groups them under one collapsible parent for visual economy.

`deps-runtime` confidence is always `high` (both sides are present by construction). `deps-topology` confidence is `partial` when the prior plan is unavailable.

## Ownership Conflict Resolution

`taken_by` may differ between a unit's `taken` and `completed` entries (e.g. alice took it, bob completed it after a handoff). The dashboard does not silently pick one.

Rules:

1. The `actualOwner` field on `UnitView` is computed as: `completed.taken_by ?? taken.taken_by ?? null` for derived-status purposes (the most recent authoritative writer).
2. When `taken.taken_by !== completed.taken_by` (both present), the unit raises a `handoff` warning surfaced as:
   - the drawer's owner pill rendering as a two-tone split chip `alice → bob` with a small `git-merge` icon
   - a warning entry in `UnitView.warnings`: `ownerHandoff`
   - an additional drift-bucket entry under `owner` with `confidence: high` because both sides are explicitly recorded
3. The active-set chip strip shows the handoff visually too: a chip displays `bob (was alice)` with the prior owner muted.
4. Handoff is not failure; it's information. No status-color change. Operators see who currently owns the next action without losing the audit trail.

## Timestamp Authority

The dashboard treats timestamps as observations, not truth. To handle clock skew across writers it computes a derived ordering layer on top of raw timestamps.

Rules:

1. **Writer clock authority**: the most recent timestamp written by `state` (file mtime as fallback when no timestamp exists in the record) is the dashboard's clock reference for that frame. The dashboard never compares its own wall clock against a writer's timestamp for ordering.
2. **Skew tolerance**: when a unit's `started_at` is later than its `finished_at`, the dashboard logs a `clockSkew` warning on `UnitView.warnings` but renders the unit normally; the timeline displays "skewed" in the drawer's middle stratum.
3. **Future timestamps** (more than 60s ahead of the latest accepted frame's writer clock) trigger a `futureTimestamp` warning. The unit still renders.
4. **Monotonic event sequence (future)**: if and when state files include an `event_seq` integer per record, that field overrides timestamps for ordering and removes all clock dependence. The dashboard will pick up `event_seq` automatically when present; it is documented in the state schema as an optional field for the next revision.
5. **Heartbeat freshness** uses dashboard wall clock (`Date.now()`) measured against the moment a frame was accepted, not the timestamps inside that frame. Stale = "we haven't seen a new frame," not "the writer's clock looks old."

Operators see, on hover of any timestamp in the drawer: the raw value, the parsed ISO form, the writer-clock skew (`writer ahead by 12s`), and any associated warning.

## Schema and Frame Contract

The current waveplan plan and state schemas do not encode several facts the dashboard treats as primary signal. Rather than fabricate that signal, this design declares the schema fields it requires and the contract under which the dashboard reads them. Schema extensions are listed here so they can be planned and merged before or alongside the dashboard implementation.

### Required Plan Schema Extensions

| field | location | type | purpose |
|---|---|---|---|
| `owner` | `units.<id>` | string (optional) | Planned agent, enables `owner` drift detection |
| `expected_artifacts` | `units.<id>` | array of `{ path, kind }` | Enables `artifacts` drift detection |
| `acceptance` | `units.<id>` | array of `{ id, label }` | Drives the `acceptance` verification tile |
| `replaces` | `units.<id>` | array of prior unit IDs | Identity continuity across plan revisions |
| `plan_version` | root | integer, monotonic | Frame contract (see below) |
| `plan_generation` | root | string (uuid or timestamp) | Frame contract |

If `owner` is absent on a unit, the dashboard treats `plannedOwner` as `null` and suppresses the owner drift bucket entry for that unit (silence rather than false drift).

### Required State Schema Extensions

| field | location | type | purpose |
|---|---|---|---|
| `failed_at` | `taken.<id>` and `completed.<id>` | ISO 8601 string | Marks `failed` status terminally |
| `failed_reason` | same | string | Tooltip and drift detail |
| `blocked_reason` | `taken.<id>` | string | Distinguishes `blocked` from `waiting` |
| `skipped` | `completed.<id>` | boolean | Marks `skipped` status |
| `actual_artifacts` | `taken.<id>` and `completed.<id>` | array of `{ path, kind, written_at }` | Enables `artifacts` drift detection |
| `verification` | `taken.<id>` and `completed.<id>` | object keyed by check name | Authoritative verification source |
| `state_version` | root | integer, monotonic | Frame contract |
| `state_generation` | root | string | Frame contract |
| `plan_ref` | root | `{ plan_version, plan_generation }` | Pairs state to the plan it derived from |

`verification` block per check (one of `tests`, `lint`, `typecheck`, `build`, `review`, `acceptance`):

```
verification.<check> = {
  total: int,
  passed: int,
  failed_items: [ { id, label, message? } ],
  ran_at: iso8601,
  source: string   // e.g. "ci:run/12345" or "local"
}
```

The dashboard never synthesizes verification counts from heuristics. If the block is missing for a check, that tile renders muted with `— / —` and the label `no data`.

### Frame Contract

Plan and state files mutate independently and may be observed mid-write. To prevent mixed-frame truth, the dashboard treats every render as a paired snapshot.

1. The compare layer reads `plan_version` + `plan_generation` and `state.plan_ref` on every poll.
2. If `state.plan_ref` does not match the loaded plan, the state is considered out-of-frame. The dashboard renders the last in-frame `ComparedView` and shows a thin amber banner: `state pinned to plan v{n}, current plan is v{m}`.
3. A new `ComparedView` is published only when both files parse, both versions match, and `state_version` increased monotonically.
4. If `state_version` regresses (file replaced or rolled back), the dashboard discards its last-good frame and refreshes from the new baseline.

This contract is also why the heartbeat strip's "last update" reflects publication of a new `ComparedView`, not raw filesystem mtime.

### Timestamp Contract

The current state schema declares `YYYY-MM-DD HH:MM` timestamps without timezone or seconds. The dashboard requires:

- All new timestamp fields use ISO 8601 with timezone (`2026-04-28T14:30:42-07:00`).
- The compare layer accepts the legacy `YYYY-MM-DD HH:MM` format and parses it as local time with seconds set to `00`. A migration warning surfaces in the heartbeat strip if any timestamp uses the legacy form: `mixed timestamp formats — promote state file to ISO 8601`.
- Ordering is by ISO timestamp; the legacy form is treated as lower-resolution and breaks ties via stable unit-ID order.

A future revision should introduce a monotonic counter (`event_seq`) to remove all clock dependence; out of scope for v1.

### Plan Revision Lifecycle

When a plan is regenerated, units may be renamed, split, or merged. Without an explicit identity contract, the drift detector reports a flood of `missing` and `extra` entries that operators must mentally diff.

Policy:

1. Unit IDs (`T1.2`, `T2.1`) are stable across plan revisions unless explicitly remapped.
2. When a remap occurs, the new unit declares `replaces: ["T1.2"]` (array) in the plan.
3. The compare layer treats `replaces` as identity continuity: state entries keyed to a replaced ID are reattributed to the replacement unit without raising `missing` or `extra`.
4. Splits (one old → many new) attribute the state record to the first new unit and surface a non-fatal `split` chip in the drift `extra` bucket as informational, not error.
5. Merges (many old → one new) accept any one of the prior IDs as the state key during a transition window.
6. If `plan_version` increments and `replaces` mappings are absent, drift is reported as-is and a single header banner reads: `plan revised — drift may reflect renames`.

## Data Model and Comparison Logic

### Inputs

- `plan.json` — conforms to `docs/specs/waveplan-plan-schema.json` (with extensions above).
- `state.json` — conforms to `docs/specs/waveplan-state-schema.json` (with extensions above).
- optional event log / heartbeat source (poll interval 1–5s, or push-driven).

### Normalization Layer

```
loadPlan(planJson)   → IndexedPlan { units, deps, owners, kinds, waves, artifacts, doc_index }
loadState(stateJson) → IndexedState { taken, completed, reviews, owners, timestamps }
compare(plan, state) → ComparedView { units: UnitView[], drift: DriftReport, metrics: Metrics }
```

Each `UnitView` carries:

- `id`, `task` (parent), `wave`, `kind`, `label`
- `replacesIds` — array of prior unit IDs this unit absorbs identity from
- `plannedOwner` (nullable), `actualOwner` (nullable)
- `plannedStatus`, `actualStatus`, `derivedStatus`
- `depsTotal`, `depsSatisfied`, `depsBlocking` (subset that resolve to `failed`/`blocked`)
- `verification` — `{ [check]: { total, passed, failedItems, ranAt, source } | null }`
- `expectedArtifacts`, `actualArtifacts`
- `drift` flags: `{ owner, status, deps, artifacts, planRefMismatch }`
- `warnings` — non-fatal issues (e.g. `staleTakenEntry`, `legacyTimestamp`, `missingVerification`)
- `lastUpdatedAt` (ISO 8601), `lastUpdatedSource` (`taken` | `completed` | `verification.<check>` | `event`)

### Derived Status Resolution

Terminal states win over in-flight states. Order of precedence, top wins:

1. `complete` — present in `completed` with `finished_at` and no `failed_at`. Terminal. Wins over a stale `taken` entry from the same unit (write-race resolution).
2. `failed` — `failed_at` is set in either `taken` or `completed`, OR any verification check for the unit reports a non-zero `failed_items` count and the unit is not yet `complete`.
3. `skipped` — `completed.<id>.skipped === true`.
4. `blocked` — `taken.<id>.blocked_reason` is set, OR at least one declared dependency resolves to `failed` or `blocked`.
5. `verifying` — `review_entered_at` set and `review_ended_at` not set, no `failed_at`.
6. `running` — present in `taken` (and not in `completed`), no `failed_at`, no `blocked_reason`.
7. `waiting` — not in `taken`, declared dependencies not all `complete`.
8. `ready` — not in `taken`, all dependencies `complete`.
9. `planned` — none of the above (catch-all).

Conflict rules:

- A unit appearing in both `taken` and `completed` resolves as `complete`. The dashboard surfaces a non-fatal warning chip (`T1.2 stale taken entry`) so the operator can clean up.
- A unit appearing in `completed` without `finished_at` resolves as `failed` if `failed_at` is set, otherwise as `running` (treated as a malformed completion).
- Drift is computed independently and ORed onto any of the above as an overlay flag, never replacing the underlying status.

### Active Task Model — Plural by Default

The state schema permits multiple concurrent entries in `taken`. The dashboard treats the active set as plural and the drawer as a focus selector, not a singular truth.

- The summary bar's `current task` field shows a count when more than one unit is active: `3 active · T1.2 (focused)`.
- The drawer renders one focused unit at a time. Focus rules, top wins:
  1. Operator selection (clicking a node or drift chip).
  2. The unit most recently transitioned to `running` or `verifying`.
  3. Lowest unit ID among the active set.
- An "active set" chip strip sits above the drawer header listing every running/verifying unit. Clicking a chip switches focus.
- Spotlight mode centers the focused unit; non-focused active units render with a thinner accent ring so they remain visible without competing.

Multi-running visibility guarantee — at every render, the dashboard guarantees three independent surfaces show every running/verifying unit:

1. The summary bar's `N active` count.
2. The active-set chip strip above the drawer header — every running/verifying unit gets a chip; the strip never collapses below 24px.
3. Each running/verifying unit's swimlane card carries its own status icon and color regardless of drawer focus; the focused unit gets the N3 emphasis, but the others are not visually deprecated.

A unit being "not in the drawer right now" never causes it to disappear from any surface. The drawer is a focus surface, not the source of truth for activity.

Blockers from non-focused active units are not silently hidden: the drawer's pinned blockers row aggregates blockers across the entire active set, prefixed by unit ID.

### Derived Metrics

All metrics are explicit about their denominator and their behavior under partial data. A metric never silently drops "missing" or "extra" units from its math; it either includes them with a documented rule or surfaces them in a separate counter.

| metric | numerator | denominator | partial-data rule |
|---|---|---|---|
| `totalTasks` | — | — | count of `units` in `IndexedPlan`; extra-in-state units are counted separately as `extraTasks`, never folded in |
| `completedTasks` | derivedStatus ∈ {`complete`, `skipped`} | — | counts only plan-known units |
| `runningTasks` | derivedStatus = `running` | — | plan-known units |
| `blockedTasks` | derivedStatus = `blocked` | — | plan-known units |
| `failedTasks` | derivedStatus = `failed` | — | plan-known units |
| `extraTasks` | state entries with no plan id and no `replaces` alias | — | always non-negative; surfaces in the summary bar when > 0 |
| `driftCount` | sum of all bucket lengths | — | excludes `missing` entries when `confidence` ≠ `high` (see Confidence Model) |
| `verificationPassRate` | sum of `passed` over checks with `verification` data | sum of `total` over checks with `verification` data | tasks with no `verification` block contribute zero to both numerator and denominator and are tallied as `verificationUnreported`; the rate is only meaningful when `verificationUnreported / totalTasks` is small (the panel surfaces this ratio) |
| `dependencyReadiness` | count of units with status ∈ {`ready`, `running`, `verifying`} | count of units with status ≠ `complete` AND ≠ `skipped` | denominator excludes `complete` and `skipped`; if denominator is zero, metric reads `1.0` (everything done) |
| `progressPercent` | `completedTasks` | `totalTasks` | denominator never includes `extraTasks`; `extraTasks > 0` surfaces a separate `(+N extra)` chip beside the percentage |
| `staleHeartbeat` | bool — see Heartbeat | — | true when last accepted-frame age > 15s OR truth-degraded |

## Implementation Shape

Default to a small React dashboard component, single-page app, polling local JSON files at a configurable interval (default 2s). Static-build deployable; no server required for v1.

| concern | choice |
|---|---|
| framework | React 18 |
| graph layout | dagre or elk for swimlane DAG, with custom orthogonal edge router |
| icons | Tabler React |
| color tokens | Radix Colors |
| fonts | self-hosted Inter and JetBrains Mono |
| state | Zustand or React context + reducer (no need for Redux scale) |
| polling | `useEffect` interval + AbortController on each fetch |

### Required UI Components

- `<SummaryBar />`
- `<PipelineStrip />`
- `<Swimlanes />` containing `<TaskNodeCard />` and `<DepEdge />`
- `<ActiveTaskDrawer />`
- `<DriftSections />`
- `<VerificationPanel />`
- `<HeartbeatStrip />`
- `<FilterToolbar />`

Each component reads from a shared `ComparedView`. No component fetches independently; the polling layer publishes new `ComparedView` snapshots.

## Scalability Targets and Render Budgets

The dashboard must remain glanceable at the scales waveplan actually produces in practice (200–1000 units, 5–40 waves) and degrade gracefully past that. The following budgets are normative; implementations must measure against them.

### Hard targets (60 FPS sustained)

- ≤ 200 units, ≤ 10 waves: full-fidelity rendering with all visual effects.
- ≤ 600 units, ≤ 20 waves: full-fidelity with viewport culling.
- ≤ 1000 units, ≤ 40 waves: degraded-fidelity mode (see below).
- > 1000 units: warning banner; recommend `?summary` query param to render aggregate-only view.

### Edge Router Budget

The orthogonal edge router is the first component to fail under density. Constraints:

1. **Recompute trigger** is reflow-bounded, not poll-bounded. Edges are computed once per layout transaction (initial mount, user-triggered reflow, viewport resize). Status changes that do not move cards — the common case — never trigger edge recomputation; only edge color/dash class is mutated via CSS.
2. **Viewport culling**: edges entirely outside the visible scroll region are not rendered. Culling uses the unit's frozen rect (Layout Stability rule) so it is O(visible).
3. **Edge bundling**: when ≥ 4 edges share the same target unit, they collapse to a single trunk edge with a fanned-out source manifold (small chevrons rendered at each source's right edge that converge into one orthogonal trunk). Reduces SVG node count by an order of magnitude in dense fanin.
4. **Crossing budget**: if naïve routing produces > 0.5 × visible-edge-count crossings, the router falls back to a per-wave "bus" routing — each wave gets one shared horizontal corridor in the gap below it; all edges from that wave drop into the bus, travel, and rise to their target. Loss of fidelity, gain of legibility.
5. **Jog cap**: each path is allowed at most 3 right angles. If routing demands more, the path falls back to a smoothed orthogonal that crosses one card border with a transparent gap visualization (gray under-line) — explicit signal that the layout is overconstrained.

### Repaint Budget for Hover and Spotlight

Per-node opacity/shadow changes do not scale. Constraints:

1. **Single dim layer**: dimming non-focused nodes uses one full-viewport overlay div with a punched-out aperture for the focused subgraph (CSS `mask-image`), not N opacity transitions. This is one repaint regardless of unit count.
2. **GPU compositing**: hover/spotlight cards opt into compositing via `will-change: transform, opacity` on hover only (added on `mouseenter`, removed on `mouseleave` to avoid permanent layer cost). Cards in the dim layer never get `will-change`.
3. **Hover debounce**: tooltips and N2 overlays trigger after 80ms of stable hover. Rapid mouse traversal (< 80ms over a card) does not promote the card to N2.
4. **Lineage highlight**: the lineage edge-color brightening is implemented as a CSS class swap on edges already in the DOM, not as a re-render of edges. Edges carry a `data-lineage-of` attribute so the swap is one selector update.

### Urgency Arbitration

Three surfaces compete for the operator's attention: drift sections, verification panel, heartbeat ticker. Without a global arbiter, the operator sees three "this is urgent" claims and cannot triage.

The dashboard maintains a single computed value, `mostUrgentUnitId`, derived from the health pill priority matrix:

1. If `Stale` → no unit; the strip itself owns urgency.
2. Else, the highest-priority condition (Failed > Blocked > PlanMismatch > Drift > Waiting) selects its first unit (ordered by lexicographic ID). That unit's ID becomes `mostUrgentUnitId`.
3. The drift sections, verification panel, and event ticker each render a single subtle `urgent` ring (1px purple) around the row/tile/event that matches `mostUrgentUnitId`. Three surfaces, one signal.
4. When `mostUrgentUnitId` is null, no urgent ring renders anywhere — the dashboard explicitly declares "nothing is screaming."

### Scroll Stability

Auto-scroll on chip click is hostile during live updates. Rules:

1. Scroll targets use `scrollIntoView({ behavior: 'smooth', block: 'center' })` against the frozen rect of the target unit; layout-stability guarantees the rect is invariant across the scroll animation.
2. If the user scrolls (wheel, touch, keyboard) within 200ms of an auto-scroll trigger, the auto-scroll cancels. The chip click behavior degrades to "open drawer + pulse the node" without scrolling.
3. Drift chip click coalesces multiple rapid clicks (< 400ms apart) onto the most recent target; the dashboard scrolls once at the end of the burst.
4. Auto-scroll is disabled entirely during `truth-degraded` and `Stale` conditions; navigation only happens by explicit click.

### Recent-Update Glow Budget

The 10-second glow is a per-node visual effect; at high update cadence on many nodes it is a GPU overdraw multiplier.

Rules:

1. **Concurrent cap**: at most 12 nodes glow at once. When a 13th update arrives, the oldest glow is evicted instantly (no fade-out cross-running) so the budget stays bounded.
2. **GPU layer**: glow renders as a pseudo-element (`::after`) with `transform: scale(1.05)` and `opacity` animation; one GPU layer per glow, no shadow blur (which is a CPU-side filter operation).
3. **Off-viewport cull**: nodes scrolled out of view do not run the glow animation. They resume the remainder when scrolled back in if any timer remains.
4. **Reduced-motion preference**: when the user has `prefers-reduced-motion: reduce`, glow becomes a single solid 200ms fade-in to a static 25% opacity ring with no pulsing — visible signal, zero motion.

### Degraded-Fidelity Mode

When the unit count exceeds the "≤ 600 units" threshold, or the dashboard's measured frame budget drops below 50 FPS for two consecutive polls, the dashboard auto-enters degraded-fidelity:

1. Edges switch to per-wave bus routing globally (no per-edge corridors).
2. N2 overlays disable on hover; only N1 is allowed unless a unit is selected (then N3 pinned).
3. Recent-update glow disables.
4. The summary bar surfaces a small `degraded view` chip the operator can click to read why and force-disable.

This is a defensive ceiling, not a goal. The architecture must aim to never enter it.

## Out of Scope (v1)

- Dark mode (token-ready, not styled).
- Edge flow animation on the running unit's incoming edges.
- Per-wave mini-pipeline strips (P4).
- Multi-plan side-by-side comparison.
- Authoring or editing plans from the dashboard.
- Push-based event subscription (poll-only v1).

## Acceptance Criteria

The dashboard ships when all of the following are true with sample plan + state covering every required schema extension:

1. A new viewer can answer all six glance-questions in under five seconds.
2. Every status in the vocabulary is reachable from the sample data and renders with both the correct icon and the correct color tokens.
3. Pipeline strip click filters swimlanes; toolbar filters compose AND-across / OR-within; chip echo row reflects active filters.
4. Spotlight and Diff modes hide non-relevant nodes and edges as specified.
5. Drift bucket chips open the drawer, scroll-to-center the node, and pulse-highlight it.
6. Verification tile click expands an inline failure list pulled from `verification.<check>.failed_items`; tiles with no data render `— / —` and never invent counts.
7. Stopping the state-file poll for >15s drives the heartbeat strip to red and desaturates nodes; resuming restores instantly.
8. Orthogonal edges never overlap node cards and use the correct color and dash style for their dependency state.
9. All counts (`5/7`, `0:03`, `wave 3 / 12`) render in tabular-nums mono.
10. Removing any color from rendering (grayscale screenshot) still allows status to be inferred from icon and dash style alone.
11. **Frame contract**: when state's `plan_ref` does not match the loaded plan, the dashboard pins the last in-frame view and surfaces the version-mismatch banner; rendering never mixes plan v_n with state targeting v_m.
12. **Conflict resolution**: a unit appearing in both `taken` and `completed` resolves to `complete` and surfaces a `stale taken entry` warning chip — never flickers between `running` and `complete`.
13. **Plural active set**: when ≥2 units are in `taken`, the summary bar shows the count, the active-set chip strip renders all of them, and the drawer focus selector switches between them with a single click.
14. **Truth-degraded mode**: parse failure persisting >60s triggers the purple banner, swimlane desaturation, and drawer action lock; recovery on a successful parse clears the mode within one poll.
15. **Owner drift suppression**: units missing the `owner` plan field produce no `owner` drift entries (silence rather than false drift).
16. **Artifact drift coverage**: a unit declaring `expected_artifacts` and reporting differing `actual_artifacts` produces an entry in the drift `artifacts` bucket with both lists visible on chip click.
17. **Verification authority**: removing the `verification.tests` block from sample state drops the tests tile to `no data`; no synthesized count appears.
18. **Identity continuity**: a plan revision that renames `T1.2` → `T1.5` with `replaces: ["T1.2"]` reattributes the prior state record without raising `missing` or `extra` drift.
19. **Timestamp tolerance**: a state file mixing legacy `YYYY-MM-DD HH:MM` and ISO 8601 timestamps parses without error and surfaces the `mixed timestamp formats` warning in the heartbeat strip.
20. **N1 mandatory fields**: every compact card visibly renders task ID, status (icon + color + shape), owner initials, dependency readiness (`d:x/y`), and a drift overlay when applicable — no tooltip required.
21. **Layout stability**: across 60s of polling on a fixture with churning status and verification counts, no card's `(x, y)` rectangle changes; only their visual chrome updates.
22. **Verification tile order**: tiles render in the fixed order `tests · lint · typecheck · build · review · acceptance` regardless of severity; the only severity cue is left-bar color, corner glyph, and weight.
23. **Pipeline failure facets**: clicking the `failed` facet chip filters swimlanes to failed units; combined with `Implement` shows only failed implement-stage units; facet chips render with their counts even when zero (muted).
24. **Grayscale resilience**: with the page rendered in CSS `filter: grayscale(1)`, every status remains identifiable from glyph shape + border pattern + edge dash style alone, and `failed` vs `complete` remain distinguishable.
25. **Multi-flag health pill**: a fixture combining one failed unit, two drifted units, and a plan-ref mismatch renders three chips in priority order (`Failed 1`, `Plan Mismatch`, `Drift 2`); the primary `Failed 1` chip is full-size, the others scale to 80%.
26. **Drawer rail**: collapsing the drawer reclaims four columns of swimlane width; the choice persists across reload via `localStorage`; hovering the rail surfaces the peek overlay without reflowing the swimlanes.
27. **Degraded-data strip**: triggering truth-degraded surfaces a 28px persistent strip above the pipeline strip; the strip remains until the condition clears or `Acknowledge` is clicked; the strip is the only piece of UI that is allowed to grow page chrome height.
28. **Edge router scale**: with a synthetic 600-unit, 20-wave fixture polling every 2s, the dashboard maintains ≥ 60 FPS during scroll and ≥ 55 FPS during status churn (Chrome Performance panel measurement).
29. **Edge bundling**: a unit with ≥ 4 inbound dependencies renders one bundled trunk edge plus a source manifold, not 4 independent orthogonal paths.
30. **Recompute discipline**: status-only updates (no card movement) on a 200-unit fixture produce zero edge-router executions across 30s of polling, verified by an instrumentation counter exposed under `window.__waveplan.metrics.edgeRecomputes`.
31. **Single dim layer**: enabling Spotlight mode produces exactly one DOM mutation per node-class change, regardless of unit count, verified by the React DevTools Profiler showing one paint per mode toggle.
32. **Urgency arbiter**: when the fixture has one failed unit `T3.1`, the drift sections, verification panel, and event ticker each render a 1px purple ring around the row/tile/event corresponding to `T3.1` and nothing else.
33. **Scroll cancellation**: clicking a drift chip and immediately scrolling within 200ms cancels the auto-scroll; the drawer still opens and the node still pulses.
34. **Glow budget**: simulating 30 simultaneous status updates produces at most 12 active glow elements at any frame; the oldest are evicted FIFO.
35. **Reduced motion**: with `prefers-reduced-motion: reduce`, the running spinner stops, glows become static rings, and pulse animations cease — but every status remains identifiable.
36. **Degraded-fidelity threshold**: a 1000-unit fixture forces edges into bus routing globally and disables N2 hover overlays; the `degraded view` chip appears in the summary bar.
37. **Filter legibility**: applying owner=alice AND wave=3 with no intersection renders a non-blank empty card spelling out the active filters; clicking any filter name in that sentence removes that filter and restores results.
38. **Filter visible/hidden tally**: the active-filter chip row always shows a `N of M visible · K hidden` counter; the count is correct after any combination of filter changes.
39. **Filtered-out vs mode-muted distinction**: filtered-out cards render with a dashed `2 2` border and gray glyph; mode-muted cards keep their solid border and status color at 50% opacity. The two are never visually conflated.
40. **Mode change confirmation**: toggling Default → Spotlight → Diff produces a 200ms color swipe across the visible swimlane area on each transition.
41. **Next Action button gating**: forcing a state file 47s old produces the soft-warn underline + confirmation toast on click; forcing > 60s parse failure locks the button (no-op click + tooltip).
42. **Hover does not commit lineage**: passing the cursor across cards in < 600ms produces tooltips only; sustained 600ms hover OR `Shift+hover` produces lineage edge brightening — a brief sweep never triggers it.
43. **Collapsed-wave loud headers**: a wave with 2 failed and 1 blocked unit, when collapsed, renders a header with the literal counts and a red left bar even when collapsed.
44. **Drift chip two-step navigation**: single click on a drift chip opens the drawer and pulses the node without scrolling; second click within 800ms or `Shift+click` performs the scroll-jump; a back affordance chip appears for 8s after any scroll-jump.
45. **Multi-running visibility**: a fixture with 3 concurrent running units shows count `3 active` in the summary bar, 3 chips in the drawer's active-set strip, and 3 distinct N1 cards on the swimlane each showing their running status — regardless of which one is currently focused in the drawer.
46. **Confidence rendering**: a unit with `expected_artifacts` declared and no `actual_artifacts` block produces a striped chip in the drift panel with `(+1 partial)` in the bucket header; the chip is excluded from `driftCount` but included in `partialDriftCount`.
47. **Unavailable bucket explicit**: a fixture with no plan-side `owner` fields renders the owner bucket header as `0 owner (n/a — no plan owner declared)`; clicking the bucket expands to show the explanation, not chips.
48. **Terminal hysteresis**: a fixture flipping `T1.1` between `complete` and `running` once does not change its rendered status; flipping for two consecutive frames does.
49. **Out-of-order discard**: posting state version 5 then 4 (in that order) leaves the version-5 view on screen; the heartbeat strip does not log an update for version 4.
50. **State version absent warning**: removing `state_version` from the fixture surfaces a `state version missing — frame ordering unverified` chip in the heartbeat strip.
51. **Dependency sub-buckets**: the drift panel's `deps` bucket renders two sub-rows (`runtime` and `topology`); a fixture with a runtime mismatch surfaces under `runtime` only, and `topology` reads `0`.
52. **Ownership handoff**: a unit with `taken.taken_by = alice` and `completed.taken_by = bob` renders a split-chip `alice → bob` in the drawer owner pill; produces a `handoff` entry in the `owner` drift bucket with `confidence: high`; status color does not change.
53. **Timestamp skew**: a unit with `started_at` later than `finished_at` renders normally but logs `clockSkew` in `UnitView.warnings` and shows `skewed` in the drawer middle stratum on hover.
54. **Denominator rules verified**: with 10 plan units and 1 extra-in-state unit, `progressPercent = completedTasks / 10` and the summary bar shows `(+1 extra)` chip; the drift `extra` bucket reports the unit; `dependencyReadiness` does not include the extra unit.
55. **Cyclic dep banner**: a fixture with `T1 → T2 → T1` produces a plan-integrity banner listing the cycle; the affected edges render double-stroke red; the units derive `planned`.
56. **Self-dependency stripped**: a fixture with `T1.2 → T1.2` strips the self-edge, derives status normally, and surfaces a `selfDependency` warning under plan-integrity.
57. **Schema mismatch**: a plan with a unit referencing a missing parent task surfaces `schema mismatch: 1 orphan unit` in the plan-integrity banner; the unit still renders.
58. **Wave renumbering**: changing a unit's wave from 3 to 5 in plan v_n+1 animates the card to the new lane over 200ms once and only once; the renumbering banner appears for 30s.
59. **Deleted task in state**: a fixture where state has `T9.9` but plan does not produces an `extra` drift entry; the drawer is reachable via the chip and shows `unit no longer in plan` banner; the card does not render in any swimlane.
60. **Wave move post-take**: moving a `taken` unit to a new wave animates it once, attaches `waveMoved` warning, and shows `wave: 5 (was 3 in v_n)` in the drawer.
61. **Review flap suppression**: a unit toggling between `verifying` and `running` 4 times in 5s holds its rendered status at the dominant reading; surfaces `reviewFlap` chip in heartbeat ticker.
62. **Truncated state file**: feeding a partial-write JSON 3 times in 6s surfaces the `state file unstable` chip; recovers automatically when the next parse succeeds.
63. **Silent stall detection**: a fixture with one `running` unit whose state never changes for 30s surfaces `no progress 0:30 · 1 units running` chip; at 120s the health pill adds `Stalled`.
64. **Empty plan div-zero**: a plan with zero units shows `—%` progress, `Idle` health pill, and an empty-state card; no `NaN` anywhere in the DOM.
65. **Long label truncation**: a unit with an 80-character title renders single-line with ellipsis on N1 and N2; full text in tooltip; no overlap with drift overlay, blocking dot, or owner avatar.
66. **Long path leading-ellipsis**: a 200-character artifact path in the drawer truncates with leading ellipsis showing the filename intact; full path on hover.
67. **Running vs verifying distinction**: a side-by-side render of one running and one verifying unit is distinguishable by hue (blue vs indigo), glyph (spinner vs static shield-check), motion (continuous vs none), and top-bar pattern (solid vs dashed). All four channels must agree; failing any one fails the test.
68. **WCAG AA contrast**: the build pipeline runs an automated contrast check on every status × text-token combination; build fails if any combination drops below 4.5:1.
69. **Deuteranopia survival**: rendering the dashboard through a deuteranopia simulator at 100/75/50% display saturation keeps every status identifiable by silhouette + border pattern + accent strip alone.
70. **Glyph silhouette uniqueness**: at 16px and 100% color, the nine status glyphs sort into nine distinct silhouette categories (no two share a basic shape).
71. **Drift halo at zoom**: at 50% browser zoom, drift-flagged cards remain identifiable by the 2px purple outer halo even when the 14px overlay glyph is not visible.
72. **Edge density adaptation**: a fixture with 150 visible edges renders pending edges at 30% opacity, satisfied/blocking at full; arrowheads measure 8×10 px not 6×8 px.
73. **Stale hatch overlay**: forcing > 5s heartbeat stale renders a 45-degree hatch pattern across the swimlane viewport; status colors remain unchanged on cards.
74. **Truth-degraded preserves stale hatch**: in truth-degraded mode the hatch remains AND the 25% desaturation applies; the two effects compose without erasing status hue boundaries.
75. **Reduced transparency honored**: a browser reporting `prefers-reduced-transparency: reduce` (where supported) replaces the hatch with a thin amber border around the entire swimlane viewport instead.
76. **Frame-bound transforms**: rapid mode toggling (Default → Spotlight → Diff → Default within 200ms) commits only the final mode after a 300ms debounce; intermediate states are not painted; verified via `window.__waveplan.metrics.modeCommits` counting one per burst.
77. **Visibility precedence**: a unit excluded by filter never renders at full opacity even when in Spotlight mode; the precedence selector returns a single source of truth consumed by every component.
78. **Selection persistence under filter**: pinning focus on `T1.2` then applying a filter that excludes it leaves the drawer showing `T1.2` with a `focused unit is filtered out` warning chip; the selection does not silently move.
79. **Selection survives wave move**: a plan-version advance that moves `T2.1` from wave 3 to wave 5 keeps `T2.1` selected; the selection ring follows the 200ms slide animation.
80. **Selection vanishes gracefully**: removing `T2.1` from both plan and state shows the prior focused snapshot with a `unit T2.1 vanished from plan and state` banner for 5s, then clears selection — never crashes.
81. **Degradation arbitration**: a fixture combining truth-degraded + plan-ref mismatch + legacy timestamps surfaces only the truth-degraded strip as dominant; the other two render as chip-only signals in the heartbeat left zone.
82. **Pinned heartbeat coherence**: in pinned mode the ticker freezes and the sparkline reads `0/s · paused`; the age counter continues to tick relative to wall clock; no new events appear in the ticker.
83. **Pulse defer-and-cancel**: clicking a drift chip whose target is filtered out defers the pulse for 600ms then cancels with a `pulse target not visible` chip in the heartbeat ticker for 4s.
84. **Concurrent pulse coalesce**: triggering 5 pulses on the same unit within 100ms results in exactly one running pulse animation.
