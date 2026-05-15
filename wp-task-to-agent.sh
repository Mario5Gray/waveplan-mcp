#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
WAVEPLAN_CLI_BIN="${WAVEPLAN_CLI_BIN:-}"
WP_PLAN_STEP_BIN="${WP_PLAN_STEP_BIN:-}"
WP_AGENT_DISPATCH_BIN="${WP_AGENT_DISPATCH_BIN:-}"

usage() {
  cat <<'USAGE'
Usage:
  wp-task-to-agent.sh --target <codex|claude|opencode> --plan <plan.json> --agent <agent-name>
                      [--mode implement|review|fix] [--reviewer <name>] [--dry-run]

Description:
  Compatibility wrapper for legacy direct agent-dispatch flows.

  implement mode:
    - resumes the agent's currently taken task when one exists
    - otherwise derives the next task_id from waveplan peek and delegates to wp-plan-step.sh

  review mode:
    - derives the agent's current taken task_id and delegates to wp-plan-step.sh

  fix mode:
    - uses SWIM_TASK_ID when injected by SWIM
    - otherwise derives the agent's current review_taken task_id and delegates to wp-plan-step.sh

Notes:
  - --task-id is not accepted here. Use wp-plan-step.sh for exact task execution.
USAGE
}

TARGET=""
PLAN=""
AGENT=""
MODE="implement"
REVIEWER=""
DRY_RUN="0"
TASK_ID_ARG=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target) TARGET="${2:-}"; shift 2 ;;
    --plan) PLAN="${2:-}"; shift 2 ;;
    --agent) AGENT="${2:-}"; shift 2 ;;
    --mode) MODE="${2:-}"; shift 2 ;;
    --reviewer) REVIEWER="${2:-}"; shift 2 ;;
    --task-id) TASK_ID_ARG="${2:-}"; shift 2 ;;
    --dry-run) DRY_RUN="1"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ -n "$TASK_ID_ARG" ]]; then
  echo "--task-id is not supported by wp-task-to-agent.sh. Use wp-plan-step.sh --action ... --task-id $TASK_ID_ARG instead." >&2
  exit 2
fi

resolve_runtime_plan() {
  local requested="$1"
  local env_plan="${WAVEPLAN_PLAN:-}"
  if [[ -n "$requested" && -f "$requested" ]]; then
    printf '%s\n' "$requested"
    return 0
  fi
  if [[ -n "$env_plan" && -f "$env_plan" ]]; then
    printf '%s\n' "$env_plan"
    return 0
  fi
  return 1
}

resolve_plan_step_bin() {
  if [[ -n "$WP_PLAN_STEP_BIN" ]]; then
    printf '%s\n' "$WP_PLAN_STEP_BIN"
    return 0
  fi
  if command -v wp-plan-step.sh >/dev/null 2>&1; then
    command -v wp-plan-step.sh
    return 0
  fi
  if [[ -x "$SCRIPT_DIR/wp-plan-step.sh" ]]; then
    printf '%s\n' "$SCRIPT_DIR/wp-plan-step.sh"
    return 0
  fi
  return 1
}

resolve_dispatch_bin() {
  if [[ -n "$WP_AGENT_DISPATCH_BIN" ]]; then
    printf '%s\n' "$WP_AGENT_DISPATCH_BIN"
    return 0
  fi
  if command -v wp-agent-dispatch.sh >/dev/null 2>&1; then
    command -v wp-agent-dispatch.sh
    return 0
  fi
  if [[ -x "$SCRIPT_DIR/wp-agent-dispatch.sh" ]]; then
    printf '%s\n' "$SCRIPT_DIR/wp-agent-dispatch.sh"
    return 0
  fi
  return 1
}

if [[ -z "$TARGET" || -z "$PLAN" || -z "$AGENT" ]]; then
  echo "Missing required args: --target, --plan, --agent" >&2
  usage
  exit 2
fi

case "$TARGET" in
  codex|claude|opencode) ;;
  *)
    echo "Invalid --target: $TARGET (must be codex|claude|opencode)" >&2
    exit 2
    ;;
esac

case "$MODE" in
  implement|review|fix) ;;
  *)
    echo "Invalid --mode: $MODE (must be implement, review, or fix)" >&2
    exit 2
    ;;
esac

if ! PLAN="$(resolve_runtime_plan "$PLAN")"; then
  echo "Plan file not found: $PLAN" >&2
  if [[ -n "${WAVEPLAN_PLAN:-}" ]]; then
    echo "WAVEPLAN_PLAN fallback also missing: $WAVEPLAN_PLAN" >&2
  fi
  exit 2
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found" >&2
  exit 2
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

PLAN_STEP_BIN="$(resolve_plan_step_bin)" || {
  echo "wp-plan-step.sh not found. Set WP_PLAN_STEP_BIN or install in PATH." >&2
  exit 2
}

waveplan_cli() {
  python3 "$WAVEPLAN_CLI_BIN" "$@"
}

json_has_error() {
  local payload="$1"
  python3 -c 'import json,sys
try:
  obj=json.loads(sys.argv[1])
except Exception:
  sys.exit(1)
sys.exit(0 if isinstance(obj, dict) and obj.get("error") else 1)
' "$payload"
}

json_task_id() {
  local payload="$1"
  python3 -c 'import json,sys; print(json.loads(sys.argv[1] or "{}").get("task_id",""))' "$payload"
}

task_status() {
  local payload="$1"
  python3 -c 'import json,sys; print(json.loads(sys.argv[1] or "{}").get("status",""))' "$payload"
}

select_task_for_agent_status() {
  local plan="$1"
  local agent="$2"
  local wanted_status="$3"
  local agent_tasks_json
  agent_tasks_json="$(waveplan_cli --plan "$plan" get "$agent")"
  if json_has_error "$agent_tasks_json"; then
    return 1
  fi
  python3 -c 'import json,sys
obj=json.loads(sys.argv[1] or "{}")
wanted=sys.argv[2]
tasks=[t for t in obj.get("tasks", []) if t.get("status")==wanted]
if not tasks:
  print("")
  sys.exit(0)
tasks.sort(key=lambda t: ((t.get("started_at") or ""), (t.get("task_id") or "")), reverse=True)
sel=tasks[0]
print(sel.get("task_id", ""))
print(json.dumps(sel))
' "$agent_tasks_json" "$wanted_status"
}

write_temp_file() {
  local content="$1"
  local path
  path="$(mktemp)"
  printf '%s\n' "$content" >"$path"
  printf '%s\n' "$path"
}

cleanup() {
  for f in "${TMP_FILES[@]:-}"; do
    [[ -n "$f" ]] && rm -f "$f"
  done
}
TMP_FILES=()
trap cleanup EXIT

if [[ "$MODE" == "implement" ]]; then
  RESUME_SELECTED="$(select_task_for_agent_status "$PLAN" "$AGENT" "taken" || true)"
  RESUME_TASK_ID="$(printf '%s\n' "$RESUME_SELECTED" | sed -n '1p')"
  RESUME_TASK_JSON="$(printf '%s\n' "$RESUME_SELECTED" | sed -n '2,$p')"

  if [[ -n "$RESUME_TASK_ID" && -n "$RESUME_TASK_JSON" ]]; then
    DISPATCH_BIN="$(resolve_dispatch_bin)" || {
      echo "wp-agent-dispatch.sh not found. Set WP_AGENT_DISPATCH_BIN or install in PATH." >&2
      exit 2
    }
    TASK_JSON_FILE="$(write_temp_file "$RESUME_TASK_JSON")"
    TMP_FILES+=("$TASK_JSON_FILE")
    DISPATCH_ARGS=(--target "$TARGET" --plan "$PLAN" --agent "$AGENT" --mode implement --task-json-file "$TASK_JSON_FILE")
    if [[ "$DRY_RUN" == "1" ]]; then
      DISPATCH_ARGS+=(--dry-run)
    fi
    exec env SWIM_TASK_SOURCE="resume_taken" "$DISPATCH_BIN" "${DISPATCH_ARGS[@]}"
  fi

  PEEK_JSON="$(waveplan_cli --plan "$PLAN" peek)"
  if json_has_error "$PEEK_JSON"; then
    echo "waveplan peek returned error payload:" >&2
    echo "$PEEK_JSON" >&2
    exit 1
  fi
  TASK_ID="$(json_task_id "$PEEK_JSON")"
  if [[ -z "$TASK_ID" ]]; then
    echo "waveplan peek did not return a task_id" >&2
    exit 1
  fi
  ARGS=(--action implement --plan "$PLAN" --task-id "$TASK_ID" --target "$TARGET" --agent "$AGENT")
  if [[ "$DRY_RUN" == "1" ]]; then
    ARGS+=(--dry-run)
  fi
  exec "$PLAN_STEP_BIN" "${ARGS[@]}"
fi

if [[ "$MODE" == "review" ]]; then
  if [[ -z "$REVIEWER" ]]; then
    REVIEWER="$AGENT"
  fi
  SELECTED="$(select_task_for_agent_status "$PLAN" "$AGENT" "taken" || true)"
  TASK_ID="$(printf '%s\n' "$SELECTED" | sed -n '1p')"
  if [[ -z "$TASK_ID" ]]; then
    echo "No currently taken task found for agent '$AGENT'" >&2
    exit 1
  fi
  ARGS=(--action review --plan "$PLAN" --task-id "$TASK_ID" --target "$TARGET" --agent "$AGENT" --reviewer "$REVIEWER")
  if [[ "$DRY_RUN" == "1" ]]; then
    ARGS+=(--dry-run)
  fi
  exec "$PLAN_STEP_BIN" "${ARGS[@]}"
fi

TASK_ID="${SWIM_TASK_ID:-}"
if [[ -n "$TASK_ID" ]]; then
  CURRENT_TASK_JSON="$(waveplan_cli --plan "$PLAN" get "task-$TASK_ID")"
  if json_has_error "$CURRENT_TASK_JSON"; then
    echo "waveplan get task returned error payload:" >&2
    echo "$CURRENT_TASK_JSON" >&2
    exit 1
  fi
  STATUS="$(task_status "$CURRENT_TASK_JSON")"
  if [[ "$STATUS" != "review_taken" && "$STATUS" != "taken" ]]; then
    echo "task $TASK_ID is not in a fixable state" >&2
    exit 1
  fi
else
  SELECTED="$(select_task_for_agent_status "$PLAN" "$AGENT" "review_taken" || true)"
  TASK_ID="$(printf '%s\n' "$SELECTED" | sed -n '1p')"
fi

if [[ -z "$TASK_ID" ]]; then
  echo "No review_taken task found for agent '$AGENT'" >&2
  exit 1
fi

ARGS=(--action fix --plan "$PLAN" --task-id "$TASK_ID" --target "$TARGET" --agent "$AGENT")
if [[ "$DRY_RUN" == "1" ]]; then
  ARGS+=(--dry-run)
fi
exec "$PLAN_STEP_BIN" "${ARGS[@]}"
