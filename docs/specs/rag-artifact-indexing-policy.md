# RAG Artifact Indexing Policy

This policy defines which waveplan/SWIM artifacts are suitable for retrieval
augmented generation (RAG), how to group them, and which artifacts should stay
as live lookups or audit-only data.

## Principle

Prefer artifacts that explain intent, constraints, and durable decisions.
Treat runtime artifacts as operational evidence, not primary knowledge.

RAG should answer questions like:

- What work exists?
- Why is this task structured this way?
- Which contract governs this behavior?
- What did reviewers or implementers decide?

RAG should not be the source of truth for current task status. Use live state
files or SWIM tools for that.

## Index Classes

### Knowledge Index

Use this as the default RAG corpus for planning, implementation assistance,
review preparation, and architecture questions.

| artifact | globs | why it is useful |
|---|---|---|
| Specs and contracts | `docs/specs/**/*.md`, `docs/specs/**/*.json` | Durable behavioral truth, schemas, operator semantics, implementation plans. |
| Execution wave plans | `docs/plans/*-execution-waves.json` | Task intent, wave topology, dependency structure, ownership units. |
| Plan prose | `docs/plans/*.md` | Human-readable planning context and progress notes. |
| Agent notes | `docs/agent_notes/**/*.md` | Handoffs, implementation notes, review context, planning journal entries. |
| Superpowers specs/plans | `docs/superpowers/specs/**/*.md`, `docs/superpowers/plans/**/*.md` | Validated design docs and implementation plans created by agent workflows. |
| Stack docs | `README.md`, `planstack.md`, `AGENTS.md` | Project-level operating model and agent workflow expectations. |

Recommended metadata:

```json
{
  "index_class": "knowledge",
  "artifact_kind": "spec|plan|agent_note|stack_doc|schema",
  "path": "docs/specs/2026-05-05-swim-ops.md",
  "task_id": "optional",
  "wave": "optional",
  "generated": false
}
```

### Operational Index

Use this for audit, incident review, progress reconstruction, and debugging.
Keep it separate from the knowledge index so transient execution facts do not
drown out durable planning context.

| artifact | globs | why it is useful |
|---|---|---|
| Compiled SWIM schedules | `docs/plans/*-execution-schedule.json`, `**/*-swim-schedule.json` | Exact executable rows, argv, preconditions, postconditions. |
| SWIM journals | `**/*.journal.json`, `.waveplan/swim/**/*.journal.json` | Append-only execution chronology and outcomes. |
| SWIM logs | `.waveplan/swim/**/logs/*.log` | Captured stdout/stderr for failed, blocked, or reviewed steps. |
| Dispatch receipts | `.waveplan/swim/**/receipts/*.dispatch.json` | Proof of prompt delivery and optional inquiry signals. |
| Refinement sidecars | `**/*.swim.refine.*.json`, `docs/**/swim-refine-*.json` | Fine-grained decomposition under a parent unit. |

Recommended metadata:

```json
{
  "index_class": "operational",
  "artifact_kind": "schedule|journal|log|receipt|refinement",
  "path": ".waveplan/swim/example/logs/S1_T1.1_implement.1.stdout.log",
  "step_id": "optional",
  "task_id": "optional",
  "attempt": "optional",
  "outcome": "optional",
  "generated": true
}
```

### Live Lookup Only

Do not use these as normal RAG documents. Query them directly when current state
matters.

| artifact | globs | reason |
|---|---|---|
| Waveplan state files | `docs/plans/*.state.json`, `**/*-execution-waves.json.state.json`, `*.state.json` | Current lifecycle state changes frequently and should not be retrieved as stale knowledge. |
| Locks | `.waveplan/swim/**/swim.lock` | Process coordination only. |
| Local generated install output | `~/.local/share/waveplan/**`, build outputs | Environment-specific, not project knowledge. |

## Chunking Rules

Markdown:

- Chunk by heading, preserving heading hierarchy in metadata.
- Keep short sections together when they are under the same parent heading.
- Target 500-900 tokens per chunk for specs and plans.
- Include preceding heading path as metadata, not repeated body text.

JSON plans:

- Chunk `*-execution-waves.json` by task/unit, not by raw byte size.
- Include parent wave, task id, title, dependencies, files scope, and acceptance
  criteria in the same chunk when present.
- Keep cross-task dependency summaries as separate chunks only when needed.

SWIM schedules:

- Chunk by schedule row for operational retrieval.
- Store `seq`, `step_id`, `task_id`, `action`, `requires`, `produces`, and
  `invoke.argv` as structured metadata.
- Prefer exact metadata filters over semantic similarity when answering
  execution-order questions.

Journals:

- Chunk by event or small event windows.
- Store `event_id`, `step_id`, `task_id`, `attempt`, `outcome`, and timestamp
  fields as metadata.
- Use recency filters by default.

Logs:

- Index only when needed for audit/debug workflows.
- Chunk by logical sections if the log has markers; otherwise use bounded
  windows with overlap.
- Attach log chunks to `step_id`, `attempt`, and stream (`stdout` or `stderr`).

Receipts:

- Prefer structured lookup over text chunking.
- Index receipt text only in the operational index when inquiry/audit search is
  required.

## Ranking Guidance

Default retrieval order for planning and implementation questions:

1. Matching spec or design doc.
2. Matching execution wave task.
3. Matching agent note or progress note.
4. Matching schedule row.
5. Matching journal/log/receipt evidence.

Default retrieval order for runtime/debug questions:

1. Live state lookup.
2. Matching journal event.
3. Matching log or receipt.
4. Matching schedule row.
5. Matching plan/spec context.

## Exclusion Rules

Exclude by default:

```text
.git/**
.waveplan/swim/**/swim.lock
**/*.state.json
**/*-execution-waves.json.state.json
**/node_modules/**
**/vendor/**
**/dist/**
**/build/**
```

These exclusions can be overridden for a dedicated operational/debug index, but
state files and locks should remain live lookups.

## Freshness Rules

- Re-index knowledge artifacts on file change.
- Re-index operational artifacts by run/session and prefer recent events.
- Do not treat indexed state files as authoritative.
- If a retrieved operational artifact conflicts with live state, live state wins.

## Minimal Recommended Setup

For a first useful RAG corpus, index only:

```text
README.md
planstack.md
AGENTS.md
docs/specs/**/*.md
docs/specs/**/*.json
docs/plans/*-execution-waves.json
docs/plans/*.md
docs/superpowers/specs/**/*.md
docs/superpowers/plans/**/*.md
docs/agent_notes/**/*.md
```

Then add an operational index for:

```text
docs/plans/*-execution-schedule.json
**/*.journal.json
.waveplan/swim/**/logs/*.log
.waveplan/swim/**/receipts/*.dispatch.json
```

