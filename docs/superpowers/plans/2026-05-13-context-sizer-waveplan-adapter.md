# Context Sizer Waveplan Adapter Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the missing Waveplan-native adapter layer so context estimation can run directly against `*-execution-waves.json` tasks and units without hand-authoring `ContextCandidate` JSON.

**Parent FP Issue:** `FP-pkymdlbv` — Contextsize: native Waveplan adapter and CLI selectors

**Child FP Issues:**
- `FP-rrxobrej` — CSIZE-ADAPTER-01: Waveplan task/unit -> ContextCandidate mapping
- `FP-ljdtcbbi` — CSIZE-ADAPTER-02: waveplan-cli native `--plan/--task/--unit` support
- `FP-kaxwxxpx` — CSIZE-ADAPTER-03: adapter e2e verification and docs cutover

**Spec:** [2026-05-13-context-sizer-design.md](/Users/darkbit1001/workspace/waveplan-mcp/docs/superpowers/specs/2026-05-13-context-sizer-design.md:290)

**Architecture:** Keep `internal/contextsize` as the source-agnostic estimator. Add a Waveplan-specific adapter layer that decodes execution-waves JSON and maps tasks or units into `ContextCandidate`. Extend `waveplan-cli context estimate` to call that adapter when `--plan` is supplied. Keep raw `--candidate` mode for advanced/manual use.

**Files:**
- Create: `internal/contextsize/adapter/waveplan.go`
- Create: `internal/contextsize/adapter/waveplan_test.go`
- Modify: `waveplan-cli`
- Modify: `docs/superpowers/specs/2026-05-13-context-sizer-design.md`

---

### Task 1: Add Waveplan adapter mapping

**FP Ref:** `FP-rrxobrej`

- [ ] **Step 1: Write adapter tests first**

Cover:
- task-level conversion from execution-waves JSON
- unit-level conversion from execution-waves JSON
- `doc_refs` resolution through `doc_index`
- deterministic rejection for unknown task/unit IDs
- deduped, sorted `referenced_files`
- task dependency collapse from unit-level edges

- [ ] **Step 2: Implement adapter conversion**

Add adapter helpers with this surface:

```go
func FromWaveplanTask(planPath string, taskID string) (contextsize.ContextCandidate, error)
func FromWaveplanUnit(planPath string, unitID string) (contextsize.ContextCandidate, error)
```

Mapping rules:
- task candidate uses union of task `files` and resolved task `doc_refs`
- unit candidate uses resolved unit `doc_refs`
- `description` remains empty for deterministic V1
- `source` is always `"waveplan"`

- [ ] **Step 3: Verify adapter tests**

Run:

```bash
go test ./internal/contextsize/adapter -v
```

---

### Task 2: Extend `waveplan-cli context estimate`

**FP Ref:** `FP-ljdtcbbi`

- [ ] **Step 1: Add native selector flags**

Extend the wrapper to support:

```bash
python waveplan-cli context estimate --plan <waves.json> --task T1
python waveplan-cli context estimate --plan <waves.json> --unit T1.1
```

Retain:

```bash
python waveplan-cli context estimate --candidate issue.json
```

- [ ] **Step 2: Enforce deterministic argument validation**

Reject:
- `--candidate` together with `--plan`
- `--task` together with `--unit`
- `--plan` without a selector
- selector without `--plan`

- [ ] **Step 3: Route native plan input through the adapter**

The wrapper should:
1. resolve the plan path
2. build a `ContextCandidate` internally from task/unit selection
3. invoke `contextsize`
4. print the same `ContextEstimate` JSON

- [ ] **Step 4: Verify both paths**

Run:

```bash
python waveplan-cli context estimate --candidate /tmp/test-candidate.json
python waveplan-cli context estimate --plan docs/plans/<plan>-execution-waves.json --task T1
python waveplan-cli context estimate --plan docs/plans/<plan>-execution-waves.json --unit T1.1
```

---

### Task 3: End-to-end verification and docs cutover

**FP Ref:** `FP-kaxwxxpx`

- [ ] **Step 1: Add fixture-backed end-to-end tests**

Cover:
- valid task estimate from real execution-waves JSON
- valid unit estimate from real execution-waves JSON
- missing referenced files surfaced as `missing_files`
- invalid selector combinations return exit code `2`

- [ ] **Step 2: Update docs**

Make the normal usage path explicit:
- Waveplan users should use `--plan` with `--task` or `--unit`
- raw `ContextCandidate` JSON is advanced/manual mode

- [ ] **Step 3: Verify install path**

Run:

```bash
make install-contextsize
python waveplan-cli context estimate --plan docs/plans/<plan>-execution-waves.json --task T1
```

Expected:
- installed `contextsize` matches repo build
- wrapper uses native plan input without requiring hand-authored candidate JSON

---

## Completion Criteria

- `waveplan-cli context estimate` supports native Waveplan plan input
- `ContextCandidate` remains an internal adapter boundary for normal Waveplan usage
- adapter mapping is deterministic and covered by tests
- docs no longer imply that hand-authored candidate JSON is the normal workflow
