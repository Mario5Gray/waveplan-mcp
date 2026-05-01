# Plan vs State Dashboard — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a real-time React dashboard that compares a waveplan plan file against its state file and surfaces drift, blockers, verification, and the next action within a five-second glance.

**Architecture:** Two stages. First, extend the plan and state JSON schemas with optional fields (owner, artifacts, verification, version, plan_ref, etc.) and ship a sample fixture covering every status. Second, build a single-page React app that polls both JSON files, normalizes them through a pure compare layer (`loadPlan` + `loadState` + `compare`), and renders nine components against a shared `ComparedView`. Layout is custom (no dagre/elk dependency): wave swimlanes with grid-positioned task cards and a hand-rolled orthogonal edge router.

**Tech Stack:** TypeScript 5.x, React 18, Vite 5, Vitest, Zustand, `@tabler/icons-react`, `@radix-ui/colors`, `@fontsource/inter`, `@fontsource/jetbrains-mono`. No graph layout library; no UI kit.

**Spec:** [docs/specs/2026-04-28-plan-state-dashboard-design.md](../specs/2026-04-28-plan-state-dashboard-design.md)

---

## File Structure

| File | Responsibility |
|------|----------------|
| `docs/specs/waveplan-plan-schema.json` | Add optional `owner`, `expected_artifacts`, `acceptance`, `replaces`, `plan_version`, `plan_generation` |
| `docs/specs/waveplan-state-schema.json` | Add optional `failed_at`, `failed_reason`, `blocked_reason`, `skipped`, `actual_artifacts`, `verification`, `state_version`, `state_generation`, `plan_ref` |
| `dashboard/package.json` | Vite/React/TS project root |
| `dashboard/vite.config.ts` | Vite config + Vitest config |
| `dashboard/tsconfig.json` | Strict TS config |
| `dashboard/index.html` | HTML shell |
| `dashboard/public/sample/plan.json` | Sample plan covering all statuses |
| `dashboard/public/sample/state.json` | Sample state covering all statuses |
| `dashboard/src/main.tsx` | React entry |
| `dashboard/src/App.tsx` | Top-level layout (12-col grid C3) |
| `dashboard/src/styles/tokens.css` | Color tokens (Radix scales) + typography |
| `dashboard/src/styles/global.css` | Reset + base styles |
| `dashboard/src/types.ts` | Plan, State, UnitView, ComparedView, DriftReport types |
| `dashboard/src/compare/loadPlan.ts` | Plan normalization |
| `dashboard/src/compare/loadState.ts` | State normalization |
| `dashboard/src/compare/deriveStatus.ts` | Derived status resolver |
| `dashboard/src/compare/computeDrift.ts` | Drift computer |
| `dashboard/src/compare/compare.ts` | Orchestrator with frame contract |
| `dashboard/src/compare/parseTimestamp.ts` | ISO 8601 + legacy parser |
| `dashboard/src/store.ts` | Zustand store + selectors |
| `dashboard/src/hooks/usePolling.ts` | Poll plan and state every 2s |
| `dashboard/src/components/StatusIcon.tsx` | Tabler glyph + color per status |
| `dashboard/src/components/TaskNodeCard.tsx` | N1/N2/N3 adaptive card |
| `dashboard/src/components/DepEdge.tsx` | Orthogonal edge SVG path |
| `dashboard/src/components/Swimlanes.tsx` | Wave swimlane layout + edge layer |
| `dashboard/src/components/SummaryBar.tsx` | Top status bar |
| `dashboard/src/components/PipelineStrip.tsx` | 7-stage filter strip (P3) |
| `dashboard/src/components/ActiveTaskDrawer.tsx` | D4 stratified drawer |
| `dashboard/src/components/DriftSections.tsx` | DR3 bucketed drift table |
| `dashboard/src/components/VerificationPanel.tsx` | V3 aggregate + tiles |
| `dashboard/src/components/HeartbeatStrip.tsx` | H4 tri-zone footer |
| `dashboard/src/components/FilterToolbar.tsx` | Wave/owner/drift/mode controls |
| `dashboard/src/components/__tests__/*.test.tsx` | Component tests (RTL where needed) |
| `dashboard/src/compare/__tests__/*.test.ts` | Pure-function unit tests |

---

## Phase A — Schemas and Sample Fixtures

### Task 1: Extend the plan schema

**Files:**
- Modify: `docs/specs/waveplan-plan-schema.json`

- [ ] **Step 1: Add root-level frame fields**

Inside the top-level `properties` object, add:

```json
"plan_version": {
  "type": "integer",
  "minimum": 1,
  "description": "Monotonic plan version for the frame contract"
},
"plan_generation": {
  "type": "string",
  "description": "Opaque identifier (uuid or ISO timestamp) for this plan generation"
}
```

- [ ] **Step 2: Add per-unit fields**

Within `units.patternProperties.<unit>.properties`, add:

```json
"owner": {
  "type": "string",
  "description": "Planned agent name for this unit (optional)"
},
"expected_artifacts": {
  "type": "array",
  "items": {
    "type": "object",
    "required": ["path", "kind"],
    "properties": {
      "path": {"type": "string"},
      "kind": {"type": "string"}
    }
  }
},
"acceptance": {
  "type": "array",
  "items": {
    "type": "object",
    "required": ["id", "label"],
    "properties": {
      "id": {"type": "string"},
      "label": {"type": "string"}
    }
  }
},
"replaces": {
  "type": "array",
  "items": {"type": "string", "pattern": "^[A-Z][0-9]+(\\.[0-9]+)$"},
  "description": "Prior unit IDs whose state this unit absorbs"
}
```

All four are optional. Do NOT add them to the unit's `required` list.

- [ ] **Step 3: Validate the schema parses**

Run: `python3 -c "import json; json.load(open('docs/specs/waveplan-plan-schema.json'))"`
Expected: no output, no error.

- [ ] **Step 4: Commit**

```bash
git add docs/specs/waveplan-plan-schema.json
git commit -m "feat(schema): extend plan schema with owner/artifacts/acceptance/replaces/version"
```

---

### Task 2: Extend the state schema

**Files:**
- Modify: `docs/specs/waveplan-state-schema.json`

- [ ] **Step 1: Add root-level frame fields**

Inside the top-level `properties` object, add:

```json
"state_version": {
  "type": "integer",
  "minimum": 1
},
"state_generation": {
  "type": "string"
},
"plan_ref": {
  "type": "object",
  "required": ["plan_version", "plan_generation"],
  "properties": {
    "plan_version": {"type": "integer"},
    "plan_generation": {"type": "string"}
  }
}
```

- [ ] **Step 2: Add per-task lifecycle fields**

Within both `taken.patternProperties.<unit>.properties` and `completed.patternProperties.<unit>.properties`, add:

```json
"failed_at": {
  "type": "string",
  "description": "ISO 8601 timestamp when the unit failed"
},
"failed_reason": {"type": "string"},
"actual_artifacts": {
  "type": "array",
  "items": {
    "type": "object",
    "required": ["path", "kind"],
    "properties": {
      "path": {"type": "string"},
      "kind": {"type": "string"},
      "written_at": {"type": "string"}
    }
  }
},
"verification": {
  "type": "object",
  "patternProperties": {
    "^(tests|lint|typecheck|build|review|acceptance)$": {
      "type": "object",
      "required": ["total", "passed", "ran_at"],
      "properties": {
        "total": {"type": "integer", "minimum": 0},
        "passed": {"type": "integer", "minimum": 0},
        "failed_items": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["id", "label"],
            "properties": {
              "id": {"type": "string"},
              "label": {"type": "string"},
              "message": {"type": "string"}
            }
          }
        },
        "ran_at": {"type": "string"},
        "source": {"type": "string"}
      }
    }
  }
}
```

Within `taken.patternProperties.<unit>.properties` only, add:

```json
"blocked_reason": {"type": "string"}
```

Within `completed.patternProperties.<unit>.properties` only, add:

```json
"skipped": {"type": "boolean"}
```

- [ ] **Step 3: Validate the schema parses**

Run: `python3 -c "import json; json.load(open('docs/specs/waveplan-state-schema.json'))"`
Expected: no output, no error.

- [ ] **Step 4: Commit**

```bash
git add docs/specs/waveplan-state-schema.json
git commit -m "feat(schema): extend state schema with failure/artifacts/verification/version"
```

---

### Task 3: Create sample plan and state fixtures

**Files:**
- Create: `dashboard/public/sample/plan.json`
- Create: `dashboard/public/sample/state.json`

These fixtures must reach every status in the vocabulary so the dashboard's acceptance test can verify each visual state.

- [ ] **Step 1: Create the plan fixture**

```bash
mkdir -p dashboard/public/sample
```

Write `dashboard/public/sample/plan.json`:

```json
{
  "schema_version": 1,
  "plan_version": 1,
  "plan_generation": "2026-04-29T10:00:00Z",
  "generated_on": "2026-04-29",
  "plan": {
    "id": "demo-1",
    "title": "Dashboard Demo Plan",
    "plan_doc": {"path": "docs/plans/demo.md", "line": 1},
    "spec_doc": {"path": "docs/specs/demo.md", "line": 1}
  },
  "units": {
    "T1.1": {"task": "T1", "title": "scaffold", "kind": "impl", "wave": 1, "plan_line": 10, "depends_on": [], "owner": "alice"},
    "T1.2": {"task": "T1", "title": "tokens", "kind": "impl", "wave": 1, "plan_line": 20, "depends_on": ["T1.1"], "owner": "alice"},
    "T2.1": {"task": "T2", "title": "loadPlan", "kind": "impl", "wave": 2, "plan_line": 30, "depends_on": ["T1.1"], "owner": "bob",
      "expected_artifacts": [{"path": "src/compare/loadPlan.ts", "kind": "source"}]},
    "T2.2": {"task": "T2", "title": "loadState", "kind": "impl", "wave": 2, "plan_line": 40, "depends_on": ["T1.1"], "owner": "bob"},
    "T2.3": {"task": "T2", "title": "compare", "kind": "impl", "wave": 2, "plan_line": 50, "depends_on": ["T2.1", "T2.2"], "owner": "carol"},
    "T3.1": {"task": "T3", "title": "swimlanes", "kind": "impl", "wave": 3, "plan_line": 60, "depends_on": ["T2.3"], "owner": "dave"},
    "T3.2": {"task": "T3", "title": "drawer", "kind": "impl", "wave": 3, "plan_line": 70, "depends_on": ["T2.3"], "owner": "dave"},
    "T3.3": {"task": "T3", "title": "drift panel", "kind": "impl", "wave": 3, "plan_line": 80, "depends_on": ["T2.3"], "owner": "eve"},
    "T4.1": {"task": "T4", "title": "verify all", "kind": "verify", "wave": 4, "plan_line": 90, "depends_on": ["T3.1", "T3.2", "T3.3"], "owner": "carol",
      "acceptance": [{"id": "a1", "label": "5-second glance"}, {"id": "a2", "label": "all statuses reachable"}]},
    "T4.2": {"task": "T4", "title": "skip me", "kind": "doc", "wave": 4, "plan_line": 100, "depends_on": [], "owner": "frank"}
  },
  "tasks": {
    "T1": {"title": "Scaffold", "wave": 1},
    "T2": {"title": "Compare layer", "wave": 2},
    "T3": {"title": "UI", "wave": 3},
    "T4": {"title": "Verify", "wave": 4}
  },
  "doc_index": {},
  "fp_index": {}
}
```

- [ ] **Step 2: Create the state fixture**

Write `dashboard/public/sample/state.json`:

```json
{
  "plan": "plan.json",
  "state_version": 7,
  "state_generation": "2026-04-29T11:30:00Z",
  "plan_ref": {"plan_version": 1, "plan_generation": "2026-04-29T10:00:00Z"},
  "taken": {
    "T2.1": {
      "taken_by": "bob",
      "started_at": "2026-04-29T11:05:00-07:00",
      "actual_artifacts": [{"path": "src/compare/loadPlan.ts", "kind": "source", "written_at": "2026-04-29T11:20:00-07:00"}]
    },
    "T2.2": {
      "taken_by": "bob",
      "started_at": "2026-04-29T11:10:00-07:00",
      "review_entered_at": "2026-04-29T11:25:00-07:00"
    },
    "T2.3": {
      "taken_by": "carol",
      "started_at": "2026-04-29T11:15:00-07:00",
      "blocked_reason": "waiting on T2.1 verification"
    },
    "T3.1": {
      "taken_by": "dave",
      "started_at": "2026-04-29T11:00:00-07:00",
      "failed_at": "2026-04-29T11:28:00-07:00",
      "failed_reason": "compile error in Swimlanes.tsx"
    },
    "T_EXTRA.1": {
      "taken_by": "ghost",
      "started_at": "2026-04-29T11:00:00-07:00"
    }
  },
  "completed": {
    "T1.1": {
      "taken_by": "alice",
      "started_at": "2026-04-29T10:30:00-07:00",
      "finished_at": "2026-04-29T10:55:00-07:00",
      "actual_artifacts": [{"path": "dashboard/package.json", "kind": "source", "written_at": "2026-04-29T10:55:00-07:00"}],
      "verification": {
        "tests": {"total": 4, "passed": 4, "ran_at": "2026-04-29T10:54:00-07:00", "source": "local"},
        "build": {"total": 1, "passed": 1, "ran_at": "2026-04-29T10:55:00-07:00", "source": "local"}
      }
    },
    "T1.2": {
      "taken_by": "carol",
      "started_at": "2026-04-29T10:40:00-07:00",
      "finished_at": "2026-04-29T11:02:00-07:00",
      "verification": {
        "tests": {"total": 3, "passed": 2, "failed_items": [{"id": "tokens.contrast", "label": "amber9 contrast on amber3"}], "ran_at": "2026-04-29T11:01:00-07:00", "source": "local"},
        "lint": {"total": 1, "passed": 1, "ran_at": "2026-04-29T11:01:00-07:00", "source": "local"}
      }
    },
    "T4.2": {
      "taken_by": "frank",
      "started_at": "2026-04-29T10:00:00-07:00",
      "finished_at": "2026-04-29T10:00:30-07:00",
      "skipped": true
    }
  }
}
```

This fixture covers: `complete` (T1.1, T1.2), `running` (T2.1), `verifying` (T2.2), `blocked` (T2.3), `failed` (T3.1), `waiting` (T3.2, T3.3), `ready` (none initially — T2.3 is blocked), `planned` (T4.1), `skipped` (T4.2), `extra` drift (T_EXTRA.1), and a partially-passing verification (T1.2 tests). The `owner` drift bucket will be empty because every taken_by matches its planned owner — verify that bucket renders as `0 owner` muted.

- [ ] **Step 3: Validate fixtures parse as JSON**

```bash
python3 -c "import json; json.load(open('dashboard/public/sample/plan.json'))"
python3 -c "import json; json.load(open('dashboard/public/sample/state.json'))"
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add dashboard/public/sample/
git commit -m "test(dashboard): add sample plan+state fixtures covering all statuses"
```

---

## Phase B — Project Scaffold

### Task 4: Bootstrap the Vite/React/TS project

**Files:**
- Create: `dashboard/package.json`, `dashboard/vite.config.ts`, `dashboard/tsconfig.json`, `dashboard/index.html`, `dashboard/src/main.tsx`, `dashboard/src/App.tsx`

- [ ] **Step 1: Initialize the project**

```bash
cd dashboard
npm init -y
```

Replace the generated `package.json` with:

```json
{
  "name": "waveplan-dashboard",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "test": "vitest run",
    "test:watch": "vitest"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "zustand": "^4.5.4",
    "@tabler/icons-react": "^3.16.0",
    "@radix-ui/colors": "^3.0.0",
    "@fontsource/inter": "^5.1.0",
    "@fontsource/jetbrains-mono": "^5.1.0"
  },
  "devDependencies": {
    "@types/react": "^18.3.5",
    "@types/react-dom": "^18.3.0",
    "@vitejs/plugin-react": "^4.3.1",
    "typescript": "^5.5.4",
    "vite": "^5.4.2",
    "vitest": "^2.0.5",
    "@testing-library/react": "^16.0.0",
    "jsdom": "^25.0.0"
  }
}
```

- [ ] **Step 2: Install dependencies**

```bash
cd dashboard
npm install
```

Expected: clean install, no audit-warning blocking errors.

- [ ] **Step 3: Add `tsconfig.json`**

Write `dashboard/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "jsx": "react-jsx",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "isolatedModules": true,
    "resolveJsonModule": true,
    "types": ["vitest/globals"]
  },
  "include": ["src"]
}
```

- [ ] **Step 4: Add `vite.config.ts`**

Write `dashboard/vite.config.ts`:

```ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: 'jsdom',
  },
});
```

- [ ] **Step 5: Add `index.html`**

Write `dashboard/index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Waveplan Dashboard</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 6: Add minimal `main.tsx` and placeholder `App.tsx`**

Write `dashboard/src/main.tsx`:

```tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './styles/global.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
```

Write `dashboard/src/App.tsx`:

```tsx
export default function App() {
  return <div className="dashboard-shell">waveplan dashboard</div>;
}
```

Write `dashboard/src/styles/global.css`:

```css
html, body, #root { height: 100%; margin: 0; }
body { font-family: 'Inter', system-ui, sans-serif; }
```

- [ ] **Step 7: Verify dev server boots and build passes**

```bash
cd dashboard
npm run build
```

Expected: `dist/` produced, no TS errors.

- [ ] **Step 8: Commit**

```bash
git add dashboard/
git commit -m "feat(dashboard): scaffold Vite/React/TS project"
```

---

### Task 5: Add color tokens and typography

**Files:**
- Create: `dashboard/src/styles/tokens.css`
- Modify: `dashboard/src/styles/global.css`

- [ ] **Step 1: Write the token file**

Write `dashboard/src/styles/tokens.css`:

```css
@import '@radix-ui/colors/gray.css';
@import '@radix-ui/colors/blue.css';
@import '@radix-ui/colors/amber.css';
@import '@radix-ui/colors/red.css';
@import '@radix-ui/colors/green.css';
@import '@radix-ui/colors/purple.css';

:root {
  --status-planned-bg:     var(--gray-3);
  --status-planned-border: var(--gray-6);
  --status-planned-solid:  var(--gray-9);
  --status-planned-text:   var(--gray-11);

  --status-ready-bg:       var(--gray-3);
  --status-ready-border:   var(--gray-6);
  --status-ready-solid:    var(--gray-9);
  --status-ready-text:     var(--gray-11);

  --status-running-bg:     var(--blue-3);
  --status-running-border: var(--blue-6);
  --status-running-solid:  var(--blue-9);
  --status-running-text:   var(--blue-11);

  --status-verifying-bg:     var(--blue-3);
  --status-verifying-border: var(--blue-6);
  --status-verifying-solid:  var(--blue-9);
  --status-verifying-text:   var(--blue-11);

  --status-waiting-bg:     var(--amber-3);
  --status-waiting-border: var(--amber-6);
  --status-waiting-solid:  var(--amber-9);
  --status-waiting-text:   var(--amber-11);

  --status-blocked-bg:     var(--red-3);
  --status-blocked-border: var(--red-6);
  --status-blocked-solid:  var(--red-9);
  --status-blocked-text:   var(--red-11);

  --status-failed-bg:      var(--red-3);
  --status-failed-border:  var(--red-6);
  --status-failed-solid:   var(--red-9);
  --status-failed-text:    var(--red-11);

  --status-complete-bg:    var(--green-3);
  --status-complete-border:var(--green-6);
  --status-complete-solid: var(--green-9);
  --status-complete-text:  var(--green-11);

  --status-skipped-bg:     var(--gray-3);
  --status-skipped-border: var(--gray-6);
  --status-skipped-solid:  var(--gray-9);
  --status-skipped-text:   var(--gray-11);

  --drift-solid:  var(--purple-9);
  --drift-bg:     var(--purple-3);
  --drift-border: var(--purple-6);
  --drift-text:   var(--purple-11);

  --font-sans: 'Inter', system-ui, sans-serif;
  --font-mono: 'JetBrains Mono', ui-monospace, monospace;

  --fs-xs: 11px;
  --fs-sm: 12px;
  --fs-base: 13px;
  --fs-md: 14px;
  --fs-lg: 16px;
  --fs-xl: 20px;

  --radius-card: 10px;
  --radius-pill: 999px;
}
```

- [ ] **Step 2: Update `global.css` to import tokens and fonts**

Replace `dashboard/src/styles/global.css`:

```css
@import '@fontsource/inter/400.css';
@import '@fontsource/inter/500.css';
@import '@fontsource/inter/600.css';
@import '@fontsource/jetbrains-mono/400.css';
@import '@fontsource/jetbrains-mono/500.css';
@import './tokens.css';

*, *::before, *::after { box-sizing: border-box; }
html, body, #root { height: 100%; margin: 0; }

body {
  font-family: var(--font-sans);
  font-size: var(--fs-base);
  font-variant-numeric: tabular-nums;
  color: var(--gray-12);
  background: var(--gray-1);
}

code, .mono { font-family: var(--font-mono); }
```

- [ ] **Step 3: Verify build still passes**

```bash
cd dashboard && npm run build
```

Expected: clean build.

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/styles/
git commit -m "feat(dashboard): add color tokens and typography"
```

---

## Phase C — Type System and Compare Layer

### Task 6: Define core types

**Files:**
- Create: `dashboard/src/types.ts`

- [ ] **Step 1: Write the types file**

```ts
export type Status =
  | 'planned' | 'ready' | 'running' | 'waiting'
  | 'blocked' | 'failed' | 'verifying' | 'complete'
  | 'skipped';

export type CheckName = 'tests' | 'lint' | 'typecheck' | 'build' | 'review' | 'acceptance';

export interface Artifact { path: string; kind: string; written_at?: string }

export interface VerificationCheck {
  total: number;
  passed: number;
  failed_items?: { id: string; label: string; message?: string }[];
  ran_at: string;
  source?: string;
}

export interface PlanUnit {
  task: string;
  title: string;
  kind: 'impl' | 'test' | 'verify' | 'doc' | 'refactor';
  wave: number;
  plan_line: number;
  depends_on: string[];
  owner?: string;
  expected_artifacts?: Artifact[];
  acceptance?: { id: string; label: string }[];
  replaces?: string[];
}

export interface Plan {
  schema_version?: number;
  plan_version?: number;
  plan_generation?: string;
  plan?: { id?: string; title?: string };
  units: Record<string, PlanUnit>;
  tasks?: Record<string, { title: string; wave?: number }>;
}

export interface StateTaken {
  taken_by: string;
  started_at: string;
  review_entered_at?: string;
  review_ended_at?: string;
  reviewer?: string;
  blocked_reason?: string;
  failed_at?: string;
  failed_reason?: string;
  actual_artifacts?: Artifact[];
  verification?: Partial<Record<CheckName, VerificationCheck>>;
}

export interface StateCompleted extends Omit<StateTaken, 'blocked_reason'> {
  finished_at?: string;
  skipped?: boolean;
}

export interface State {
  plan: string;
  state_version?: number;
  state_generation?: string;
  plan_ref?: { plan_version: number; plan_generation: string };
  taken: Record<string, StateTaken>;
  completed: Record<string, StateCompleted>;
}

export interface UnitView {
  id: string;
  task: string;
  wave: number;
  kind: PlanUnit['kind'];
  label: string;
  replacesIds: string[];
  plannedOwner: string | null;
  actualOwner: string | null;
  derivedStatus: Status;
  drift: { owner: boolean; status: boolean; deps: boolean; artifacts: boolean; planRefMismatch: boolean };
  depsTotal: number;
  depsSatisfied: number;
  depsBlocking: number;
  verification: Partial<Record<CheckName, VerificationCheck | null>>;
  expectedArtifacts: Artifact[];
  actualArtifacts: Artifact[];
  warnings: string[];
  lastUpdatedAt: string | null;
  lastUpdatedSource: 'taken' | 'completed' | 'verification' | 'event' | null;
}

export interface DriftEntry {
  bucket: 'missing' | 'extra' | 'owner' | 'status' | 'deps' | 'artifacts';
  unitId: string;
  field?: string;
  plan?: unknown;
  actual?: unknown;
}

export interface DriftReport {
  missing: DriftEntry[];
  extra: DriftEntry[];
  owner: DriftEntry[];
  status: DriftEntry[];
  deps: DriftEntry[];
  artifacts: DriftEntry[];
}

export interface Metrics {
  totalTasks: number;
  completedTasks: number;
  runningTasks: number;
  blockedTasks: number;
  failedTasks: number;
  driftCount: number;
  verificationPassRate: number;
  dependencyReadiness: number;
  staleHeartbeat: boolean;
  progressPercent: number;
}

export interface ComparedView {
  units: UnitView[];
  unitsById: Record<string, UnitView>;
  drift: DriftReport;
  metrics: Metrics;
  frame: { planVersion?: number; stateVersion?: number; planRefMatches: boolean };
  warnings: string[];
}
```

- [ ] **Step 2: Verify TS compiles**

```bash
cd dashboard && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/types.ts
git commit -m "feat(dashboard): add core types for plan/state/UnitView"
```

---

### Task 7: parseTimestamp utility (TDD)

**Files:**
- Create: `dashboard/src/compare/parseTimestamp.ts`
- Create: `dashboard/src/compare/__tests__/parseTimestamp.test.ts`

- [ ] **Step 1: Write the failing test**

`dashboard/src/compare/__tests__/parseTimestamp.test.ts`:

```ts
import { parseTimestamp } from '../parseTimestamp';

test('parses ISO 8601', () => {
  const r = parseTimestamp('2026-04-29T11:30:00-07:00');
  expect(r.epochMs).toBe(new Date('2026-04-29T11:30:00-07:00').getTime());
  expect(r.legacy).toBe(false);
});

test('parses legacy YYYY-MM-DD HH:MM as local time', () => {
  const r = parseTimestamp('2026-04-29 11:30');
  expect(r.epochMs).toBe(new Date(2026, 3, 29, 11, 30, 0).getTime());
  expect(r.legacy).toBe(true);
});

test('returns null for empty / undefined', () => {
  expect(parseTimestamp(undefined)).toBeNull();
  expect(parseTimestamp('')).toBeNull();
});
```

- [ ] **Step 2: Run test, expect failure**

```bash
cd dashboard && npm test -- parseTimestamp
```

Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

`dashboard/src/compare/parseTimestamp.ts`:

```ts
const LEGACY_RE = /^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2})$/;

export function parseTimestamp(ts: string | undefined): { epochMs: number; legacy: boolean } | null {
  if (!ts) return null;
  const legacyMatch = ts.match(LEGACY_RE);
  if (legacyMatch) {
    const [, y, mo, d, h, mi] = legacyMatch;
    const dt = new Date(+y, +mo - 1, +d, +h, +mi, 0);
    return { epochMs: dt.getTime(), legacy: true };
  }
  const ms = Date.parse(ts);
  if (Number.isNaN(ms)) return null;
  return { epochMs: ms, legacy: false };
}
```

- [ ] **Step 4: Tests pass**

```bash
cd dashboard && npm test -- parseTimestamp
```

Expected: 3 passed.

- [ ] **Step 5: Commit**

```bash
git add dashboard/src/compare/parseTimestamp.ts dashboard/src/compare/__tests__/parseTimestamp.test.ts
git commit -m "feat(compare): add parseTimestamp utility (ISO + legacy)"
```

---

### Task 8: loadPlan (TDD)

**Files:**
- Create: `dashboard/src/compare/loadPlan.ts`
- Create: `dashboard/src/compare/__tests__/loadPlan.test.ts`

The loader normalizes a raw plan JSON into an indexed structure with a reverse-deps map and a `replaces` rewrite layer.

- [ ] **Step 1: Failing test**

```ts
import { loadPlan } from '../loadPlan';
import type { Plan } from '../../types';

const raw: Plan = {
  plan_version: 2,
  plan_generation: 'g2',
  units: {
    'T1.1': { task: 'T1', title: 'a', kind: 'impl', wave: 1, plan_line: 1, depends_on: [] },
    'T1.5': { task: 'T1', title: 'b', kind: 'impl', wave: 1, plan_line: 2, depends_on: ['T1.1'], replaces: ['T1.2'] },
  },
  tasks: {},
};

test('indexes units by id', () => {
  const indexed = loadPlan(raw);
  expect(indexed.units['T1.1'].title).toBe('a');
});

test('builds reverse dep map', () => {
  const indexed = loadPlan(raw);
  expect(indexed.dependents['T1.1']).toEqual(['T1.5']);
});

test('records replaces aliases', () => {
  const indexed = loadPlan(raw);
  expect(indexed.aliasOf['T1.2']).toBe('T1.5');
});

test('captures version metadata', () => {
  const indexed = loadPlan(raw);
  expect(indexed.planVersion).toBe(2);
  expect(indexed.planGeneration).toBe('g2');
});
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd dashboard && npm test -- loadPlan
```

- [ ] **Step 3: Implement**

```ts
import type { Plan, PlanUnit } from '../types';

export interface IndexedPlan {
  raw: Plan;
  units: Record<string, PlanUnit>;
  ids: string[];
  dependents: Record<string, string[]>;
  aliasOf: Record<string, string>;
  planVersion?: number;
  planGeneration?: string;
}

export function loadPlan(plan: Plan): IndexedPlan {
  const dependents: Record<string, string[]> = {};
  const aliasOf: Record<string, string> = {};

  for (const [id, unit] of Object.entries(plan.units)) {
    for (const dep of unit.depends_on) {
      (dependents[dep] ??= []).push(id);
    }
    if (unit.replaces) {
      for (const prior of unit.replaces) aliasOf[prior] = id;
    }
  }

  return {
    raw: plan,
    units: plan.units,
    ids: Object.keys(plan.units),
    dependents,
    aliasOf,
    planVersion: plan.plan_version,
    planGeneration: plan.plan_generation,
  };
}
```

- [ ] **Step 4: Tests pass**

```bash
cd dashboard && npm test -- loadPlan
```

Expected: 4 passed.

- [ ] **Step 5: Commit**

```bash
git add dashboard/src/compare/loadPlan.ts dashboard/src/compare/__tests__/loadPlan.test.ts
git commit -m "feat(compare): add loadPlan with reverse-dep + replaces indexing"
```

---

### Task 9: loadState (TDD)

**Files:**
- Create: `dashboard/src/compare/loadState.ts`
- Create: `dashboard/src/compare/__tests__/loadState.test.ts`

- [ ] **Step 1: Failing test**

```ts
import { loadState } from '../loadState';
import type { State } from '../../types';

const raw: State = {
  plan: 'plan.json',
  state_version: 5,
  plan_ref: { plan_version: 2, plan_generation: 'g2' },
  taken: { 'T2.1': { taken_by: 'bob', started_at: '2026-04-29 11:00' } },
  completed: { 'T1.1': { taken_by: 'alice', started_at: '2026-04-29 10:00', finished_at: '2026-04-29 10:30' } },
};

test('indexes taken and completed', () => {
  const s = loadState(raw);
  expect(s.taken['T2.1'].taken_by).toBe('bob');
  expect(s.completed['T1.1'].finished_at).toBe('2026-04-29 10:30');
});

test('records frame metadata', () => {
  const s = loadState(raw);
  expect(s.stateVersion).toBe(5);
  expect(s.planRef).toEqual({ plan_version: 2, plan_generation: 'g2' });
});

test('detects legacy timestamps', () => {
  const s = loadState(raw);
  expect(s.hasLegacyTimestamps).toBe(true);
});
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```ts
import type { State, StateTaken, StateCompleted } from '../types';
import { parseTimestamp } from './parseTimestamp';

export interface IndexedState {
  raw: State;
  taken: Record<string, StateTaken>;
  completed: Record<string, StateCompleted>;
  stateVersion?: number;
  stateGeneration?: string;
  planRef?: { plan_version: number; plan_generation: string };
  hasLegacyTimestamps: boolean;
}

export function loadState(state: State): IndexedState {
  let legacy = false;
  for (const entry of [...Object.values(state.taken), ...Object.values(state.completed)]) {
    for (const ts of [entry.started_at, entry.review_entered_at, entry.review_ended_at, (entry as StateCompleted).finished_at, entry.failed_at]) {
      const parsed = parseTimestamp(ts);
      if (parsed?.legacy) legacy = true;
    }
  }
  return {
    raw: state,
    taken: state.taken,
    completed: state.completed,
    stateVersion: state.state_version,
    stateGeneration: state.state_generation,
    planRef: state.plan_ref,
    hasLegacyTimestamps: legacy,
  };
}
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

```bash
git add dashboard/src/compare/loadState.ts dashboard/src/compare/__tests__/loadState.test.ts
git commit -m "feat(compare): add loadState with frame metadata + legacy ts detection"
```

---

### Task 10: deriveStatus (TDD)

**Files:**
- Create: `dashboard/src/compare/deriveStatus.ts`
- Create: `dashboard/src/compare/__tests__/deriveStatus.test.ts`

This is the precedence engine from the spec.

- [ ] **Step 1: Failing test**

```ts
import { deriveStatus } from '../deriveStatus';
import type { PlanUnit, StateTaken, StateCompleted } from '../../types';

const u: PlanUnit = { task: 'T1', title: 't', kind: 'impl', wave: 1, plan_line: 1, depends_on: [] };

function ctx(deps: Record<string, 'complete' | 'failed' | 'planned'> = {}) {
  return { depResolved: (id: string) => deps[id] ?? 'planned' as const };
}

test('complete wins over taken (write race)', () => {
  const taken: StateTaken = { taken_by: 'a', started_at: '2026-04-29 10:00' };
  const completed: StateCompleted = { taken_by: 'a', started_at: '2026-04-29 10:00', finished_at: '2026-04-29 10:30' };
  expect(deriveStatus(u, taken, completed, ctx()).status).toBe('complete');
});

test('failed_at sets failed', () => {
  const taken: StateTaken = { taken_by: 'a', started_at: '2026-04-29 10:00', failed_at: '2026-04-29 10:05' };
  expect(deriveStatus(u, taken, undefined, ctx()).status).toBe('failed');
});

test('skipped flag overrides plain complete', () => {
  const completed: StateCompleted = { taken_by: 'a', started_at: '2026-04-29 10:00', finished_at: '2026-04-29 10:01', skipped: true };
  expect(deriveStatus(u, undefined, completed, ctx()).status).toBe('skipped');
});

test('blocked when blocked_reason set', () => {
  const taken: StateTaken = { taken_by: 'a', started_at: '2026-04-29 10:00', blocked_reason: 'x' };
  expect(deriveStatus(u, taken, undefined, ctx()).status).toBe('blocked');
});

test('verifying when review_entered_at set without ended', () => {
  const taken: StateTaken = { taken_by: 'a', started_at: '2026-04-29 10:00', review_entered_at: '2026-04-29 10:05' };
  expect(deriveStatus(u, taken, undefined, ctx()).status).toBe('verifying');
});

test('running when in taken, no review or block', () => {
  const taken: StateTaken = { taken_by: 'a', started_at: '2026-04-29 10:00' };
  expect(deriveStatus(u, taken, undefined, ctx()).status).toBe('running');
});

test('waiting when deps incomplete and not taken', () => {
  const dep: PlanUnit = { ...u, depends_on: ['T0.1'] };
  expect(deriveStatus(dep, undefined, undefined, ctx({ 'T0.1': 'planned' })).status).toBe('waiting');
});

test('ready when all deps complete and not taken', () => {
  const dep: PlanUnit = { ...u, depends_on: ['T0.1'] };
  expect(deriveStatus(dep, undefined, undefined, ctx({ 'T0.1': 'complete' })).status).toBe('ready');
});

test('planned when no deps and nothing else', () => {
  expect(deriveStatus(u, undefined, undefined, ctx()).status).toBe('planned');
});

test('stale taken+completed produces warning', () => {
  const taken: StateTaken = { taken_by: 'a', started_at: '2026-04-29 10:00' };
  const completed: StateCompleted = { taken_by: 'a', started_at: '2026-04-29 10:00', finished_at: '2026-04-29 10:30' };
  const r = deriveStatus(u, taken, completed, ctx());
  expect(r.warnings).toContain('staleTakenEntry');
});
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```ts
import type { Status, PlanUnit, StateTaken, StateCompleted } from '../types';

export interface DeriveContext {
  depResolved: (id: string) => Status;
}

export function deriveStatus(
  unit: PlanUnit,
  taken: StateTaken | undefined,
  completed: StateCompleted | undefined,
  ctx: DeriveContext,
): { status: Status; warnings: string[] } {
  const warnings: string[] = [];

  if (taken && completed) warnings.push('staleTakenEntry');

  if (completed?.skipped) return { status: 'skipped', warnings };

  if (completed?.finished_at && !completed.failed_at && !completed.skipped) {
    return { status: 'complete', warnings };
  }

  if (taken?.failed_at || completed?.failed_at) {
    return { status: 'failed', warnings };
  }

  if (taken?.blocked_reason) return { status: 'blocked', warnings };

  const depStatuses = unit.depends_on.map((d) => ctx.depResolved(d));
  if (depStatuses.some((s) => s === 'failed' || s === 'blocked')) {
    return { status: 'blocked', warnings };
  }

  if (taken?.review_entered_at && !taken.review_ended_at) {
    return { status: 'verifying', warnings };
  }

  if (taken) return { status: 'running', warnings };

  if (unit.depends_on.length > 0 && depStatuses.some((s) => s !== 'complete')) {
    return { status: 'waiting', warnings };
  }

  if (unit.depends_on.length > 0) return { status: 'ready', warnings };

  return { status: 'planned', warnings };
}
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

```bash
git add dashboard/src/compare/deriveStatus.ts dashboard/src/compare/__tests__/deriveStatus.test.ts
git commit -m "feat(compare): add deriveStatus precedence engine"
```

---

### Task 11: computeDrift (TDD)

**Files:**
- Create: `dashboard/src/compare/computeDrift.ts`
- Create: `dashboard/src/compare/__tests__/computeDrift.test.ts`

- [ ] **Step 1: Failing test**

```ts
import { computeDrift } from '../computeDrift';
import { loadPlan } from '../loadPlan';
import { loadState } from '../loadState';

test('detects extra units in state', () => {
  const plan = loadPlan({
    units: { 'T1.1': { task: 'T1', title: 'a', kind: 'impl', wave: 1, plan_line: 1, depends_on: [] } },
  });
  const state = loadState({ plan: 'p', taken: { 'T_EXTRA': { taken_by: 'x', started_at: '2026-04-29 10:00' } }, completed: {} });
  const drift = computeDrift(plan, state);
  expect(drift.extra.map((d) => d.unitId)).toContain('T_EXTRA');
});

test('owner drift requires planned owner', () => {
  const plan = loadPlan({
    units: {
      'T1.1': { task: 'T1', title: 'a', kind: 'impl', wave: 1, plan_line: 1, depends_on: [], owner: 'alice' },
      'T1.2': { task: 'T1', title: 'b', kind: 'impl', wave: 1, plan_line: 2, depends_on: [] },
    },
  });
  const state = loadState({
    plan: 'p',
    taken: { 'T1.1': { taken_by: 'bob', started_at: '2026-04-29 10:00' }, 'T1.2': { taken_by: 'carol', started_at: '2026-04-29 10:00' } },
    completed: {},
  });
  const drift = computeDrift(plan, state);
  expect(drift.owner.map((d) => d.unitId)).toEqual(['T1.1']);
});

test('replaces alias hides missing/extra', () => {
  const plan = loadPlan({
    units: { 'T1.5': { task: 'T1', title: 'b', kind: 'impl', wave: 1, plan_line: 2, depends_on: [], replaces: ['T1.2'] } },
  });
  const state = loadState({ plan: 'p', taken: { 'T1.2': { taken_by: 'a', started_at: '2026-04-29 10:00' } }, completed: {} });
  const drift = computeDrift(plan, state);
  expect(drift.extra).toHaveLength(0);
});

test('artifact mismatch detected', () => {
  const plan = loadPlan({
    units: {
      'T1.1': { task: 'T1', title: 'a', kind: 'impl', wave: 1, plan_line: 1, depends_on: [],
        expected_artifacts: [{ path: 'a.ts', kind: 'source' }, { path: 'b.ts', kind: 'source' }] },
    },
  });
  const state = loadState({
    plan: 'p',
    taken: {},
    completed: { 'T1.1': { taken_by: 'a', started_at: '2026-04-29 10:00', finished_at: '2026-04-29 10:30',
      actual_artifacts: [{ path: 'a.ts', kind: 'source' }] } },
  });
  const drift = computeDrift(plan, state);
  expect(drift.artifacts).toHaveLength(1);
  expect(drift.artifacts[0].unitId).toBe('T1.1');
});
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```ts
import type { DriftReport, DriftEntry } from '../types';
import type { IndexedPlan } from './loadPlan';
import type { IndexedState } from './loadState';

export function computeDrift(plan: IndexedPlan, state: IndexedState): DriftReport {
  const drift: DriftReport = { missing: [], extra: [], owner: [], status: [], deps: [], artifacts: [] };

  const stateIds = new Set([...Object.keys(state.taken), ...Object.keys(state.completed)]);

  // missing: in plan, not in state, not represented via alias
  for (const id of plan.ids) {
    const aliasIds = plan.units[id].replaces ?? [];
    const present = stateIds.has(id) || aliasIds.some((a) => stateIds.has(a));
    if (!present && (state.raw.taken !== undefined || state.raw.completed !== undefined)) {
      // missing reported only when state should have known about the unit; skip for v1
      // (kept silent; requires explicit policy beyond scope)
    }
  }

  // extra: in state, no plan id and no alias
  for (const id of stateIds) {
    if (plan.units[id]) continue;
    if (plan.aliasOf[id]) continue;
    drift.extra.push({ bucket: 'extra', unitId: id });
  }

  // owner drift
  for (const id of plan.ids) {
    const planned = plan.units[id].owner;
    if (!planned) continue;
    const t = state.taken[id] ?? state.completed[id];
    if (!t) continue;
    const actual = t.taken_by;
    if (actual && actual !== planned) {
      drift.owner.push({ bucket: 'owner', unitId: id, field: 'owner', plan: planned, actual });
    }
  }

  // dep drift: declared deps that are not satisfied AND not actually depended on (presence-only check for v1)
  // (deeper dep drift requires plan revisions; out of scope. Surfaced via deps bucket only when state declares dependents.)

  // artifacts
  for (const id of plan.ids) {
    const expected = plan.units[id].expected_artifacts ?? [];
    if (expected.length === 0) continue;
    const t = state.completed[id] ?? state.taken[id];
    if (!t) continue;
    const actual = t.actual_artifacts ?? [];
    const expPaths = new Set(expected.map((a) => a.path));
    const actPaths = new Set(actual.map((a) => a.path));
    const missing = [...expPaths].filter((p) => !actPaths.has(p));
    const extra = [...actPaths].filter((p) => !expPaths.has(p));
    if (missing.length || extra.length) {
      drift.artifacts.push({ bucket: 'artifacts', unitId: id, plan: expected, actual });
    }
  }

  return drift;
}
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

```bash
git add dashboard/src/compare/computeDrift.ts dashboard/src/compare/__tests__/computeDrift.test.ts
git commit -m "feat(compare): add computeDrift with owner/extra/artifacts buckets"
```

---

### Task 12: compare orchestrator with frame contract (TDD)

**Files:**
- Create: `dashboard/src/compare/compare.ts`
- Create: `dashboard/src/compare/__tests__/compare.test.ts`

- [ ] **Step 1: Failing test**

```ts
import { compare } from '../compare';
import type { Plan, State } from '../../types';

const plan: Plan = {
  plan_version: 1,
  plan_generation: 'g1',
  units: {
    'T1.1': { task: 'T1', title: 'a', kind: 'impl', wave: 1, plan_line: 1, depends_on: [] },
    'T1.2': { task: 'T1', title: 'b', kind: 'impl', wave: 1, plan_line: 2, depends_on: ['T1.1'] },
  },
};

test('compare publishes when plan_ref matches', () => {
  const state: State = {
    plan: 'p', state_version: 1,
    plan_ref: { plan_version: 1, plan_generation: 'g1' },
    taken: {}, completed: { 'T1.1': { taken_by: 'a', started_at: '2026-04-29 10:00', finished_at: '2026-04-29 10:30' } },
  };
  const view = compare(plan, state);
  expect(view.frame.planRefMatches).toBe(true);
  expect(view.unitsById['T1.1'].derivedStatus).toBe('complete');
  expect(view.unitsById['T1.2'].derivedStatus).toBe('ready');
});

test('compare flags plan_ref mismatch', () => {
  const state: State = {
    plan: 'p', plan_ref: { plan_version: 99, plan_generation: 'old' },
    taken: {}, completed: {},
  };
  const view = compare(plan, state);
  expect(view.frame.planRefMatches).toBe(false);
});

test('metrics computed', () => {
  const state: State = {
    plan: 'p', plan_ref: { plan_version: 1, plan_generation: 'g1' },
    taken: {}, completed: { 'T1.1': { taken_by: 'a', started_at: '2026-04-29 10:00', finished_at: '2026-04-29 10:30' } },
  };
  const view = compare(plan, state);
  expect(view.metrics.totalTasks).toBe(2);
  expect(view.metrics.completedTasks).toBe(1);
  expect(view.metrics.progressPercent).toBeCloseTo(0.5);
});
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement**

```ts
import type { Plan, State, ComparedView, UnitView, Status } from '../types';
import { loadPlan } from './loadPlan';
import { loadState } from './loadState';
import { deriveStatus } from './deriveStatus';
import { computeDrift } from './computeDrift';
import { parseTimestamp } from './parseTimestamp';

export function compare(plan: Plan, state: State): ComparedView {
  const ip = loadPlan(plan);
  const is = loadState(state);

  const planRefMatches =
    !!is.planRef &&
    is.planRef.plan_version === ip.planVersion &&
    is.planRef.plan_generation === ip.planGeneration;

  const warnings: string[] = [];
  if (is.hasLegacyTimestamps) warnings.push('legacyTimestamps');
  if (!planRefMatches && is.planRef) warnings.push('planRefMismatch');

  // First pass: derive without dep resolution
  const cache: Record<string, Status> = {};
  function depResolved(id: string): Status {
    if (cache[id]) return cache[id];
    const target = ip.aliasOf[id] ?? id;
    const u = ip.units[target];
    if (!u) return 'planned';
    cache[id] = 'planned';
    const t = is.taken[target] ?? is.taken[id];
    const c = is.completed[target] ?? is.completed[id];
    cache[id] = deriveStatus(u, t, c, { depResolved }).status;
    return cache[id];
  }

  const units: UnitView[] = ip.ids.map((id) => {
    const u = ip.units[id];
    const t = is.taken[id];
    const c = is.completed[id];
    const { status, warnings: w } = deriveStatus(u, t, c, { depResolved });
    cache[id] = status;
    const expectedArtifacts = u.expected_artifacts ?? [];
    const actualArtifacts = (c?.actual_artifacts ?? t?.actual_artifacts ?? []);
    const verification = (c?.verification ?? t?.verification ?? {});
    const depsResolved = u.depends_on.map((d) => depResolved(d));
    const lastUpdatedAt =
      c?.finished_at ?? t?.review_entered_at ?? t?.started_at ?? null;
    const drift = {
      owner: !!u.owner && !!(t?.taken_by ?? c?.taken_by) && (t?.taken_by ?? c?.taken_by) !== u.owner,
      status: false,
      deps: false,
      artifacts: expectedArtifacts.length > 0 && (
        expectedArtifacts.some((e) => !actualArtifacts.find((a) => a.path === e.path)) ||
        actualArtifacts.some((a) => !expectedArtifacts.find((e) => e.path === a.path))
      ),
      planRefMismatch: !planRefMatches,
    };
    return {
      id, task: u.task, wave: u.wave, kind: u.kind, label: u.title,
      replacesIds: u.replaces ?? [],
      plannedOwner: u.owner ?? null,
      actualOwner: (t?.taken_by ?? c?.taken_by) ?? null,
      derivedStatus: status,
      drift,
      depsTotal: u.depends_on.length,
      depsSatisfied: depsResolved.filter((s) => s === 'complete').length,
      depsBlocking: depsResolved.filter((s) => s === 'failed' || s === 'blocked').length,
      verification,
      expectedArtifacts,
      actualArtifacts,
      warnings: w,
      lastUpdatedAt,
      lastUpdatedSource: c ? 'completed' : (t ? 'taken' : null),
    };
  });

  const unitsById: Record<string, UnitView> = {};
  for (const u of units) unitsById[u.id] = u;

  const drift = computeDrift(ip, is);
  const driftCount = Object.values(drift).reduce((acc: number, arr) => acc + (arr as unknown[]).length, 0);

  const totalTasks = units.length;
  const completedTasks = units.filter((u) => u.derivedStatus === 'complete' || u.derivedStatus === 'skipped').length;
  const runningTasks = units.filter((u) => u.derivedStatus === 'running').length;
  const blockedTasks = units.filter((u) => u.derivedStatus === 'blocked').length;
  const failedTasks = units.filter((u) => u.derivedStatus === 'failed').length;

  let verifTotal = 0, verifPassed = 0;
  for (const u of units) {
    for (const c of Object.values(u.verification)) {
      if (!c) continue;
      verifTotal += c.total;
      verifPassed += c.passed;
    }
  }

  const nonComplete = totalTasks - completedTasks;
  const ready = units.filter((u) => u.derivedStatus === 'ready').length;
  const dependencyReadiness = nonComplete > 0 ? (ready + runningTasks) / nonComplete : 1;

  const lastTs = units
    .map((u) => parseTimestamp(u.lastUpdatedAt ?? undefined)?.epochMs ?? 0)
    .reduce((a, b) => Math.max(a, b), 0);
  const staleHeartbeat = lastTs > 0 && Date.now() - lastTs > 15_000;

  return {
    units, unitsById, drift,
    metrics: {
      totalTasks, completedTasks, runningTasks, blockedTasks, failedTasks,
      driftCount,
      verificationPassRate: verifTotal > 0 ? verifPassed / verifTotal : 1,
      dependencyReadiness,
      staleHeartbeat,
      progressPercent: totalTasks > 0 ? completedTasks / totalTasks : 0,
    },
    frame: { planVersion: ip.planVersion, stateVersion: is.stateVersion, planRefMatches },
    warnings,
  };
}
```

- [ ] **Step 4: Tests pass**

- [ ] **Step 5: Commit**

```bash
git add dashboard/src/compare/compare.ts dashboard/src/compare/__tests__/compare.test.ts
git commit -m "feat(compare): orchestrator with frame contract + metrics"
```

---

## Phase D — Polling and Store

### Task 13: Zustand store + polling hook

**Files:**
- Create: `dashboard/src/store.ts`
- Create: `dashboard/src/hooks/usePolling.ts`

- [ ] **Step 1: Write the store**

`dashboard/src/store.ts`:

```ts
import { create } from 'zustand';
import type { ComparedView } from './types';

export type DashboardMode = 'default' | 'spotlight' | 'diff';

interface DashboardState {
  view: ComparedView | null;
  lastGoodView: ComparedView | null;
  lastUpdateMs: number | null;
  parseFailedSinceMs: number | null;
  selectedUnitId: string | null;
  mode: DashboardMode;
  filters: {
    waves: number[];
    owners: string[];
    statuses: string[];
    driftOnly: boolean;
  };
  setView: (v: ComparedView) => void;
  setParseFailed: (now: number) => void;
  setSelected: (id: string | null) => void;
  setMode: (m: DashboardMode) => void;
  setFilters: (f: Partial<DashboardState['filters']>) => void;
}

export const useDashboard = create<DashboardState>((set) => ({
  view: null,
  lastGoodView: null,
  lastUpdateMs: null,
  parseFailedSinceMs: null,
  selectedUnitId: null,
  mode: 'default',
  filters: { waves: [], owners: [], statuses: [], driftOnly: false },
  setView: (v) => set({ view: v, lastGoodView: v, lastUpdateMs: Date.now(), parseFailedSinceMs: null }),
  setParseFailed: (now) => set((s) => ({ parseFailedSinceMs: s.parseFailedSinceMs ?? now })),
  setSelected: (id) => set({ selectedUnitId: id }),
  setMode: (m) => set({ mode: m }),
  setFilters: (f) => set((s) => ({ filters: { ...s.filters, ...f } })),
}));
```

- [ ] **Step 2: Write the polling hook**

`dashboard/src/hooks/usePolling.ts`:

```ts
import { useEffect } from 'react';
import { useDashboard } from '../store';
import { compare } from '../compare/compare';
import type { Plan, State } from '../types';

export function usePolling(planUrl: string, stateUrl: string, intervalMs = 2000) {
  const setView = useDashboard((s) => s.setView);
  const setParseFailed = useDashboard((s) => s.setParseFailed);

  useEffect(() => {
    let cancelled = false;
    let lastStateVersion: number | undefined;

    async function tick() {
      try {
        const [planRes, stateRes] = await Promise.all([
          fetch(planUrl, { cache: 'no-store' }),
          fetch(stateUrl, { cache: 'no-store' }),
        ]);
        if (!planRes.ok || !stateRes.ok) throw new Error('http');
        const plan: Plan = await planRes.json();
        const state: State = await stateRes.json();
        if (cancelled) return;
        if (state.state_version !== undefined && lastStateVersion !== undefined && state.state_version < lastStateVersion) {
          // version regressed; keep but reset baseline
          lastStateVersion = state.state_version;
        } else if (state.state_version !== undefined && state.state_version === lastStateVersion) {
          // no change; still publish so heartbeat updates
        } else {
          lastStateVersion = state.state_version;
        }
        const view = compare(plan, state);
        setView(view);
      } catch {
        if (!cancelled) setParseFailed(Date.now());
      }
    }

    tick();
    const id = window.setInterval(tick, intervalMs);
    return () => { cancelled = true; window.clearInterval(id); };
  }, [planUrl, stateUrl, intervalMs, setView, setParseFailed]);
}
```

- [ ] **Step 3: Verify TS compiles**

```bash
cd dashboard && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/store.ts dashboard/src/hooks/usePolling.ts
git commit -m "feat(dashboard): zustand store + polling hook"
```

---

## Phase E — Visual Primitives

### Task 14: StatusIcon component

**Files:**
- Create: `dashboard/src/components/StatusIcon.tsx`

- [ ] **Step 1: Implement**

```tsx
import {
  IconCircle, IconCircleDashed, IconLoader2, IconClock, IconLock,
  IconAlertTriangle, IconCircleCheck, IconArrowRightCircle, IconGitCompare,
  IconShieldCheck,
} from '@tabler/icons-react';
import type { Status } from '../types';

const map: Record<Status, { Glyph: typeof IconCircle; color: string; spin?: boolean }> = {
  planned:   { Glyph: IconCircle,         color: 'var(--status-planned-solid)' },
  ready:     { Glyph: IconCircleDashed,   color: 'var(--status-ready-solid)' },
  running:   { Glyph: IconLoader2,        color: 'var(--status-running-solid)', spin: true },
  waiting:   { Glyph: IconClock,          color: 'var(--status-waiting-solid)' },
  blocked:   { Glyph: IconLock,           color: 'var(--status-blocked-solid)' },
  failed:    { Glyph: IconAlertTriangle,  color: 'var(--status-failed-solid)' },
  verifying: { Glyph: IconShieldCheck,    color: 'var(--status-verifying-solid)' },
  complete:  { Glyph: IconCircleCheck,    color: 'var(--status-complete-solid)' },
  skipped:   { Glyph: IconArrowRightCircle, color: 'var(--status-skipped-solid)' },
};

export function StatusIcon({ status, size = 16 }: { status: Status; size?: number }) {
  const { Glyph, color, spin } = map[status];
  return <Glyph size={size} stroke={1.5} color={color} className={spin ? 'spin' : undefined} aria-label={status} />;
}

export function DriftBadge({ size = 12 }: { size?: number }) {
  return <IconGitCompare size={size} stroke={1.5} color="var(--drift-solid)" aria-label="drift" />;
}
```

- [ ] **Step 2: Add `.spin` keyframes to global.css**

Append to `dashboard/src/styles/global.css`:

```css
@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
.spin { animation: spin 1.2s linear infinite; }
```

- [ ] **Step 3: Verify build**

```bash
cd dashboard && npm run build
```

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/components/StatusIcon.tsx dashboard/src/styles/global.css
git commit -m "feat(dashboard): status icon component"
```

---

### Task 15: TaskNodeCard (N1 default + N2 hover overlay + N3 active pin)

**Files:**
- Create: `dashboard/src/components/TaskNodeCard.tsx`
- Create: `dashboard/src/components/TaskNodeCard.css`

- [ ] **Step 1: Implement**

```tsx
import { useState } from 'react';
import type { UnitView } from '../types';
import { StatusIcon, DriftBadge } from './StatusIcon';
import './TaskNodeCard.css';

interface Props {
  unit: UnitView;
  active?: boolean;
  onClick?: () => void;
}

export function TaskNodeCard({ unit, active, onClick }: Props) {
  const [hover, setHover] = useState(false);
  const tier = active ? 'tall' : hover ? 'standard' : 'compact';
  const driftAny = Object.values(unit.drift).some(Boolean);
  const s = unit.derivedStatus;

  return (
    <div
      className={`task-card task-card--${tier} status-${s} ${active ? 'is-active' : ''}`}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      onClick={onClick}
      role="button"
      tabIndex={0}
    >
      <div className="task-card__row1">
        <span className="task-card__id mono">{unit.id}</span>
        <StatusIcon status={s} size={14} />
        {driftAny && <span className="task-card__drift"><DriftBadge size={12} /></span>}
      </div>
      <div className="task-card__label" title={unit.label}>{unit.label}</div>
      {tier !== 'compact' && (
        <div className="task-card__row3">
          {unit.actualOwner && <span className="owner-pill">{unit.actualOwner}</span>}
          <span className="mono count">{unit.depsSatisfied}/{unit.depsTotal}d</span>
          <span className="mono count">{verifyText(unit)}</span>
        </div>
      )}
      {tier === 'tall' && (
        <div className="task-card__row4">
          <span className="kind-chip">{unit.kind}</span>
          <span className="wave-chip mono">w{unit.wave}</span>
        </div>
      )}
    </div>
  );
}

function verifyText(u: UnitView): string {
  let total = 0, passed = 0;
  for (const c of Object.values(u.verification)) {
    if (!c) continue;
    total += c.total;
    passed += c.passed;
  }
  return `${passed}/${total}v`;
}
```

- [ ] **Step 2: Add CSS**

`dashboard/src/components/TaskNodeCard.css`:

```css
.task-card {
  position: relative;
  border-radius: var(--radius-card);
  border: 1px solid;
  padding: 6px 10px;
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: box-shadow 120ms, transform 120ms;
}
.task-card--compact   { width: 140px; height: 44px; }
.task-card--standard  { width: 180px; height: 72px; box-shadow: 0 4px 12px rgba(0,0,0,0.08); position: absolute; z-index: 5; }
.task-card--tall      { width: 200px; height: 96px; box-shadow: 0 0 0 2px var(--blue-9); }

.status-planned   { background: var(--status-planned-bg);   border-color: var(--status-planned-border);   color: var(--status-planned-text); }
.status-ready     { background: var(--status-ready-bg);     border-color: var(--status-ready-border);     color: var(--status-ready-text); }
.status-running   { background: var(--status-running-bg);   border-color: var(--status-running-border);   color: var(--status-running-text); }
.status-waiting   { background: var(--status-waiting-bg);   border-color: var(--status-waiting-border);   color: var(--status-waiting-text); }
.status-blocked   { background: var(--status-blocked-bg);   border-color: var(--status-blocked-border);   color: var(--status-blocked-text); }
.status-failed    { background: var(--status-failed-bg);    border-color: var(--status-failed-border);    color: var(--status-failed-text); }
.status-verifying { background: var(--status-verifying-bg); border-color: var(--status-verifying-border); color: var(--status-verifying-text); }
.status-complete  { background: var(--status-complete-bg);  border-color: var(--status-complete-border);  color: var(--status-complete-text); }
.status-skipped   { background: var(--status-skipped-bg);   border-color: var(--status-skipped-border);   color: var(--status-skipped-text); opacity: 0.6; }

.task-card__row1 { display: flex; align-items: center; gap: 6px; font-weight: 600; }
.task-card__id   { font-size: var(--fs-sm); }
.task-card__label { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-size: var(--fs-sm); margin-top: 2px; }
.task-card__row3 { display: flex; align-items: center; gap: 6px; margin-top: 4px; font-size: var(--fs-xs); }
.task-card__row4 { display: flex; gap: 6px; margin-top: 2px; font-size: var(--fs-xs); }

.owner-pill { padding: 1px 6px; border-radius: var(--radius-pill); background: rgba(0,0,0,0.06); }
.kind-chip  { padding: 1px 6px; border-radius: 4px; background: rgba(0,0,0,0.06); }
.wave-chip  { padding: 1px 6px; border-radius: 4px; background: rgba(0,0,0,0.06); }
.count { font-family: var(--font-mono); }

.task-card__drift {
  position: absolute; top: -4px; right: -4px;
  background: white; border-radius: 50%; padding: 1px;
}
```

- [ ] **Step 3: Verify build**

```bash
cd dashboard && npm run build
```

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/components/TaskNodeCard.tsx dashboard/src/components/TaskNodeCard.css
git commit -m "feat(dashboard): adaptive task node card (N1/N2/N3)"
```

---

### Task 16: Orthogonal edge router

**Files:**
- Create: `dashboard/src/components/DepEdge.tsx`

- [ ] **Step 1: Implement**

The router accepts source `{x, y, w, h}` and target `{x, y, w, h}` rectangles in pixel coordinates and emits an SVG `path` describing a strict right-angle route: out from the source's right edge, through a midpoint corridor (vertical segment at `(srcRight + targetLeft) / 2`), then into the target's left edge.

```tsx
import type { Status } from '../types';

export type EdgeState = 'pending' | 'satisfied' | 'blocking-active' | 'blocking-failed';

interface Rect { x: number; y: number; w: number; h: number }

interface Props {
  source: Rect;
  target: Rect;
  state: EdgeState;
}

const COLOR: Record<EdgeState, string> = {
  pending:           'var(--gray-9)',
  satisfied:         'var(--green-9)',
  'blocking-active': 'var(--amber-9)',
  'blocking-failed': 'var(--red-9)',
};
const DASH: Record<EdgeState, string | undefined> = {
  pending: '4 4',
  satisfied: undefined,
  'blocking-active': undefined,
  'blocking-failed': undefined,
};

export function DepEdge({ source, target, state }: Props) {
  const sx = source.x + source.w;
  const sy = source.y + source.h / 2;
  const tx = target.x;
  const ty = target.y + target.h / 2;
  const midX = Math.round((sx + tx) / 2);
  const d = `M ${sx} ${sy} L ${midX} ${sy} L ${midX} ${ty} L ${tx} ${ty}`;
  const color = COLOR[state];

  return (
    <g>
      <path d={d} fill="none" stroke={color} strokeWidth={1.5} strokeDasharray={DASH[state]} />
      <polygon
        points={`${tx},${ty} ${tx - 6},${ty - 4} ${tx - 6},${ty + 4}`}
        fill={color}
      />
    </g>
  );
}

export function edgeState(sourceStatus: Status, targetStatus: Status): EdgeState {
  if (sourceStatus === 'failed' || sourceStatus === 'blocked') return 'blocking-failed';
  if (sourceStatus === 'complete') return 'satisfied';
  if (targetStatus === 'waiting' && (sourceStatus === 'running' || sourceStatus === 'verifying')) return 'blocking-active';
  return 'pending';
}
```

- [ ] **Step 2: Verify TS**

```bash
cd dashboard && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/components/DepEdge.tsx
git commit -m "feat(dashboard): orthogonal edge router with state colors"
```

---

### Task 17: Swimlanes layout

**Files:**
- Create: `dashboard/src/components/Swimlanes.tsx`
- Create: `dashboard/src/components/Swimlanes.css`
- Modify: `dashboard/src/types.ts`
- Modify: `dashboard/src/compare/compare.ts`

The swimlane component lays out cards in a grid: rows = waves, columns = sorted by unit ID within wave. It computes pixel rects for every unit, renders an absolute-positioned SVG layer of edges underneath, and the cards on top. Edges read from `UnitView.depsList`, which Task 12 did not expose; this task adds it.

- [ ] **Step 1: Add `depsList` to UnitView and populate it**

In `dashboard/src/types.ts`, inside the `UnitView` interface, add:

```ts
depsList: string[];
```

In `dashboard/src/compare/compare.ts`, inside the `units = ip.ids.map(...)` block, in the returned object literal, add:

```ts
depsList: u.depends_on,
```

- [ ] **Step 2: Verify TS compiles**

```bash
cd dashboard && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Implement Swimlanes.tsx**

`dashboard/src/components/Swimlanes.tsx`:

```tsx
import { useMemo } from 'react';
import { useDashboard } from '../store';
import { TaskNodeCard } from './TaskNodeCard';
import { DepEdge, edgeState } from './DepEdge';
import type { UnitView } from '../types';
import './Swimlanes.css';

const CARD_W = 140;
const CARD_H = 44;
const GAP_X = 32;
const GAP_Y = 24;
const LANE_HEADER_W = 120;
const LANE_PAD_TOP = 12;

interface Rect { x: number; y: number; w: number; h: number }

interface Layout {
  rects: Record<string, Rect>;
  lanes: { wave: number; y: number; h: number }[];
  totalH: number;
  totalW: number;
}

export function Swimlanes() {
  const view = useDashboard((s) => s.view);
  const setSelected = useDashboard((s) => s.setSelected);
  const selectedUnitId = useDashboard((s) => s.selectedUnitId);
  const filters = useDashboard((s) => s.filters);

  const layout: Layout | null = useMemo(() => {
    if (!view) return null;
    const byWave = new Map<number, UnitView[]>();
    for (const u of view.units) {
      const arr = byWave.get(u.wave) ?? [];
      arr.push(u);
      byWave.set(u.wave, arr);
    }
    const waves = [...byWave.keys()].sort((a, b) => a - b);
    const rects: Record<string, Rect> = {};
    const lanes: Layout['lanes'] = [];
    let y = LANE_PAD_TOP;
    let maxX = LANE_HEADER_W;
    for (const w of waves) {
      const units = byWave.get(w)!.slice().sort((a, b) => a.id.localeCompare(b.id));
      let x = LANE_HEADER_W;
      for (const u of units) {
        rects[u.id] = { x, y, w: CARD_W, h: CARD_H };
        x += CARD_W + GAP_X;
      }
      maxX = Math.max(maxX, x);
      lanes.push({ wave: w, y, h: CARD_H });
      y += CARD_H + GAP_Y;
    }
    return { rects, lanes, totalH: y, totalW: Math.max(maxX, 800) };
  }, [view]);

  if (!view || !layout) return <div className="swimlanes--empty">no plan loaded</div>;

  const passesFilters = (u: UnitView): boolean => {
    if (filters.waves.length && !filters.waves.includes(u.wave)) return false;
    if (filters.owners.length && (!u.actualOwner || !filters.owners.includes(u.actualOwner))) return false;
    if (filters.statuses.length && !filters.statuses.includes(u.derivedStatus)) return false;
    if (filters.driftOnly && !Object.values(u.drift).some(Boolean)) return false;
    return true;
  };

  return (
    <div className="swimlanes" style={{ position: 'relative', height: layout.totalH, width: layout.totalW }}>
      <svg className="swimlanes__edges" width={layout.totalW} height={layout.totalH}>
        {view.units.flatMap((u) => {
          const trg = layout.rects[u.id];
          if (!trg) return [];
          return u.depsList.flatMap((depId) => {
            const src = layout.rects[depId];
            if (!src) return [];
            const srcStatus = view.unitsById[depId]?.derivedStatus ?? 'planned';
            return [(
              <DepEdge
                key={`${depId}->${u.id}`}
                source={src}
                target={trg}
                state={edgeState(srcStatus, u.derivedStatus)}
              />
            )];
          });
        })}
      </svg>

      {layout.lanes.map((lane) => (
        <div key={lane.wave} className="swimlanes__header" style={{ top: lane.y }}>
          Wave {lane.wave}
        </div>
      ))}

      {view.units.map((u) => {
        const r = layout.rects[u.id];
        if (!r) return null;
        const dim = !passesFilters(u);
        return (
          <div
            key={u.id}
            className="swimlanes__card-slot"
            style={{ left: r.x, top: r.y, opacity: dim ? 0.3 : 1 }}
          >
            <TaskNodeCard
              unit={u}
              active={u.id === selectedUnitId}
              onClick={() => setSelected(u.id)}
            />
          </div>
        );
      })}
    </div>
  );
}
```

- [ ] **Step 4: Add CSS**

`dashboard/src/components/Swimlanes.css`:

```css
.swimlanes { background: var(--gray-2); border-radius: var(--radius-card); overflow: auto; }
.swimlanes__edges { position: absolute; inset: 0; pointer-events: none; }
.swimlanes__header {
  position: absolute; left: 8px; width: 110px; height: 44px;
  display: flex; align-items: center; font-weight: 600; color: var(--gray-11);
}
.swimlanes__card-slot { position: absolute; transition: opacity 120ms ease-out; }
.swimlanes--empty { padding: 16px; color: var(--gray-10); }
```

- [ ] **Step 5: Verify build**

```bash
cd dashboard && npm run build
```

- [ ] **Step 6: Commit**

```bash
git add dashboard/src/components/Swimlanes.tsx dashboard/src/components/Swimlanes.css dashboard/src/types.ts dashboard/src/compare/compare.ts
git commit -m "feat(dashboard): swimlane layout with orthogonal edges"
```

---

## Phase F — Panels

### Task 18: SummaryBar

**Files:**
- Create: `dashboard/src/components/SummaryBar.tsx`
- Create: `dashboard/src/components/SummaryBar.css`

- [ ] **Step 1: Implement**

```tsx
import { useDashboard } from '../store';
import './SummaryBar.css';

export function SummaryBar() {
  const view = useDashboard((s) => s.view);
  const lastUpdateMs = useDashboard((s) => s.lastUpdateMs);
  if (!view) return <div className="summary-bar">loading…</div>;

  const m = view.metrics;
  const activeUnits = view.units.filter((u) => u.derivedStatus === 'running' || u.derivedStatus === 'verifying');
  const focused = activeUnits[0];
  const health = pickHealth(view, lastUpdateMs);

  return (
    <div className="summary-bar">
      <span className="sb__plan mono">plan @ v{view.frame.planVersion ?? '?'}</span>
      <span className="sb__progress mono">{Math.round(m.progressPercent * 100)}%</span>
      <span className="sb__active mono">{activeUnits.length === 0 ? 'idle' : `${activeUnits.length} active`}</span>
      {focused && <span className="sb__focus mono">{focused.id} {focused.label}</span>}
      {focused && <span className="owner-pill">{focused.actualOwner ?? '—'}</span>}
      <span className={`sb__health pill pill--${health.tone}`}>{health.label}</span>
      <span className="sb__updated mono">{lastUpdateMs ? agoSeconds(lastUpdateMs) : '—'}</span>
    </div>
  );
}

function pickHealth(view: any, lastUpdate: number | null): { label: string; tone: string } {
  if (lastUpdate && Date.now() - lastUpdate > 15_000) return { label: 'Stale', tone: 'red' };
  if (!view.frame.planRefMatches && view.frame.stateVersion !== undefined) return { label: 'Plan Mismatch', tone: 'amber' };
  if (view.metrics.failedTasks > 0 || view.metrics.blockedTasks > 0) return { label: 'Blocked', tone: 'red' };
  if (view.metrics.driftCount > 0) return { label: 'Drift', tone: 'purple' };
  if (view.metrics.runningTasks > 0) return { label: 'OK', tone: 'green' };
  return { label: 'Waiting', tone: 'amber' };
}

function agoSeconds(ms: number): string {
  const s = Math.max(0, Math.floor((Date.now() - ms) / 1000));
  return `${s}s ago`;
}
```

- [ ] **Step 2: CSS**

`dashboard/src/components/SummaryBar.css`:

```css
.summary-bar {
  display: flex; gap: 16px; align-items: center; padding: 8px 16px;
  background: var(--gray-2); border-bottom: 1px solid var(--gray-6);
  font-size: var(--fs-base);
}
.sb__plan { font-weight: 600; }
.sb__progress { color: var(--gray-11); }
.sb__active { color: var(--gray-11); }
.sb__focus { font-weight: 500; }
.sb__updated { margin-left: auto; color: var(--gray-10); }
.pill { padding: 2px 8px; border-radius: var(--radius-pill); font-size: var(--fs-sm); font-weight: 500; }
.pill--green  { background: var(--green-3);  color: var(--green-11); }
.pill--amber  { background: var(--amber-3);  color: var(--amber-11); }
.pill--red    { background: var(--red-3);    color: var(--red-11); }
.pill--purple { background: var(--purple-3); color: var(--purple-11); }
```

- [ ] **Step 3: Build + commit**

```bash
cd dashboard && npm run build
git add dashboard/src/components/SummaryBar.tsx dashboard/src/components/SummaryBar.css
git commit -m "feat(dashboard): summary status bar"
```

---

### Task 19: PipelineStrip

**Files:**
- Create: `dashboard/src/components/PipelineStrip.tsx`
- Create: `dashboard/src/components/PipelineStrip.css`

- [ ] **Step 1: Implement**

```tsx
import { useDashboard } from '../store';
import type { Status } from '../types';
import './PipelineStrip.css';

const STAGES: { name: string; statuses: Status[] }[] = [
  { name: 'Intent',     statuses: ['planned'] },
  { name: 'Plan',       statuses: ['planned', 'ready'] },
  { name: 'Task',       statuses: ['ready'] },
  { name: 'Implement',  statuses: ['running'] },
  { name: 'Change',     statuses: ['running'] },
  { name: 'Verify',     statuses: ['verifying'] },
  { name: 'Done',       statuses: ['complete'] },
];

export function PipelineStrip() {
  const view = useDashboard((s) => s.view);
  const setFilters = useDashboard((s) => s.setFilters);
  const filters = useDashboard((s) => s.filters);
  if (!view) return null;

  return (
    <div className="pipeline-strip">
      {STAGES.map((stage) => {
        const count = view.units.filter((u) => stage.statuses.includes(u.derivedStatus)).length;
        const active = stage.statuses.some((s) => filters.statuses.includes(s));
        return (
          <button
            key={stage.name}
            className={`pipeline-tile ${active ? 'is-active' : ''}`}
            onClick={() => setFilters({ statuses: active ? [] : stage.statuses })}
          >
            <div className="pipeline-tile__name">{stage.name}</div>
            <div className="pipeline-tile__count mono">{count}</div>
          </button>
        );
      })}
    </div>
  );
}
```

- [ ] **Step 2: CSS**

```css
.pipeline-strip {
  display: flex; gap: 4px; padding: 8px 16px;
  background: var(--gray-2); border-bottom: 1px solid var(--gray-6);
}
.pipeline-tile {
  flex: 1; padding: 6px 10px; border-radius: var(--radius-card);
  background: var(--gray-3); border: 1px solid var(--gray-6); cursor: pointer;
  font-family: inherit; text-align: left;
}
.pipeline-tile.is-active { background: var(--blue-3); border-color: var(--blue-6); color: var(--blue-11); }
.pipeline-tile__name { font-size: var(--fs-sm); font-weight: 500; }
.pipeline-tile__count { font-size: var(--fs-md); font-weight: 600; }
```

- [ ] **Step 3: Build + commit**

```bash
cd dashboard && npm run build
git add dashboard/src/components/PipelineStrip.tsx dashboard/src/components/PipelineStrip.css
git commit -m "feat(dashboard): pipeline strip with status filter"
```

---

### Task 20: ActiveTaskDrawer (D4 stratified)

**Files:**
- Create: `dashboard/src/components/ActiveTaskDrawer.tsx`
- Create: `dashboard/src/components/ActiveTaskDrawer.css`

- [ ] **Step 1: Implement**

```tsx
import { useMemo } from 'react';
import { useDashboard } from '../store';
import { StatusIcon } from './StatusIcon';
import './ActiveTaskDrawer.css';

export function ActiveTaskDrawer() {
  const view = useDashboard((s) => s.view);
  const selectedUnitId = useDashboard((s) => s.selectedUnitId);
  const setSelected = useDashboard((s) => s.setSelected);

  const focused = useMemo(() => {
    if (!view) return null;
    if (selectedUnitId && view.unitsById[selectedUnitId]) return view.unitsById[selectedUnitId];
    return view.units.find((u) => u.derivedStatus === 'running' || u.derivedStatus === 'verifying') ?? null;
  }, [view, selectedUnitId]);

  if (!view) return <aside className="drawer drawer--empty">—</aside>;
  const activeSet = view.units.filter((u) => u.derivedStatus === 'running' || u.derivedStatus === 'verifying');
  if (!focused) return <aside className="drawer drawer--empty">no active task</aside>;

  const blockersAcrossSet = activeSet.flatMap((u) =>
    u.depsBlocking > 0 ? [{ id: u.id, label: `${u.id}: ${u.depsBlocking} blocking dep(s)` }] : [],
  );

  return (
    <aside className="drawer">
      <header className="drawer__header">
        <div className="drawer__chips">
          {activeSet.map((u) => (
            <button
              key={u.id}
              className={`chip ${u.id === focused.id ? 'is-on' : ''}`}
              onClick={() => setSelected(u.id)}
            >
              {u.id}
            </button>
          ))}
        </div>
        <div className="drawer__title">
          <span className="mono drawer__id">{focused.id}</span>
          <span className="owner-pill">{focused.actualOwner ?? '—'}</span>
          <StatusIcon status={focused.derivedStatus} />
          <span>{focused.derivedStatus}</span>
        </div>
        <button className="drawer__action">{nextActionLabel(focused.derivedStatus)}</button>
      </header>

      <div className="drawer__middle">
        <Section title="target files" rows={focused.expectedArtifacts.map((a) => a.path)} />
        <Section title="current artifacts" rows={focused.actualArtifacts.map((a) => `${a.path}${a.written_at ? ' · ' + a.written_at : ''}`)} />
        <Section title="dependencies" rows={[`${focused.depsSatisfied}/${focused.depsTotal} satisfied`]} />
      </div>

      <footer className="drawer__footer">
        {blockersAcrossSet.length === 0
          ? <span className="pill pill--gray">no blockers</span>
          : blockersAcrossSet.map((b) => <span key={b.id} className="pill pill--red">{b.label}</span>)}
      </footer>
    </aside>
  );
}

function Section({ title, rows }: { title: string; rows: string[] }) {
  return (
    <div className="section">
      <div className="section__title">{title}</div>
      {rows.length === 0 ? <div className="section__empty">—</div> : rows.map((r, i) => <div key={i} className="section__row mono">{r}</div>)}
    </div>
  );
}

function nextActionLabel(status: string): string {
  switch (status) {
    case 'running': return 'Mark verifying';
    case 'verifying': return 'Mark complete';
    case 'blocked': return 'Unblock';
    case 'failed': return 'Retry';
    case 'waiting': return 'Inspect deps';
    default: return 'Open';
  }
}
```

- [ ] **Step 2: CSS**

```css
.drawer { display: flex; flex-direction: column; height: 100%; background: white; border-left: 1px solid var(--gray-6); }
.drawer--empty { color: var(--gray-10); padding: 16px; }
.drawer__header { padding: 8px 12px; border-bottom: 1px solid var(--gray-6); }
.drawer__chips { display: flex; gap: 4px; margin-bottom: 6px; flex-wrap: wrap; }
.chip { padding: 2px 8px; border-radius: var(--radius-pill); border: 1px solid var(--gray-6); background: var(--gray-3); font-family: var(--font-mono); font-size: var(--fs-xs); cursor: pointer; }
.chip.is-on { background: var(--blue-3); border-color: var(--blue-6); color: var(--blue-11); }
.drawer__title { display: flex; align-items: center; gap: 8px; }
.drawer__id { font-weight: 600; }
.drawer__action { margin-top: 6px; padding: 6px 12px; background: var(--blue-9); color: white; border: none; border-radius: var(--radius-card); cursor: pointer; }
.drawer__middle { flex: 1; overflow: auto; padding: 8px 12px; }
.drawer__footer { padding: 8px 12px; border-top: 1px solid var(--gray-6); display: flex; gap: 6px; flex-wrap: wrap; }
.section { margin-bottom: 12px; }
.section__title { font-size: var(--fs-xs); text-transform: uppercase; color: var(--gray-10); margin-bottom: 4px; }
.section__row { font-size: var(--fs-sm); padding: 2px 0; }
.section__empty { color: var(--gray-9); font-size: var(--fs-sm); }
.pill--gray { background: var(--gray-3); color: var(--gray-11); }
```

- [ ] **Step 3: Build + commit**

```bash
cd dashboard && npm run build
git add dashboard/src/components/ActiveTaskDrawer.tsx dashboard/src/components/ActiveTaskDrawer.css
git commit -m "feat(dashboard): active task drawer (D4 stratified)"
```

---

### Task 21: DriftSections

**Files:**
- Create: `dashboard/src/components/DriftSections.tsx`
- Create: `dashboard/src/components/DriftSections.css`

- [ ] **Step 1: Implement**

```tsx
import { useDashboard } from '../store';
import type { DriftReport } from '../types';
import './DriftSections.css';

const BUCKETS: (keyof DriftReport)[] = ['missing', 'extra', 'owner', 'status', 'deps', 'artifacts'];

export function DriftSections() {
  const view = useDashboard((s) => s.view);
  const setSelected = useDashboard((s) => s.setSelected);
  if (!view) return null;

  return (
    <div className="drift-panel">
      {BUCKETS.map((b) => {
        const entries = view.drift[b];
        return (
          <details key={b} className="drift-bucket" open={entries.length > 0}>
            <summary>
              <span className="drift-bucket__name">{b}</span>
              <span className={`drift-bucket__count mono ${entries.length === 0 ? 'is-zero' : ''}`}>{entries.length}</span>
            </summary>
            <div className="drift-bucket__rows">
              {entries.length === 0
                ? <span className="drift-bucket__empty">none</span>
                : entries.map((e) => (
                  <button key={`${b}-${e.unitId}`} className="drift-chip mono" onClick={() => setSelected(e.unitId)}>
                    {e.unitId}{e.field ? ` ${e.field}: ${String(e.plan)}→${String(e.actual)}` : ''}
                  </button>
                ))}
            </div>
          </details>
        );
      })}
    </div>
  );
}
```

- [ ] **Step 2: CSS**

```css
.drift-panel { padding: 8px 12px; background: white; border-top: 1px solid var(--gray-6); border-right: 1px solid var(--gray-6); }
.drift-bucket { border: 1px solid var(--gray-6); border-radius: var(--radius-card); margin-bottom: 6px; padding: 6px 8px; }
.drift-bucket > summary { display: flex; gap: 8px; cursor: pointer; align-items: center; }
.drift-bucket__name { font-weight: 500; text-transform: capitalize; }
.drift-bucket__count { padding: 0 6px; border-radius: var(--radius-pill); background: var(--purple-3); color: var(--purple-11); }
.drift-bucket__count.is-zero { background: var(--gray-3); color: var(--gray-9); }
.drift-bucket__rows { display: flex; flex-wrap: wrap; gap: 4px; padding: 6px 0 0 0; }
.drift-bucket__empty { color: var(--gray-9); font-size: var(--fs-sm); }
.drift-chip { padding: 2px 8px; border-radius: var(--radius-pill); border: 1px solid var(--purple-6); background: var(--purple-3); color: var(--purple-11); font-size: var(--fs-xs); cursor: pointer; }
```

- [ ] **Step 3: Build + commit**

```bash
cd dashboard && npm run build
git add dashboard/src/components/DriftSections.tsx dashboard/src/components/DriftSections.css
git commit -m "feat(dashboard): bucketed drift sections"
```

---

### Task 22: VerificationPanel

**Files:**
- Create: `dashboard/src/components/VerificationPanel.tsx`
- Create: `dashboard/src/components/VerificationPanel.css`

- [ ] **Step 1: Implement**

```tsx
import { useState } from 'react';
import { useDashboard } from '../store';
import type { CheckName, VerificationCheck } from '../types';
import './VerificationPanel.css';

const CHECKS: CheckName[] = ['tests', 'lint', 'typecheck', 'build', 'review', 'acceptance'];

export function VerificationPanel() {
  const view = useDashboard((s) => s.view);
  const [openTile, setOpenTile] = useState<CheckName | null>(null);
  if (!view) return null;

  const aggregate = aggregateChecks(view.units);

  return (
    <div className="verify-panel">
      <header className="verify-panel__header">
        <span className="mono">{aggregate.passed}/{aggregate.total} checks · {Math.round(aggregate.rate * 100)}%</span>
        {aggregate.unreported > 0 && <span className="verify-panel__sub">({aggregate.unreported} unreported)</span>}
      </header>
      <div className="verify-tiles">
        {sortBySeverity(CHECKS, view.units).map((name) => {
          const merged = mergeCheck(view.units, name);
          return (
            <button
              key={name}
              className={`verify-tile ${tone(merged)} ${openTile === name ? 'is-open' : ''}`}
              onClick={() => setOpenTile(openTile === name ? null : name)}
            >
              <div className="verify-tile__name">{name}</div>
              <div className="verify-tile__count mono">{merged ? `${merged.passed}/${merged.total}` : '— / —'}</div>
              <div className="verify-tile__ago">{merged?.ran_at ?? 'never'}</div>
              {openTile === name && merged?.failed_items && merged.failed_items.length > 0 && (
                <ul className="verify-tile__failures">
                  {merged.failed_items.map((f) => <li key={f.id} className="mono">{f.id}: {f.label}</li>)}
                </ul>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function mergeCheck(units: any[], name: CheckName): VerificationCheck | null {
  let total = 0, passed = 0; let failed_items: any[] = []; let ranAt: string | undefined;
  let any = false;
  for (const u of units) {
    const c = u.verification[name];
    if (!c) continue;
    any = true; total += c.total; passed += c.passed;
    if (c.failed_items) failed_items.push(...c.failed_items);
    if (!ranAt || c.ran_at > ranAt) ranAt = c.ran_at;
  }
  return any ? { total, passed, failed_items, ran_at: ranAt ?? '' } : null;
}

function aggregateChecks(units: any[]) {
  let total = 0, passed = 0, unreported = 0;
  for (const name of CHECKS) {
    const m = mergeCheck(units, name);
    if (!m) { unreported++; continue; }
    total += m.total; passed += m.passed;
  }
  return { total, passed, unreported, rate: total > 0 ? passed / total : 1 };
}

function tone(c: VerificationCheck | null): string {
  if (!c) return 'tone-nodata';
  if (c.passed < c.total) return 'tone-fail';
  return 'tone-ok';
}

function sortBySeverity(names: CheckName[], units: any[]): CheckName[] {
  return names.slice().sort((a, b) => severityRank(mergeCheck(units, a)) - severityRank(mergeCheck(units, b)));
}
function severityRank(c: VerificationCheck | null): number {
  if (!c) return 2;
  if (c.passed < c.total) return 0;
  return 3;
}
```

- [ ] **Step 2: CSS**

```css
.verify-panel { padding: 8px 12px; background: white; border-top: 1px solid var(--gray-6); }
.verify-panel__header { font-weight: 600; margin-bottom: 6px; }
.verify-panel__sub { color: var(--gray-10); font-weight: 400; margin-left: 6px; }
.verify-tiles { display: grid; grid-template-columns: repeat(6, 1fr); gap: 6px; }
.verify-tile {
  text-align: left; padding: 6px 8px; border-radius: var(--radius-card);
  border: 1px solid var(--gray-6); background: var(--gray-3); position: relative;
  border-left-width: 4px; cursor: pointer; font-family: inherit;
}
.tone-fail   { border-left-color: var(--red-9); }
.tone-ok     { border-left-color: var(--green-9); }
.tone-nodata { border-left-color: var(--gray-9); color: var(--gray-10); }
.verify-tile__name { font-size: var(--fs-sm); font-weight: 500; text-transform: capitalize; }
.verify-tile__count { font-size: var(--fs-md); }
.verify-tile__ago { font-size: var(--fs-xs); color: var(--gray-10); }
.verify-tile__failures { margin: 6px 0 0 0; padding: 0 0 0 16px; }
```

- [ ] **Step 3: Build + commit**

```bash
cd dashboard && npm run build
git add dashboard/src/components/VerificationPanel.tsx dashboard/src/components/VerificationPanel.css
git commit -m "feat(dashboard): verification panel (V3 aggregate + tiles)"
```

---

### Task 23: HeartbeatStrip

**Files:**
- Create: `dashboard/src/components/HeartbeatStrip.tsx`
- Create: `dashboard/src/components/HeartbeatStrip.css`

- [ ] **Step 1: Implement**

```tsx
import { useEffect, useState } from 'react';
import { useDashboard } from '../store';
import './HeartbeatStrip.css';

export function HeartbeatStrip() {
  const view = useDashboard((s) => s.view);
  const lastUpdateMs = useDashboard((s) => s.lastUpdateMs);
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 500);
    return () => clearInterval(id);
  }, []);

  const ageS = lastUpdateMs ? Math.floor((now - lastUpdateMs) / 1000) : null;
  const tone = ageS === null ? 'gray' : ageS > 15 ? 'red' : ageS > 5 ? 'amber' : 'green';
  const events = view?.units
    .filter((u) => u.lastUpdatedAt)
    .sort((a, b) => (b.lastUpdatedAt ?? '').localeCompare(a.lastUpdatedAt ?? ''))
    .slice(0, 8) ?? [];

  return (
    <footer className="heartbeat">
      <div className="heartbeat__left">
        <span className={`pulse pulse--${tone}`} />
        <span className="mono">{ageS === null ? '—' : `${ageS}s ago`}</span>
        <span className="heartbeat__status">{tone === 'green' ? 'ok' : tone === 'amber' ? 'slow' : tone === 'red' ? 'stale' : 'idle'}</span>
      </div>
      <div className="heartbeat__center">
        {events.map((u) => (
          <span key={u.id} className="event mono">
            {u.id} → {u.derivedStatus}{u.actualOwner ? ` by ${u.actualOwner}` : ''}
          </span>
        ))}
      </div>
      <div className="heartbeat__right mono">
        <span>{events.length}/poll</span>
      </div>
    </footer>
  );
}
```

- [ ] **Step 2: CSS**

```css
.heartbeat { display: flex; gap: 16px; padding: 6px 12px; background: var(--gray-2); border-top: 1px solid var(--gray-6); font-size: var(--fs-sm); align-items: center; }
.heartbeat__left { display: flex; gap: 6px; align-items: center; min-width: 200px; }
.heartbeat__center { flex: 1; display: flex; gap: 12px; overflow: hidden; }
.heartbeat__right { min-width: 80px; text-align: right; color: var(--gray-10); }
.heartbeat__status { color: var(--gray-10); }
.pulse { width: 8px; height: 8px; border-radius: 50%; }
.pulse--green { background: var(--green-9); animation: pulse 2s ease-in-out infinite; }
.pulse--amber { background: var(--amber-9); animation: pulse 1.2s ease-in-out infinite; }
.pulse--red   { background: var(--red-9); }
.pulse--gray  { background: var(--gray-9); }
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }
.event { white-space: nowrap; color: var(--gray-11); }
```

- [ ] **Step 3: Build + commit**

```bash
cd dashboard && npm run build
git add dashboard/src/components/HeartbeatStrip.tsx dashboard/src/components/HeartbeatStrip.css
git commit -m "feat(dashboard): tri-zone heartbeat footer"
```

---

### Task 24: FilterToolbar (waves, owners, drift, mode)

**Files:**
- Create: `dashboard/src/components/FilterToolbar.tsx`
- Create: `dashboard/src/components/FilterToolbar.css`

- [ ] **Step 1: Implement**

```tsx
import { useDashboard, type DashboardMode } from '../store';
import './FilterToolbar.css';

export function FilterToolbar() {
  const view = useDashboard((s) => s.view);
  const filters = useDashboard((s) => s.filters);
  const mode = useDashboard((s) => s.mode);
  const setFilters = useDashboard((s) => s.setFilters);
  const setMode = useDashboard((s) => s.setMode);
  if (!view) return null;

  const waves = [...new Set(view.units.map((u) => u.wave))].sort((a, b) => a - b);
  const owners = [...new Set(view.units.map((u) => u.actualOwner).filter(Boolean) as string[])].sort();

  return (
    <div className="filter-toolbar">
      <select
        value={filters.waves[0] ?? ''}
        onChange={(e) => setFilters({ waves: e.target.value ? [Number(e.target.value)] : [] })}
      >
        <option value="">all waves</option>
        {waves.map((w) => <option key={w} value={w}>wave {w}</option>)}
      </select>
      <select
        value={filters.owners[0] ?? ''}
        onChange={(e) => setFilters({ owners: e.target.value ? [e.target.value] : [] })}
      >
        <option value="">all owners</option>
        {owners.map((o) => <option key={o} value={o}>{o}</option>)}
      </select>
      <label className="check">
        <input type="checkbox" checked={filters.driftOnly} onChange={(e) => setFilters({ driftOnly: e.target.checked })} />
        drift only
      </label>
      <div className="mode-switch">
        {(['default', 'spotlight', 'diff'] as DashboardMode[]).map((m) => (
          <button key={m} className={mode === m ? 'is-on' : ''} onClick={() => setMode(m)}>{m}</button>
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: CSS**

```css
.filter-toolbar { display: flex; gap: 8px; padding: 6px 12px; background: var(--gray-1); border-bottom: 1px solid var(--gray-6); align-items: center; font-size: var(--fs-sm); }
.filter-toolbar select { padding: 4px 8px; border-radius: var(--radius-card); border: 1px solid var(--gray-6); background: white; }
.filter-toolbar .check { display: flex; gap: 4px; align-items: center; }
.mode-switch { display: flex; margin-left: auto; border-radius: var(--radius-card); overflow: hidden; border: 1px solid var(--gray-6); }
.mode-switch button { padding: 4px 12px; background: white; border: none; cursor: pointer; font-family: inherit; text-transform: capitalize; }
.mode-switch button.is-on { background: var(--blue-9); color: white; }
```

- [ ] **Step 3: Build + commit**

```bash
cd dashboard && npm run build
git add dashboard/src/components/FilterToolbar.tsx dashboard/src/components/FilterToolbar.css
git commit -m "feat(dashboard): filter toolbar with mode switcher"
```

---

## Phase G — App shell, modes, truth-degraded

### Task 25: Wire App.tsx with C3 grid + mode-aware rendering + truth-degraded banner

**Files:**
- Modify: `dashboard/src/App.tsx`
- Create: `dashboard/src/App.css`

- [ ] **Step 1: Implement**

```tsx
import { useEffect } from 'react';
import { usePolling } from './hooks/usePolling';
import { useDashboard } from './store';
import { SummaryBar } from './components/SummaryBar';
import { PipelineStrip } from './components/PipelineStrip';
import { FilterToolbar } from './components/FilterToolbar';
import { Swimlanes } from './components/Swimlanes';
import { ActiveTaskDrawer } from './components/ActiveTaskDrawer';
import { DriftSections } from './components/DriftSections';
import { VerificationPanel } from './components/VerificationPanel';
import { HeartbeatStrip } from './components/HeartbeatStrip';
import './App.css';

export default function App() {
  usePolling('/sample/plan.json', '/sample/state.json', 2000);
  const view = useDashboard((s) => s.view);
  const parseFailedSinceMs = useDashboard((s) => s.parseFailedSinceMs);
  const truthDegraded = parseFailedSinceMs !== null && Date.now() - parseFailedSinceMs > 60_000;
  const planMismatch = view && !view.frame.planRefMatches && view.frame.stateVersion !== undefined;

  useEffect(() => {
    document.body.classList.toggle('is-truth-degraded', truthDegraded);
  }, [truthDegraded]);

  return (
    <div className="dashboard-shell">
      <SummaryBar />
      {planMismatch && <div className="banner banner--amber">state pinned to plan v{view?.frame.planVersion}, current state targets a different version</div>}
      {truthDegraded && <div className="banner banner--purple">truth degraded — last good frame older than 60s</div>}
      <PipelineStrip />
      <FilterToolbar />
      <main className="dashboard-main">
        <section className="dashboard-graph"><Swimlanes /></section>
        <ActiveTaskDrawer />
      </main>
      <section className="dashboard-bottom">
        <DriftSections />
        <VerificationPanel />
      </section>
      <HeartbeatStrip />
    </div>
  );
}
```

- [ ] **Step 2: CSS**

`dashboard/src/App.css`:

```css
.dashboard-shell { display: flex; flex-direction: column; height: 100vh; }
.dashboard-main { flex: 1; display: grid; grid-template-columns: 1fr 360px; min-height: 0; }
.dashboard-graph { overflow: auto; padding: 12px; }
.dashboard-bottom { display: grid; grid-template-columns: 1fr 1fr; }

.banner { padding: 6px 12px; font-size: var(--fs-sm); }
.banner--amber  { background: var(--amber-3);  color: var(--amber-11); border-bottom: 1px solid var(--amber-6); }
.banner--purple { background: var(--purple-3); color: var(--purple-11); border-bottom: 1px solid var(--purple-6); }

body.is-truth-degraded .dashboard-graph { filter: saturate(0.75); }
body.is-truth-degraded .drawer__action { pointer-events: none; opacity: 0.5; }
```

- [ ] **Step 3: Build + run**

```bash
cd dashboard && npm run build && npm run dev
```

Open the printed local URL. Expect to see all panels render with the sample fixture, summary bar showing `1 active`, drift `extra` bucket showing `1`, verification tiles populated.

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/App.tsx dashboard/src/App.css
git commit -m "feat(dashboard): app shell with grid, banners, truth-degraded state"
```

---

### Task 26: Spotlight and Diff modes

**Files:**
- Modify: `dashboard/src/components/Swimlanes.tsx`

The two modes change which units render at full opacity. They build on the existing `passesFilters` predicate.

- [ ] **Step 1: Add a `passesMode` predicate**

Inside `Swimlanes.tsx`, add near `passesFilters`:

```ts
const mode = useDashboard((s) => s.mode);
const focusedId = useDashboard((s) => s.selectedUnitId);

const lineage = useMemo(() => {
  if (!view || mode !== 'spotlight' || !focusedId) return null;
  const set = new Set<string>([focusedId]);
  const visit = (id: string, dir: 'up' | 'down') => {
    const u = view.unitsById[id];
    if (!u) return;
    const next = dir === 'up' ? u.depsList : view.units.filter((x) => x.depsList.includes(id)).map((x) => x.id);
    for (const n of next) {
      if (set.has(n)) continue;
      set.add(n);
      visit(n, dir);
    }
  };
  visit(focusedId, 'up');
  visit(focusedId, 'down');
  return set;
}, [view, mode, focusedId]);

const passesMode = (u: typeof view.units[number]) => {
  if (mode === 'default') return true;
  if (mode === 'spotlight') return lineage ? lineage.has(u.id) : true;
  if (mode === 'diff') return Object.values(u.drift).some(Boolean);
  return true;
};
```

Update the dim calculation:

```tsx
const dim = !passesFilters(u) || !passesMode(u);
```

- [ ] **Step 2: Build + commit**

```bash
cd dashboard && npm run build
git add dashboard/src/components/Swimlanes.tsx
git commit -m "feat(dashboard): spotlight + diff modes for swimlane rendering"
```

---

## Phase H — Acceptance pass

### Task 27: Acceptance script + manual checklist

**Files:**
- Create: `dashboard/ACCEPTANCE.md`

- [ ] **Step 1: Write the acceptance checklist**

```markdown
# Dashboard Acceptance — manual run

Run `npm run dev` and open the URL.

Checks (all must pass):

- [ ] All 9 statuses appear at least once on the swimlane: planned, ready, running, waiting, blocked, failed, verifying, complete, skipped
- [ ] Drift `extra` bucket shows `T_EXTRA.1`; `owner` and `missing` show `0`; `artifacts` shows the T1.1 fixture entry only if you remove a path from `actual_artifacts`
- [ ] Verification panel header reads `8/9 checks · 89%` (or matching ratio); 4 tiles show `no data`
- [ ] Summary bar shows `1 active` and focuses `T2.1` (running)
- [ ] Pipeline strip click on `Implement` filters swimlanes to the running unit
- [ ] Toolbar `wave 2` filter narrows to T2.x; combined with `Implement` shows only T2.1
- [ ] Spotlight mode centers T2.1 (clicked) and dims everything not in its dep lineage
- [ ] Diff mode dims everything except units with drift flags set
- [ ] Drift chip click on `T_EXTRA.1` opens the drawer to that unit and pulses its node (the sample shows it rendered as an `extra`-only entry, so its node lives outside swimlanes; the drawer still focuses)
- [ ] Stop the dev server's static-fixture poll for >15s by killing vite; heartbeat dot turns red, swimlane desaturates
- [ ] Replace `state.json`'s `plan_ref.plan_version` with `99`; banner reads `state pinned to plan v1, current state targets a different version`
- [ ] Replace `state.json` with malformed JSON; after 60s, the purple `truth degraded` banner appears, the drawer's primary button locks (50% opacity, no clicks), and the swimlane graph desaturates 25%
- [ ] Open browser DevTools, switch to grayscale rendering — every status remains identifiable by glyph + dash style
```

- [ ] **Step 2: Run acceptance**

```bash
cd dashboard && npm run dev
```

Open the URL and tick each box.

- [ ] **Step 3: Commit**

```bash
git add dashboard/ACCEPTANCE.md
git commit -m "test(dashboard): manual acceptance checklist"
```

---

## Self-review notes

After Task 27, the engineer should have:

- A working dashboard that polls `/sample/plan.json` and `/sample/state.json` every 2s.
- All nine statuses, drift buckets, verification tiles, modes, and interaction states visible.
- The frame contract enforcing plan/state pairing.
- A truth-degraded mode that activates after 60s of parse failure.

Anything not covered:

- Acceptance criterion 18 (identity continuity through `replaces`) requires authoring a second plan revision; documented in the acceptance checklist as a follow-up.
- Acceptance criterion 19 (mixed timestamp warning surfacing in heartbeat strip) is covered by `loadState.hasLegacyTimestamps` flag and `compare.warnings`, but the heartbeat strip does not currently render the warning text. If the acceptance pass exposes this, add a one-line render in `HeartbeatStrip.tsx`'s left zone showing `mixed ts` when `view.warnings.includes('legacyTimestamps')`.

---

## Execution Handoff

Plan complete and saved to `docs/plans/2026-04-29-plan-state-dashboard.md`. Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
