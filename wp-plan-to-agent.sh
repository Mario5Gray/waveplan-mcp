#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
WAVEPLAN_CLI_BIN="${WAVEPLAN_CLI_BIN:-}"
WP_PLAN_STEP_BIN="${WP_PLAN_STEP_BIN:-}"

usage() {
  cat <<'USAGE'
Usage:
  wp-plan-to-agent.sh --mode implement --target <codex|claude|opencode> --plan <plan.json> --agent <agent> [--dry-run]
  wp-plan-to-agent.sh --mode review --target <codex|claude|opencode> --plan <plan.json> --agent <owner_agent> --reviewer <reviewer_agent> [--dry-run]
  wp-plan-to-agent.sh --mode review_end --plan <plan.json> --task-id <Tn.m> [--review-note <text>] [--dry-run]
  wp-plan-to-agent.sh --mode fin --plan <plan.json> --task-id <Tn.m> [--git-sha <sha|DEFERRED>] [--dry-run]

Description:
  Compatibility wrapper that resolves a task_id from the old mode-based API and
  then delegates to wp-plan-step.sh.
USAGE
}

MODE=""
TARGET=""
PLAN=""
AGENT=""
REVIEWER=""
TASK_ID=""
REVIEW_NOTE=""
GIT_SHA=""
DRY_RUN="0"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="${2:-}"; shift 2 ;;
    --target) TARGET="${2:-}"; shift 2 ;;
    --plan) PLAN="${2:-}"; shift 2 ;;
    --agent) AGENT="${2:-}"; shift 2 ;;
    --reviewer) REVIEWER="${2:-}"; shift 2 ;;
    --task-id) TASK_ID="${2:-}"; shift 2 ;;
    --review-note) REVIEW_NOTE="${2:-}"; shift 2 ;;
    --git-sha) GIT_SHA="${2:-}"; shift 2 ;;
    --dry-run) DRY_RUN="1"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

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

if [[ -z "$MODE" || -z "$PLAN" ]]; then
  echo "Missing required args: --mode and --plan" >&2
  usage
  exit 2
fi

if ! PLAN="$(resolve_runtime_plan "$PLAN")"; then
  echo "Plan file not found: $PLAN" >&2
  exit 2
fi

PLAN_STEP_BIN="$(resolve_plan_step_bin)" || {
  echo "wp-plan-step.sh not found. Set WP_PLAN_STEP_BIN or install in PATH." >&2
  exit 2
}

if [[ "$MODE" == "implement" || "$MODE" == "review" ]]; then
  if [[ -z "$TARGET" || -z "$AGENT" ]]; then
    echo "Mode $MODE requires --target and --agent" >&2
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

  if [[ "$MODE" == "implement" ]]; then
    PEEK_JSON="$(waveplan_cli --plan "$PLAN" peek)"
    if json_has_error "$PEEK_JSON"; then
      echo "waveplan peek returned error payload:" >&2
      echo "$PEEK_JSON" >&2
      exit 1
    fi
    TASK_ID="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1] or "{}").get("task_id",""))' "$PEEK_JSON")"
    if [[ -z "$TASK_ID" ]]; then
      echo "waveplan peek did not return a task_id" >&2
      exit 1
    fi
  else
    AGENT_TASKS_JSON="$(waveplan_cli --plan "$PLAN" get "$AGENT")"
    if json_has_error "$AGENT_TASKS_JSON"; then
      echo "waveplan get returned error payload:" >&2
      echo "$AGENT_TASKS_JSON" >&2
      exit 1
    fi
    TASK_ID="$(python3 -c 'import json,sys
obj=json.loads(sys.argv[1] or "{}")
taken=[t for t in obj.get("tasks", []) if t.get("status")=="taken"]
if not taken:
  print("")
  sys.exit(0)
taken.sort(key=lambda t: ((t.get("started_at") or ""), (t.get("task_id") or "")), reverse=True)
print(taken[0].get("task_id",""))
' "$AGENT_TASKS_JSON")"
    if [[ -z "$TASK_ID" ]]; then
      echo "No currently taken task found for agent '$AGENT'" >&2
      exit 1
    fi
  fi
fi

case "$MODE" in
  implement) ACTION="implement" ;;
  review) ACTION="review" ;;
  review_end) ACTION="end_review" ;;
  fin) ACTION="finish" ;;
  *)
    echo "Invalid --mode: $MODE" >&2
    usage
    exit 2
    ;;
esac

ARGS=("$PLAN_STEP_BIN" "--action" "$ACTION" "--plan" "$PLAN" "--task-id" "$TASK_ID")
if [[ -n "$TARGET" ]]; then
  ARGS+=("--target" "$TARGET")
fi
if [[ -n "$AGENT" ]]; then
  ARGS+=("--agent" "$AGENT")
fi
if [[ -n "$REVIEWER" ]]; then
  ARGS+=("--reviewer" "$REVIEWER")
fi
if [[ -n "$REVIEW_NOTE" ]]; then
  ARGS+=("--review-note" "$REVIEW_NOTE")
fi
if [[ -n "$GIT_SHA" ]]; then
  ARGS+=("--git-sha" "$GIT_SHA")
fi
if [[ "$DRY_RUN" == "1" ]]; then
  ARGS+=("--dry-run")
fi

exec "${ARGS[@]}"
