# Context Sizer Design

**Date:** 2026-05-13
**Status:** Draft

---

## Overview

A deterministic tool that estimates the context footprint of an issue candidate — the tokens an LLM would need to read to understand and implement the change. The output guides split/merge decisions without relying on LLM intuition.

```
ContextCandidate + Budget → Estimator.Estimate() → ContextEstimate
```

The core package is source-agnostic. Adapters convert Waveplan units or FP issues into `ContextCandidate`. The CLI provides a scriptable entrypoint. An MCP tool (`waveplan_context_estimate`) is implemented and documented in `2026-05-13-contextsizer-mcp-integration.md`.

---

## File Structure

```
cmd/contextsize/main.go        — Go CLI binary
internal/contextsize/
  types.go                     — ContextCandidate, Budget, ContextEstimate, SectionRef
  estimate.go                  — Estimator, Estimate()
  json.go                      — DecodeCandidateJSON, EncodeEstimateJSON
  estimate_test.go             — Unit tests for estimation logic
  json_test.go                 — Round-trip and validation tests
```

No external dependencies beyond the Go standard library.

---

## Types

### ContextCandidate (input)

```go
type ContextCandidate struct {
    ID                 string         `json:"id"`
    Title              string         `json:"title"`
    Description        string         `json:"description"`
    ReferencedFiles    []string       `json:"referenced_files"`
    ReferencedSections []SectionRef   `json:"referenced_sections"`
    DependsOn          []string       `json:"depends_on"`
    Source             string         `json:"source"`
}

type SectionRef struct {
    Path    string `json:"path"`
    Heading string `json:"heading"`
}
```

Source-agnostic. Adapters convert from Waveplan `PlanUnit`, FP issues, or hand-authored JSON.

### Budget

```go
type Budget struct {
    MinTokens int
    MaxTokens int
}
```

No defaults in the core package. CLI defaults to `64000:192000`.

### ContextEstimate (output)

```go
type ContextEstimate struct {
    EstimatedTokens   int          `json:"estimated_tokens"`
    BudgetMin         int          `json:"budget_min"`
    BudgetMax         int          `json:"budget_max"`
    Fit               string       `json:"fit"`                 // "within", "over", "under"
    Confidence        string       `json:"confidence"`          // "high", "medium", "low"
    Drivers           []string     `json:"drivers"`
    Recommendation    string       `json:"recommendation"`      // "keep", "split", "merge_candidate"
    MissingFiles      []string     `json:"missing_files"`
    MissingSections   []SectionRef `json:"missing_sections"`
    UnknownFiles      []string     `json:"unknown_files"`
    SplitCandidates   []string     `json:"split_candidates"`
    MergeCandidates   []string     `json:"merge_candidates"`
}
```

`SplitCandidates` and `MergeCandidates` are human-readable hints, not structured sub-issues.

---

## Estimator API

```go
type Estimator struct {
    BaseDir string // root for resolving relative file paths; empty = cwd
}

func (e *Estimator) Estimate(c ContextCandidate, budget Budget) (ContextEstimate, error)
```

Returns `(estimate, nil)` on success. Returns `(zero, err)` only for invalid inputs (e.g. zero/negative budget) or IO conditions that prevent deterministic behavior (e.g. unreadable directory). A sparse candidate with no signal returns a valid estimate with low confidence, not an error.

---

## Estimation Algorithm

### 1. Token Estimation

For each referenced file:
- Read file bytes
- Apply ratio based on extension:
  - Code files (`.go`, `.json`, `.yaml`, `.yml`, `.toml`, `.proto`, `.html`, `.css`, `.js`, `.ts`, `.tsx`, `.py`, `.sh`, `.sql`): `bytes / 3`
  - Prose files (`.md`, `.txt`, `.rst`): `bytes / 4`
  - Unknown extension: `bytes / 3` (conservative, assume code-like density)
- For description prose: `len([]rune(description)) / 4`
- For referenced sections: find heading in file, count bytes from heading to next heading at same-or-lower level or EOF, apply prose ratio (`bytes / 4`)

### 2. Dependency Edges

For each referenced `.go` file:
- Parse `import` blocks
- Count imports matching the local module path (local packages)
- Sum unique local import paths across all files

### 3. Fit Classification

- `estimated_tokens >= min && estimated_tokens <= max` → `"within"`
- `estimated_tokens > max` → `"over"`
- `estimated_tokens < min` → `"under"`

### 4. Confidence

Start at `"high"`. Downgrade for concrete uncertainty:

| Condition | Effect |
|-----------|--------|
| Any referenced file missing | → `"low"` |
| Any referenced file > 50k bytes | Downgrade one level |
| Referenced section heading not found | Downgrade one level |
| File with unknown extension | Downgrade one level |

No downgrade for "no Go files referenced" — docs-only tasks are not penalized.

### 5. Recommendation

- `"over"` → `"split"`
- `"under"` AND `estimated_tokens < min * 0.3` → `"merge_candidate"`
- Otherwise → `"keep"`

### 6. Drivers

Human-readable strings explaining the estimate:

- `"7 referenced files"`
- `"2 imported local packages"`
- `"description 320 chars"`
- `"section heading not found: docs/x.md#Foo"`
- `"unknown extension: foo.xyz (1.2k bytes)"`

---

## Confidence Downgrade Examples

| Scenario | Starting | Result |
|----------|----------|--------|
| 100k `.go` file (single downgrade) | high | medium |
| 100k `.go` file + missing file | high | low (capped) |
| 100k `.go` file + heading not found | high | low (two downgrades) |
| 5 files, all < 50k, all found | high | high |

---

## JSON Handling

```go
func DecodeCandidateJSON(data []byte) (ContextCandidate, error)
func EncodeEstimateJSON(e ContextEstimate) ([]byte, error)
```

`DecodeCandidateJSON` uses strict decoding (rejects unknown fields). `EncodeEstimateJSON` produces deterministic JSON with sorted keys where applicable.

---

## CLI

### Go Binary

```
cmd/contextsize/main.go
```

```bash
go run ./cmd/contextsize \
  --candidate issue.json \
  --budget 64000:192000 \
  --base-dir /path/to/repo
```

Arguments:
- `--candidate` (required): path to ContextCandidate JSON file
- `--budget` (optional): `min:max` in tokens. Default `64000:192000`
- `--base-dir` (optional): root for resolving referenced file paths. Defaults to cwd

The binary accepts an optional leading `estimate` alias for compatibility:

```bash
go run ./cmd/contextsize estimate --candidate issue.json
```

Exit codes: 0 = success, 2 = invalid input, 3 = IO error.

### Python Wrapper

`waveplan-cli context estimate` delegates to the Go binary the same way `swim` commands delegate to `cmd/swim-*`. It resolves the binary via `_resolve_local_tool("contextsize")` or falls back to `go run ./cmd/contextsize`.

---

## Test Cases

| Test | Input | Expected |
|------|-------|----------|
| No signal | no files, no sections, no description | 0 tokens, fit "under", rec "merge_candidate", conf "low" |
| Small Go file | 1k bytes `.go` | ~333 tokens, fit "under", conf "high" |
| Large Go file | 100k bytes `.go` | ~33k tokens, fit "under", conf "medium" (downgraded from high) |
| Exceeds budget | 200k bytes total | ~66k tokens, fit "over", rec "split" |
| Missing file | references `nonexistent.go` | estimate from existing files, conf "low", missing_files includes it |
| Unknown extension | `foo.xyz` (5k bytes) | ~1666 tokens, conf downgraded, unknown_files includes it |
| Heading not found | heading "Foo" not in `docs/x.md` | 0 section tokens, conf downgraded, missing_sections includes it |
| Mixed extensions | `.go` (10k) + `.md` (10k) + `.xyz` (5k) | ~4516 tokens, conf downgraded (unknown ext) |
| Under budget threshold | 5k total tokens, min=64k | fit "under", rec "merge_candidate" (5k < 64k * 0.3 = 19.2k) |
| Within budget | 100k tokens, budget 64k:192k | fit "within", rec "keep" |

---

## Output Example

Candidate touching 7 files, 2 local imports, within budget:

```json
{
  "estimated_tokens": 91000,
  "budget_min": 64000,
  "budget_max": 192000,
  "fit": "within",
  "confidence": "high",
  "drivers": [
    "7 referenced files",
    "2 imported local packages",
    "description 320 chars"
  ],
  "recommendation": "keep",
  "missing_files": [],
  "missing_sections": [],
  "unknown_files": [],
  "split_candidates": [],
  "merge_candidates": []
}
```

Candidate exceeding budget with issues:

```json
{
  "estimated_tokens": 238000,
  "budget_min": 64000,
  "budget_max": 192000,
  "fit": "over",
  "confidence": "medium",
  "drivers": [
    "12 referenced files",
    "5 imported local packages",
    "section heading not found: docs/arch.md#Deployment",
    "unknown extension: build.rs (2.1k bytes)"
  ],
  "recommendation": "split",
  "missing_files": ["internal/deploy/worker.go"],
  "missing_sections": [{"path": "docs/arch.md", "heading": "Deployment"}],
  "unknown_files": ["build.rs"],
  "split_candidates": [
    "Consider splitting: 3 files touch different packages (swim, planedit, aiclient)"
  ],
  "merge_candidates": []
}
```

---

## Native Waveplan Input

The current `ContextCandidate` JSON path is the low-level estimator interface.
It is appropriate for tests and hand-authored advanced usage, but it is not the
intended normal workflow for Waveplan users.

Normal Waveplan usage should be:

```bash
python waveplan-cli context estimate \
  --plan docs/plans/<plan>-execution-waves.json \
  --task T1
```

or:

```bash
python waveplan-cli context estimate \
  --plan docs/plans/<plan>-execution-waves.json \
  --unit T1.1
```

The wrapper should:

1. Load the execution-waves JSON
2. Resolve the selected task or unit
3. Convert that source object into `ContextCandidate`
4. Invoke `contextsize`
5. Print the `ContextEstimate`

`ContextCandidate` therefore remains the adapter boundary, not the primary UX.

### Waveplan Adapter Contract

The first native adapter targets execution-waves JSON and supports both
task-level and unit-level estimation.

Suggested surface:

```go
func FromWaveplanTask(planPath string, taskID string) (contextsize.ContextCandidate, error)
func FromWaveplanUnit(planPath string, unitID string) (contextsize.ContextCandidate, error)
```

V1 mapping rules:

- **Task candidate**
  - `id` = task ID
  - `title` = task title
  - `description` = `""`
  - `referenced_files` = union of task `files` and task `doc_refs` resolved through `doc_index`
  - `depends_on` = collapsed external task dependencies derived from unit-level dependencies
  - `source` = `"waveplan"`
- **Unit candidate**
  - `id` = unit ID
  - `title` = unit title
  - `description` = `""`
  - `referenced_files` = unit `doc_refs` resolved through `doc_index`
  - `depends_on` = unit `depends_on`
  - `source` = `"waveplan"`

Additional rules:

- Deduplicate and sort `referenced_files`
- Reject missing `task_id` / `unit_id` deterministically
- Reject invalid selector combinations (`--candidate` with `--plan`, `--task` with `--unit`, missing selector for `--plan`)
- Keep the raw `--candidate` path for advanced/manual use

### Adapter Scope

Keep the estimator package source-agnostic. The Waveplan-specific decode and
mapping logic belongs in a separate adapter layer, not in `internal/contextsize`
itself.

## Future Extensions (deferred)

- **FP issue adapter**: `FromFPIssue(issue) ContextCandidate`
- **Tree-sitter/LSP enrichment**: symbol maps, precise dependency graph
- **Dagdir integration**: candidates from `dagdir2waveplan` output

These extend the adapters layer without changing the core `Estimate()` contract.

---

## MCP Tool (implemented)

The MCP tool `waveplan_context_estimate` is implemented. See `docs/superpowers/specs/2026-05-13-contextsizer-mcp-integration.md` for the full spec.

---

## Changelog

- **2026-05-13** Initial draft. MCP tool implemented (FP-vedqsmra, see `2026-05-13-contextsizer-mcp-integration.md`).
- **2026-05-13** Added native Waveplan adapter contract and `waveplan-cli context estimate --plan/--task|--unit` follow-up scope.
