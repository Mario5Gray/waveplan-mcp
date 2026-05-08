# Swim Markdown Plan Format v1

## Purpose

Define a **deterministic** markdown source format that can be compiled into `*-execution-waves.json` without interpretation drift.

This format is intentionally strict:
- human-editable
- machine-parseable
- fail-fast on ambiguity

## Scope

This spec defines:
- required markdown sections
- required table schemas
- lexical and normalization rules
- deterministic compile rules to JSON
- validation/error behavior

This spec does **not** define runtime execution semantics (see SWIM implementation spec for that).

## File Requirements

- File extension: `.md`
- Encoding: UTF-8
- Line endings: `\n` (LF)
- Exactly one top-level `#` heading
- Required sections in this exact order:
  1. `## Meta`
  2. `## Plan`
  3. `## Doc Index`
  4. `## FP Index`
  5. `## Tasks`
  6. `## Units`

Anything outside those sections is ignored except:
- the top-level heading text
- markdown comments are ignored

## Lexical Rules

- Identifiers are case-sensitive.
- `task_id`: `^T[0-9]+$`
- `unit_id`: `^T[0-9]+\\.[0-9]+$`
- `wave`: positive integer
- `kind`: one of `impl|test|verify|doc|refactor|code`
- List fields use comma-separated values with no escaped commas.
- Empty list fields are `-`.

## Section Schemas

### 1) `## Meta`

Required markdown table columns:
- `key`
- `value`

Required keys:
- `schema_version` (integer)
- `generated_on` (`YYYY-MM-DD`)
- `plan_version` (integer)
- `plan_generation` (RFC3339 timestamp)

Optional keys:
- `title_override`

Example:

```markdown
## Meta
| key | value |
|---|---|
| schema_version | 1 |
| generated_on | 2026-05-08 |
| plan_version | 1 |
| plan_generation | 2026-05-08T00:00:00Z |
```

### 2) `## Plan`

Required markdown table columns:
- `plan_id`
- `plan_title`
- `plan_doc_path`
- `spec_doc_path`

Exactly one data row is required.

### 3) `## Doc Index`

Required markdown table columns:
- `ref`
- `path`
- `line`
- `kind`

`kind` must be one of: `plan|spec|code|test|doc`.

`ref` must be unique.

### 4) `## FP Index`

Required markdown table columns:
- `fp_ref`
- `fp_id`

Both must be unique.
`fp_ref` is display key (e.g. `FP-abc12345`), `fp_id` is canonical backend ID.

### 5) `## Tasks`

Required markdown table columns:
- `task_id`
- `title`
- `plan_line`
- `doc_refs`
- `files`

Rules:
- `task_id` unique, `Tn` format.
- `doc_refs` list values must exist in Doc Index `ref`.
- `files` is comma-separated path list or `-`.

### 6) `## Units`

Required markdown table columns:
- `unit_id`
- `task_id`
- `title`
- `kind`
- `wave`
- `plan_line`
- `depends_on`
- `fp_refs`
- `doc_refs`

Rules:
- `unit_id` unique, `Tn.m` format.
- `task_id` must exist in Tasks.
- `depends_on` entries must be valid `unit_id` values.
- `fp_refs` entries must exist in FP Index.
- `doc_refs` entries must exist in Doc Index.

## Normalization Rules (Deterministic Compile)

Compiler MUST:

1. Trim outer whitespace for every scalar cell.
2. Treat `-` as empty list for list fields.
3. Split list fields on `,`, trim each element, drop empties.
4. Enforce uniqueness and schema constraints before emit.
5. Sort object keys deterministically in output:
  - `fp_index`: lexical by key
  - `doc_index`: lexical by key
  - `tasks`: numeric `Tn`
  - `units`: numeric `Tn.m`
6. Preserve declared `depends_on` order only if it is already stable; otherwise sort `depends_on` by numeric unit order.
7. Emit canonical JSON formatting: `indent=2`, trailing newline, no trailing spaces.

## JSON Emission Mapping

Compiler outputs `*-execution-waves.json` with:

- `schema_version`, `generated_on`, `plan_version`, `plan_generation` from Meta
- `plan` object:
  - `id`, `title`
  - `plan_doc.path`, `plan_doc.line=1`
  - `spec_doc.path`, `spec_doc.line=1`
  - optional `notes` if implemented by extension
- `fp_index` map from FP Index section
- `doc_index` map from Doc Index section
- `tasks` object keyed by `task_id`
- `units` object keyed by `unit_id`

## Determinism Guarantees

For identical markdown bytes, compiler must produce identical JSON bytes.

For semantically identical markdown with reordered rows, compiler must produce identical JSON bytes after normalization.

## Failure Semantics

Compiler must exit nonzero on any violation and print one deterministic primary error.

Primary error classes:
- `FORMAT_ERROR`: section/table shape invalid
- `SCHEMA_ERROR`: field missing/invalid enum/type
- `REFERENCE_ERROR`: missing doc/fp/task/unit references
- `GRAPH_ERROR`: unit dependency cycle
- `DUPLICATE_ERROR`: duplicate IDs/refs

No best-effort output on failure.

## Example (Minimal Valid Skeleton)

```markdown
# Swim Plan Source

## Meta
| key | value |
|---|---|
| schema_version | 1 |
| generated_on | 2026-05-08 |
| plan_version | 1 |
| plan_generation | 2026-05-08T00:00:00Z |

## Plan
| plan_id | plan_title | plan_doc_path | spec_doc_path |
|---|---|---|---|
| swim-implementation | Swim — Durable Execution State Machine | docs/specs/2026-05-05-swim-implementation-plan.md | docs/specs/2026-05-05-swim-implementation-plan.md |

## Doc Index
| ref | path | line | kind |
|---|---|---:|---|
| spec | docs/specs/2026-05-05-swim-implementation-plan.md | 1 | spec |

## FP Index
| fp_ref | fp_id |
|---|---|
| FP-example | abcdefghijklmnopqrstuvwxyz |

## Tasks
| task_id | title | plan_line | doc_refs | files |
|---|---|---:|---|---|
| T1 | Phase 1 | 270 | spec | waveplan-cli |

## Units
| unit_id | task_id | title | kind | wave | plan_line | depends_on | fp_refs | doc_refs |
|---|---|---|---|---:|---:|---|---|---|
| T1.1 | T1 | Compile deterministic JSON | impl | 1 | 270 | - | FP-example | spec |
```

## Versioning

- This spec is `v1`.
- Backward-incompatible changes require `v2` document and compiler mode flag.
- Compiler default should remain explicit (`--format v1`) rather than implicit latest.
