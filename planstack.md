# The Optimal Stack: Fiberplane, Drift, Waveplan, Superpowers, txtstore

This guide describes how to use the five components together as a cohesive agent-first development workflow.

## Architecture

```
Superpowers (agent skills)
    ↓ guides
Waveplan (execution waves)
    ↓ orchestrates
Drift (infrastructure state)
    ↓ observes
Fiberplane (API & observability)
    ↑ journals (plan mode only)
txtstore (agent notes / planning journal)
```

## How It Works

### 1. Plan with Waveplan

Create an execution wave plan (`*-execution-waves.json`) that breaks your feature into ordered, dependency-aware waves. Use `waveplan-mcp` or `waveplan-cli` to manage the lifecycle:

```bash
python waveplan-cli peek          # see next task
python waveplan-cli pop agent-1   # claim a task
# ... agent works ...
python waveplan-cli fin T1.1 abc123  # mark complete
```

### 2. Manage Infrastructure with Drift

Use Drift to track and manage infrastructure state alongside your code changes. Drift detects deviations between desired and actual state, ensuring infrastructure stays in sync with your plans.

### 3. Observe with Fiberplane

Fiberplane provides the observability layer — monitoring execution progress, API health, and system state in real time. It surfaces insights from both code and infrastructure changes.

### 4. Guide Agents with Superpowers

Superpowers skills are invoked contextually to ensure agents follow disciplined workflows:

- **brainstorming** — explore requirements before implementation
- **writing-plans** — structure multi-step tasks
- **subagent-driven-development** — coordinate parallel agent work
- **verification-before-completion** — confirm work is actually done
- **test-driven-development** — write tests before code

### 5. Journal with txtstore

txtstore is a note-taking MCP that agents use as a planning journal. It writes sectioned markdown files with an embedded index (TOC), making it easy to accumulate structured notes across a planning session.

**When it is active:**

- **Disabled by default** — not loaded unless explicitly configured
- **Enabled in plan mode** — automatically active when an agent enters a planning phase
- **Enabled on request** — activated when the user or agent explicitly asks for note-taking

**File naming convention:**

Notes are stored under `docs/agent_notes/` using this pattern:

```text
docs/agent_notes/{AGENT_NAME}_{SHORT_PLAN_NAME}_{PHASE}_{SECTION}_{ETC}_{DATE:yymmdd}.md
```

Examples:

```text
docs/agent_notes/claude_auth-refactor_planning_260509.md
docs/agent_notes/claude_auth-refactor_implementation_wave1_260509.md
docs/agent_notes/subagent-2_txtstore-mcp_review_260509.md
```

Fields after `AGENT_NAME` and `SHORT_PLAN_NAME` are optional and positional — include `PHASE`, `SECTION`, and additional qualifiers only when they add meaningful disambiguation.

**Tools provided by `txtstore-mcp`:**

- `txtstore_append` — add a new section (auto-renames duplicate titles with `-2`, `-3`, …)
- `txtstore_edit` — replace an existing section
- `txtstore_write_swim_plan` — write a deterministic SWIM markdown plan from a structured payload

Both tools accept `unit` and `section` flags for heading hierarchy (e.g. `## Wave1 > OAuth > Notes`).

Human CLI help is available with `txtstore --help`. MCP tools expose descriptions and argument schemas, but not an interactive help command. Use `append`/`edit` for journals and notes; use `write-swim-plan` only for canonical SWIM plan source generation, because it validates and overwrites the target markdown file atomically.

## Typical Workflow

1. **Design**: Use Superpowers `brainstorming` to explore requirements, then `writing-plans` to create an execution plan.
2. **Plan**: Generate a Waveplan execution wave file with `*-execution-waves.json`.
3. **Execute**: Agents use `waveplan-cli` or `waveplan-mcp` to claim and complete tasks wave by wave.
4. **Deploy**: Drift manages infrastructure state changes in parallel with code changes.
5. **Observe**: Fiberplane surfaces the full picture — task progress, infrastructure state, and system health.
6. **Review**: Use Superpowers `verification-before-completion` and `requesting-code-review` skills to ensure quality.

## Configuration

### waveplan-mcp

```json
{
  "mcpServers": {
    "waveplan": {
      "command": "./waveplan-mcp",
      "args": ["--plan", "my-feature-execution-waves.json"]
    }
  }
}
```

### Superpowers

Install skills via the Superpowers CLI or manually from [github.com/obra/superpowers](https://github.com/obra/superpowers).

### Drift & Fiberplane

Follow the respective documentation at [fiberplane.com](https://fiberplane.com) for setup and configuration.
