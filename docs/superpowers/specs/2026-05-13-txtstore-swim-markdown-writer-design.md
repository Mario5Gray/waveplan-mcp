# txtstore SWIM Markdown Writer Design

**Date:** 2026-05-13

## Purpose

Extend `txtstore` with a deterministic structured writer for the strict SWIM
markdown source format defined in
`docs/specs/swim-markdown-plan-format-v1.md`.

This feature must not alter the behavior of the existing freeform section tools:

- `txtstore_append`
- `txtstore_edit`

Those tools remain section/index oriented markdown storage. The new writer is a
separate path for emitting a complete, schema-constrained markdown document in
one pass.

## Problem

`internal/txtstore` currently models freeform markdown sections with a TOC
block and `##` headings. That shape is incompatible with the SWIM markdown plan
format, which requires:

- exactly one `#` heading
- six required `##` sections in exact order
- markdown tables with fixed column schemas
- deterministic normalization and sorting rules
- fail-fast validation before emit

Trying to assemble this format through repeated `append` or `edit` operations
would make temporary invalid states normal and would move validation burden to
callers. That is the wrong boundary.

## Design

Add a new structured writer API in `internal/txtstore` that accepts a typed
document model and renders the entire SWIM markdown file atomically.

### Core package API

Add a new source file:

`internal/txtstore/swim_markdown.go`

Proposed public surface:

```go
package txtstore

type SwimPlanDoc struct {
	Title          string
	Meta           SwimMeta
	Plan           SwimPlan
	DocIndex       []SwimDocRef
	FPIndex        []SwimFPRef
	Tasks          []SwimTask
	Units          []SwimUnit
}

type SwimMeta struct {
	SchemaVersion  int
	GeneratedOn    string
	PlanVersion    int
	PlanGeneration string
	TitleOverride  string
}

type SwimPlan struct {
	PlanID      string
	PlanTitle   string
	PlanDocPath string
	SpecDocPath string
}

type SwimDocRef struct {
	Ref  string
	Path string
	Line int
	Kind string
}

type SwimFPRef struct {
	FPRef string
	FPID  string
}

type SwimTask struct {
	TaskID   string
	Title    string
	PlanLine int
	DocRefs  []string
	Files    []string
}

type SwimUnit struct {
	UnitID    string
	TaskID    string
	Title     string
	Kind      string
	Wave      int
	PlanLine  int
	DependsOn []string
	FPRefs    []string
	DocRefs   []string
}

func (fs *FileStore) WriteSwimPlan(doc SwimPlanDoc) error
func RenderSwimPlan(doc SwimPlanDoc) (string, error)
func ValidateSwimPlan(doc SwimPlanDoc) error
```

### Output contract

`WriteSwimPlan` must:

1. validate the document against the SWIM markdown format rules
2. normalize rows and list fields deterministically
3. render the complete markdown document
4. write atomically to `fs.filepath`

The writer never mutates an existing file structurally. It overwrites the target
with the canonical rendered form of the provided `SwimPlanDoc`.

### Normalization rules

The implementation must follow `swim-markdown-plan-format-v1.md` directly.

Required behavior:

- section order is fixed:
  `Meta`, `Plan`, `Doc Index`, `FP Index`, `Tasks`, `Units`
- scalar cells are trimmed
- list fields render as comma-separated values
- empty lists render as `-`
- `doc_index` rows sort lexically by `ref`
- `fp_index` rows sort lexically by `fp_ref`
- `tasks` sort by numeric `Tn`
- `units` sort by numeric `Tn.m`
- `depends_on` order is normalized to numeric unit order
- output uses `\n` line endings and ends with one trailing newline

### Validation rules

`ValidateSwimPlan` must fail before rendering on:

- missing required top-level fields
- invalid `task_id`, `unit_id`, `wave`, `kind`
- duplicate doc refs, fp refs, task ids, or unit ids
- task references to missing doc refs
- unit references to missing task/doc/fp refs
- dependency cycles in `units.depends_on`

Validation errors should be deterministic and stable. A typed error is useful,
but not required for v1 as long as the primary failure is specific and
reproducible.

## MCP and CLI surface

Existing tools remain unchanged:

- `txtstore_append`
- `txtstore_edit`

Add one new MCP tool:

- `txtstore_write_swim_plan`

Parameters:

- `filepath` (required)
- `title` (required)
- `meta` (required object)
- `plan` (required object)
- `doc_index` (required array)
- `fp_index` (required array)
- `tasks` (required array)
- `units` (required array)

Behavior:

- build `SwimPlanDoc` from arguments
- validate and render using `internal/txtstore`
- return JSON summary including `success`, `filepath`, `title`, and counts

Add one matching CLI command:

- `txtstore write-swim-plan <filepath> <json>`

The CLI may accept either raw JSON payload or a path to a JSON file, but the
MCP tool is the primary interface.

## File map

- `internal/txtstore/swim_markdown.go`
- `internal/txtstore/swim_markdown_test.go`
- `cmd/txtstore-mcp/main.go`
- `cmd/txtstore/main.go`

No changes to the semantics of:

- `internal/txtstore/filestore.go`
- `txtstore_append`
- `txtstore_edit`

## Testing

The implementation should be TDD-driven.

Required test coverage:

- minimal valid document renders exact required section order
- list normalization renders `-` for empties
- tasks and units sort deterministically regardless of input order
- duplicate refs/ids fail validation
- missing references fail validation
- dependency cycle fails validation
- `WriteSwimPlan` writes atomically and overwrites non-canonical target content
- MCP tool round-trips a valid payload into a correct markdown file

## Non-goals

- parsing markdown back into `SwimPlanDoc`
- making `txtstore_append` or `txtstore_edit` SWIM-aware
- introducing a generic markdown schema engine
- supporting alternate markdown plan dialects in this change

## Recommended implementation sequence

1. Add validation and render tests for `SwimPlanDoc`
2. Implement `ValidateSwimPlan` and `RenderSwimPlan`
3. Add `FileStore.WriteSwimPlan`
4. Expose `txtstore_write_swim_plan` in `cmd/txtstore-mcp/main.go`
5. Expose `write-swim-plan` in `cmd/txtstore/main.go`
