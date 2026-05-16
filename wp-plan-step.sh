#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
WAVEPLAN_CLI_BIN="${WAVEPLAN_CLI_BIN:-}"
WP_AGENT_DISPATCH_BIN="${WP_AGENT_DISPATCH_BIN:-}"

usage() {
  cat <<'USAGE'
Usage:
  wp-plan-step.sh --action <implement|review|fix|end_review|finish> --plan <plan.json> --task-id <Tn.m>
                  [--target <codex|claude|opencode>] [--agent <name>] [--reviewer <name>]
                  [--review-note <text>] [--git-sha <sha|DEFERRED>] [--dry-run]

Description:
  Canonical scheduled-step executor. This script owns waveplan state transitions
  for a specific task_id and delegates prompt delivery to wp-agent-dispatch.sh.
USAGE
}

ACTION=""
PLAN=""
TASK_ID=""
TARGET=""
AGENT=""
REVIEWER=""
REVIEW_NOTE=""
GIT_SHA=""
DRY_RUN="0"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --action) ACTION="${2:-}"; shift 2 ;;
    --plan) PLAN="${2:-}"; shift 2 ;;
    --task-id) TASK_ID="${2:-}"; shift 2 ;;
    --target) TARGET="${2:-}"; shift 2 ;;
    --agent) AGENT="${2:-}"; shift 2 ;;
    --reviewer) REVIEWER="${2:-}"; shift 2 ;;
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

if [[ -z "$ACTION" || -z "$PLAN" || -z "$TASK_ID" ]]; then
  echo "Missing required args: --action, --plan, --task-id" >&2
  usage
  exit 2
fi

case "$ACTION" in
  implement|review|fix|end_review|finish) ;;
  *)
    echo "Invalid --action: $ACTION" >&2
    exit 2
    ;;
esac

if ! PLAN="$(resolve_runtime_plan "$PLAN")"; then
  echo "Plan file not found: $PLAN" >&2
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

json_error_message() {
  local payload="$1"
  python3 -c 'import json,sys
try:
  obj=json.loads(sys.argv[1] or "{}")
except Exception:
  print("")
  sys.exit(0)
if isinstance(obj, dict):
  print(obj.get("error",""))
else:
  print("")
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

write_temp_file() {
  local content="$1"
  local path
  path="$(mktemp)"
  printf '%s\n' "$content" >"$path"
  printf '%s\n' "$path"
}

require_dispatch_args() {
  if [[ -z "$TARGET" || -z "$AGENT" ]]; then
    echo "Action $ACTION requires --target and --agent" >&2
    exit 2
  fi
}

if [[ "$ACTION" == "review" && -z "$REVIEWER" ]]; then
  REVIEWER="$AGENT"
fi

case "$ACTION" in
  implement|review|fix)
    require_dispatch_args
    DISPATCH_BIN="$(resolve_dispatch_bin)" || {
      echo "wp-agent-dispatch.sh not found. Set WP_AGENT_DISPATCH_BIN or install in PATH." >&2
      exit 2
    }
    ;;
esac

cleanup() {
  for f in "${TMP_FILES[@]:-}"; do
    [[ -n "$f" ]] && rm -f "$f"
  done
}
TMP_FILES=()
trap cleanup EXIT

case "$ACTION" in
  implement)
    PEEK_JSON="$(waveplan_cli --plan "$PLAN" peek)"
    if json_has_error "$PEEK_JSON"; then
      echo "waveplan peek returned error payload:" >&2
      echo "$PEEK_JSON" >&2
      exit 1
    fi
    if [[ "$(json_task_id "$PEEK_JSON")" != "$TASK_ID" ]]; then
      echo "peek task_id mismatch: expected $TASK_ID got $(json_task_id "$PEEK_JSON")" >&2
      exit 1
    fi

    if [[ "$DRY_RUN" == "1" ]]; then
      TASK_JSON="$PEEK_JSON"
      TASK_SOURCE="peek"
    else
      TASK_JSON="$(waveplan_cli --plan "$PLAN" pop "$AGENT")"
      if json_has_error "$TASK_JSON"; then
        echo "waveplan pop returned error payload:" >&2
        echo "$TASK_JSON" >&2
        exit 1
      fi
      if [[ "$(json_task_id "$TASK_JSON")" != "$TASK_ID" ]]; then
        echo "pop task_id mismatch: expected $TASK_ID got $(json_task_id "$TASK_JSON")" >&2
        exit 1
      fi
      TASK_SOURCE="pop"
    fi

    TASK_JSON_FILE="$(write_temp_file "$TASK_JSON")"
    TMP_FILES+=("$TASK_JSON_FILE")
    DISPATCH_ARGS=(--target "$TARGET" --plan "$PLAN" --agent "$AGENT" --mode implement --task-json-file "$TASK_JSON_FILE")
    if [[ "$DRY_RUN" == "1" ]]; then
      DISPATCH_ARGS+=(--dry-run)
    fi
    exec "$DISPATCH_BIN" "${DISPATCH_ARGS[@]}"
    ;;

  review)
    CURRENT_TASK_JSON="$(waveplan_cli --plan "$PLAN" get "task-$TASK_ID")"
    if json_has_error "$CURRENT_TASK_JSON"; then
      echo "waveplan get task returned error payload:" >&2
      echo "$CURRENT_TASK_JSON" >&2
      exit 1
    fi
    if [[ "$(task_status "$CURRENT_TASK_JSON")" != "taken" ]]; then
      echo "task $TASK_ID is not in taken state" >&2
      exit 1
    fi

    if [[ "$DRY_RUN" == "1" ]]; then
      REVIEW_RESULT_JSON="{\"dry_run\":true,\"would_run\":\"python3 $WAVEPLAN_CLI_BIN --plan $PLAN start_review $TASK_ID $REVIEWER\"}"
      TASK_AFTER_JSON="$CURRENT_TASK_JSON"
    else
      REVIEW_RESULT_JSON="$(waveplan_cli --plan "$PLAN" start_review "$TASK_ID" "$REVIEWER")"
      if json_has_error "$REVIEW_RESULT_JSON"; then
        echo "waveplan start_review returned error payload:" >&2
        echo "$REVIEW_RESULT_JSON" >&2
        exit 1
      fi
      TASK_AFTER_JSON="$(waveplan_cli --plan "$PLAN" get "task-$TASK_ID")"
    fi

    TASK_JSON_FILE="$(write_temp_file "$CURRENT_TASK_JSON")"
    REVIEW_RESULT_FILE="$(write_temp_file "$REVIEW_RESULT_JSON")"
    TASK_AFTER_FILE="$(write_temp_file "$TASK_AFTER_JSON")"
    TMP_FILES+=("$TASK_JSON_FILE" "$REVIEW_RESULT_FILE" "$TASK_AFTER_FILE")
    DISPATCH_ARGS=(--target "$TARGET" --plan "$PLAN" --agent "$AGENT" --reviewer "$REVIEWER" --mode review \
      --task-json-file "$TASK_JSON_FILE" --review-result-json-file "$REVIEW_RESULT_FILE" \
      --task-after-json-file "$TASK_AFTER_FILE")
    if [[ "$DRY_RUN" == "1" ]]; then
      DISPATCH_ARGS+=(--dry-run)
    fi
    exec "$DISPATCH_BIN" "${DISPATCH_ARGS[@]}"
    ;;

  fix)
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

    if [[ "$DRY_RUN" != "1" ]]; then
      FIX_RESULT_JSON="$(waveplan_cli --plan "$PLAN" start_fix "$TASK_ID")"
      if json_has_error "$FIX_RESULT_JSON"; then
        echo "waveplan start_fix returned error payload:" >&2
        echo "$FIX_RESULT_JSON" >&2
        exit 1
      fi
      CURRENT_TASK_JSON="$(waveplan_cli --plan "$PLAN" get "task-$TASK_ID")"
    fi

    TASK_JSON_FILE="$(write_temp_file "$CURRENT_TASK_JSON")"
    TMP_FILES+=("$TASK_JSON_FILE")
    DISPATCH_ARGS=(--target "$TARGET" --plan "$PLAN" --agent "$AGENT" --mode fix --task-json-file "$TASK_JSON_FILE")
    if [[ -n "${SWIM_PRIOR_STDOUT_PATH:-}" ]]; then
      DISPATCH_ARGS+=(--prior-review-stdout "$SWIM_PRIOR_STDOUT_PATH")
    fi
    if [[ "$DRY_RUN" == "1" ]]; then
      DISPATCH_ARGS+=(--dry-run)
    fi
    exec "$DISPATCH_BIN" "${DISPATCH_ARGS[@]}"
    ;;

  end_review)
    CMD=("python3" "$WAVEPLAN_CLI_BIN" "--plan" "$PLAN" "end_review" "$TASK_ID")
    if [[ -n "$REVIEW_NOTE" ]]; then
      CMD+=("$REVIEW_NOTE")
    fi
    ;;

  finish)
    CMD=("python3" "$WAVEPLAN_CLI_BIN" "--plan" "$PLAN" "fin" "$TASK_ID")
    if [[ -n "$GIT_SHA" ]]; then
      CMD+=("$GIT_SHA")
    fi
    ;;
esac

if [[ "$DRY_RUN" == "1" ]]; then
  printf '%q ' "${CMD[@]}"
  printf '\n'
  exit 0
fi

"${CMD[@]}"
