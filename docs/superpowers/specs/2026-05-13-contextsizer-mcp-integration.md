# Context Sizer MCP Integration

**Date:** 2026-05-13
**Status:** Implemented
**Issue:** FP-vedqsmra

---

## Overview

Extends the waveplan-mcp server with a `waveplan_context_estimate` tool that exposes the existing `internal/contextsize` estimation package as an MCP callable tool. This was previously listed as a future extension in the design spec.

```
ContextCandidate JSON + Budget → handleContextEstimate → ContextEstimate JSON
```

---

## File Changes

```
main.go
  + import "github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/contextsize"
  + tool registration in createTools() (~line 388)
  + handler func handleContextEstimate() (~line 1225)

main_test.go
  + TestHandleContextEstimate
  + TestHandleContextEstimate_MissingCandidate
  + TestHandleContextEstimate_InvalidCandidate
  + TestHandleContextEstimate_InvalidBudget
```

No new files — the handler delegates directly to `contextsize.Estimator.Estimate()` and `contextsize.EncodeEstimateJSON()`.

---

## Tool Definition

```go
mcp.NewTool("waveplan_context_estimate",
    mcp.WithDescription("Estimate the context footprint (token budget) of an issue candidate. Accepts a ContextCandidate JSON object, optional budget range, and base directory for resolving file paths."),
    mcp.WithString("candidate", mcp.Required(), mcp.Description("ContextCandidate JSON object (id, title, description, referenced_files, referenced_sections, depends_on, source)")),
    mcp.WithString("budget", mcp.Description("Budget range in tokens, min:max (default: 64000:192000)")),
    mcp.WithString("base_dir", mcp.Description("Root directory for resolving referenced file paths")),
)
```

---

## Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `candidate` | string | yes | — | ContextCandidate JSON object matching the schema from `internal/contextsize/types.go` |
| `budget` | string | no | `64000:192000` | Budget range in tokens, format `min:max` |
| `base_dir` | string | no | cwd | Root directory for resolving referenced file paths |

---

## Candidate Schema

The `candidate` parameter accepts a JSON string matching `ContextCandidate`:

```json
{
  "id": "T1.1",
  "title": "Add feature X",
  "description": "Brief description of the change",
  "referenced_files": ["main.go", "internal/foo.go"],
  "referenced_sections": [{"path": "docs/spec.md", "heading": "Architecture"}],
  "depends_on": ["T1.0"],
  "source": "waveplan"
}
```

Fields map directly to `internal/contextsize/types.go`:

```go
type ContextCandidate struct {
    ID                 string        `json:"id"`
    Title              string        `json:"title"`
    Description        string        `json:"description"`
    ReferencedFiles    []string      `json:"referenced_files"`
    ReferencedSections []SectionRef  `json:"referenced_sections"`
    DependsOn          []string      `json:"depends_on"`
    Source             string        `json:"source"`
}

type SectionRef struct {
    Path    string `json:"path"`
    Heading string `json:"heading"`
}
```

---

## Output

Returns a `ContextEstimate` JSON string matching the schema from `internal/contextsize/types.go`:

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

Field meanings are documented in the design spec (`2026-05-13-context-sizer-design.md`).

---

## Handler Implementation

```go
func (s *WaveplanServer) handleContextEstimate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // 1. Extract candidate JSON string (required)
    // 2. Unmarshal into contextsize.ContextCandidate
    // 3. Parse budget string "min:max" → contextsize.Budget
    // 4. Extract optional base_dir
    // 5. Call contextsize.Estimator{BaseDir: baseDir}.Estimate(candidate, budget)
    // 6. Encode result via contextsize.EncodeEstimateJSON()
    // 7. Return mcp.NewToolResultText(json output)
}
```

Error handling:
- Missing `candidate` parameter → `mcp.NewToolResultError(...)`
- Invalid candidate JSON → `mcp.NewToolResultError(...)`
- Invalid budget format → `mcp.NewToolResultError(...)`
- Estimation error → `mcp.NewToolResultError(...)`

All errors return a text result with `IsError: true` on the tool response.

---

## Test Coverage

| Test | Purpose |
|------|---------|
| `TestHandleContextEstimate` | Happy path: valid candidate with existing file, non-empty budget and base_dir |
| `TestHandleContextEstimate_MissingCandidate` | Returns error when candidate parameter is omitted |
| `TestHandleContextEstimate_InvalidCandidate` | Returns error when candidate is not valid JSON |
| `TestHandleContextEstimate_InvalidBudget` | Returns error when budget is not parseable |

---

## Integration with Existing Tools

The MCP tool complements the existing CLI and Python wrapper:

```
waveplan-mcp (MCP)    → waveplan_context_estimate  ← NEW
waveplan-cli          → context estimate
cmd/contextsize       → contextsize (Go binary)
```

All three share the same underlying `contextsize.Estimator.Estimate()` function. The MCP tool passes the candidate JSON inline rather than requiring a file path, which is more convenient for agent-to-server calls.

---

## Changelog

- **2026-05-13** Implemented. Added to `main.go` (import, tool registration, handler). Added 4 unit tests. FP-vedqsmra.