#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
WAVEPLAN_CLI_BIN="${WAVEPLAN_CLI_BIN:-}"
INVOKER="${WP_PLAN_TO_AGENT_BIN:-wp-plan-to-agent.sh}"

usage() {
  cat <<'USAGE'
Usage:
  wp-emit-wave-execution.sh --plan <plan.json> --agents <waveagents.json> [--task-scope <all|open>] [--invoker <path-or-cmd>] [--out <path>]

Description:
  Emits a JSON execution plan (no execution) with rows:
    { "seq", "step_id", "task_id", "action", "requires", "produces", "wp_invoke", "status" }

  Workflow per emitted task:
    1) implement/pop dispatch via wp-plan-to-agent.sh
    2) review dispatch via wp-plan-to-agent.sh
    3) end_review via wp-plan-to-agent.sh
    4) fin via wp-plan-to-agent.sh

Agent rotation:
  - Uses schedule from waveagents.json if provided.
  - Else uses agent list order.
  - Cycles indefinitely.
  - If pop-agent == review-agent, advances review-agent until different.

Task scope:
  - all (default): emit for every unit/task in plan order (wave, task_id)
  - open: emit only for currently claimable tasks from waveplan-cli get open

Environment:
  WAVEPLAN_CLI_BIN        Path to waveplan-cli (default: PATH, then sibling file)
  WP_PLAN_TO_AGENT_BIN    Command/path used in emitted wp_invoke strings (default: wp-plan-to-agent.sh)
USAGE
}

PLAN=""
AGENTS_JSON=""
OUT=""
TASK_SCOPE="all"
INVOKER_ARG=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --plan) PLAN="${2:-}"; shift 2 ;;
    --agents) AGENTS_JSON="${2:-}"; shift 2 ;;
    --task-scope) TASK_SCOPE="${2:-}"; shift 2 ;;
    --invoker) INVOKER_ARG="${2:-}"; shift 2 ;;
    --out) OUT="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ -z "$PLAN" || -z "$AGENTS_JSON" ]]; then
  echo "Missing required args: --plan and --agents" >&2
  usage
  exit 2
fi

if [[ "$TASK_SCOPE" != "all" && "$TASK_SCOPE" != "open" ]]; then
  echo "Invalid --task-scope: $TASK_SCOPE (must be all|open)" >&2
  exit 2
fi

if [[ ! -f "$PLAN" ]]; then
  echo "Plan file not found: $PLAN" >&2
  exit 2
fi

if [[ ! -f "$AGENTS_JSON" ]]; then
  echo "Agent config not found: $AGENTS_JSON" >&2
  exit 2
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found" >&2
  exit 2
fi

if [[ -n "$INVOKER_ARG" ]]; then
  INVOKER="$INVOKER_ARG"
fi

if [[ -z "$WAVEPLAN_CLI_BIN" ]]; then
  if command -v waveplan-cli >/dev/null 2>&1; then
    WAVEPLAN_CLI_BIN="$(command -v waveplan-cli)"
  elif [[ -x "$SCRIPT_DIR/waveplan-cli" ]]; then
    WAVEPLAN_CLI_BIN="$SCRIPT_DIR/waveplan-cli"
  elif [[ -f "$SCRIPT_DIR/waveplan-cli" ]]; then
    WAVEPLAN_CLI_BIN="$SCRIPT_DIR/waveplan-cli"
  fi
fi

if [[ -n "$WAVEPLAN_CLI_BIN" && ! -f "$WAVEPLAN_CLI_BIN" ]]; then
  if command -v "$WAVEPLAN_CLI_BIN" >/dev/null 2>&1; then
    WAVEPLAN_CLI_BIN="$(command -v "$WAVEPLAN_CLI_BIN")"
  fi
fi

if [[ -z "$WAVEPLAN_CLI_BIN" || ! -f "$WAVEPLAN_CLI_BIN" ]]; then
  echo "waveplan-cli not found. Set WAVEPLAN_CLI_BIN or install waveplan-cli in PATH." >&2
  exit 2
fi

EMITTED="$(python3 - "$PLAN" "$AGENTS_JSON" "$TASK_SCOPE" "$WAVEPLAN_CLI_BIN" "$INVOKER" <<'PY'
import json
import shlex
import subprocess
import sys

plan_path = sys.argv[1]
agents_path = sys.argv[2]
task_scope = sys.argv[3]
waveplan_cli_bin = sys.argv[4]
invoker = sys.argv[5]

with open(agents_path, 'r', encoding='utf-8') as f:
    cfg = json.load(f)

def parse_agents(data):
    # Preferred shape:
    # {
    #   "agents": [{"name":"phi","provider":"codex"}, ...],
    #   "schedule": ["phi","sigma","theta"]
    # }
    agents = []

    if isinstance(data, dict) and isinstance(data.get("agents"), list):
        for row in data["agents"]:
            if isinstance(row, dict) and row.get("name") and row.get("provider"):
                agents.append({"name": str(row["name"]), "provider": str(row["provider"])})
    elif isinstance(data, dict):
        # Fallback: object map values containing {name,provider}
        for value in data.values():
            if isinstance(value, dict) and value.get("name") and value.get("provider"):
                agents.append({"name": str(value["name"]), "provider": str(value["provider"])})

    if not agents:
        raise SystemExit("No agents found. Expected agents list or object values with {name,provider}.")

    seen = set()
    deduped = []
    for a in agents:
        if a["name"] in seen:
            continue
        seen.add(a["name"])
        deduped.append(a)

    agent_map = {a["name"]: a["provider"] for a in deduped}

    schedule = data.get("schedule") if isinstance(data, dict) else None
    if isinstance(schedule, list) and schedule:
        schedule = [str(x) for x in schedule]
        unknown = [s for s in schedule if s not in agent_map]
        if unknown:
            raise SystemExit(f"schedule contains unknown agent(s): {', '.join(unknown)}")
    else:
        schedule = [a["name"] for a in deduped]

    valid = {"codex", "claude", "opencode"}
    bad = [f"{k}:{v}" for k,v in agent_map.items() if v not in valid]
    if bad:
        raise SystemExit("Invalid provider(s): " + ", ".join(bad) + ". Allowed: codex|claude|opencode")

    uniq = list(dict.fromkeys(schedule))
    if len(uniq) < 2:
        raise SystemExit("Need at least 2 distinct scheduled agents for pop/review separation.")

    return agent_map, schedule

agent_map, schedule = parse_agents(cfg)

def load_tasks(scope):
    if scope == "open":
        raw = subprocess.check_output([
            "python3", waveplan_cli_bin, "--plan", plan_path, "get", "open"
        ], text=True)
        state = json.loads(raw)
        if isinstance(state, dict) and state.get("error"):
            raise SystemExit(f"waveplan error: {state['error']}")
        tasks = state.get("tasks", [])
        return tasks if isinstance(tasks, list) else []

    with open(plan_path, "r", encoding="utf-8") as f:
        plan_obj = json.load(f)
    units = plan_obj.get("units", {})
    if not isinstance(units, dict):
        return []
    tasks = []
    for unit_key, unit in units.items():
        if not isinstance(unit, dict):
            continue
        row = dict(unit)
        row["task_id"] = str(unit.get("task_id") or unit_key)
        tasks.append(row)
    return tasks

tasks = load_tasks(task_scope)

def sk(t):
    return (int(t.get("wave", 10**9)), str(t.get("task_id", "")))

tasks.sort(key=sk)

idx = 0
n = len(schedule)

def next_agent_name():
    global idx
    name = schedule[idx % n]
    idx += 1
    return name

execution = []
seq = 1
for t in tasks:
    tid = str(t.get("task_id", "")).strip()
    if not tid:
        continue

    pop_agent = next_agent_name()
    review_agent = next_agent_name()
    guard = 0
    while review_agent == pop_agent and guard < n:
        review_agent = next_agent_name()
        guard += 1

    pop_provider = agent_map[pop_agent]
    review_provider = agent_map[review_agent]
    wave = int(t.get("wave", 0) or 0)

    def add_row(action, wp_invoke_cmd, status):
        global seq
        status_map = {
            "implement": ("available", "taken"),
            "review": ("taken", "review_taken"),
            "end_review": ("review_taken", "review_ended"),
            "finish": ("review_ended", "completed"),
        }
        req, prod = status_map[action]
        step_id = f"S{wave}_{tid}_{action}"
        execution.append({
            "seq": seq,
            "step_id": step_id,
            "task_id": tid,
            "action": action,
            "requires": {"task_status": req},
            "produces": {"task_status": prod},
            "wp_invoke": wp_invoke_cmd,
            "status": status,
        })
        seq += 1

    # 1) Implement / pop dispatch
    cmd1 = (
        f"{shlex.quote(invoker)} --mode implement --target {shlex.quote(pop_provider)} "
        f"--plan {shlex.quote(plan_path)} --agent {shlex.quote(pop_agent)}"
    )
    add_row("implement", cmd1, "available")

    # 2) Review dispatch (owner=pop agent; reviewer=review agent)
    cmd2 = (
        f"{shlex.quote(invoker)} --mode review --target {shlex.quote(review_provider)} "
        f"--plan {shlex.quote(plan_path)} --agent {shlex.quote(pop_agent)} "
        f"--reviewer {shlex.quote(review_agent)}"
    )
    add_row("review", cmd2, "taken")

    # 3) End review
    cmd3 = (
        f"{shlex.quote(invoker)} --mode review_end --plan {shlex.quote(plan_path)} "
        f"--task-id {shlex.quote(tid)} --review-note '${{WP_COMMENT:-}}'"
    )
    add_row("end_review", cmd3, "taken")

    # 4) Complete
    cmd4 = (
        f"{shlex.quote(invoker)} --mode fin --plan {shlex.quote(plan_path)} "
        f"--task-id {shlex.quote(tid)} --git-sha '${{GIT_SHA:-DEFERRED}}'"
    )
    add_row("finish", cmd4, "completed")

print(json.dumps({"schema_version": 2, "execution": execution}, indent=2))
PY
)"

if [[ -n "$OUT" ]]; then
  printf '%s\n' "$EMITTED" > "$OUT"
  echo "Wrote execution emission JSON to $OUT"
else
  printf '%s\n' "$EMITTED"
fi
