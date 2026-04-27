# The Optimal Stack: Fiberplane, Drift, Waveplan, Superpowers

This guide describes how to use the four components together as a cohesive agent-first development workflow.

## Architecture

```
Superpowers (agent skills)
    ↓ guides
Waveplan (execution waves)
    ↓ orchestrates
Drift (infrastructure state)
    ↓ observes
Fiberplane (API & observability)
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