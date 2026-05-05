#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
WAVEPLAN_CLI_BIN="${WAVEPLAN_CLI_BIN:-}"

usage() {
  cat <<'USAGE'
Usage:
  wp-task-to-agent.sh \
    --target <codex|claude|opencode> \
    --plan <plan.json> \
    --agent <agent-name> \
    [--mode implement|review] \
    [--reviewer <name>] \
    [--dry-run]

Description:
  implement mode:
    1) pops next task from waveplan for given agent (write action)
    2) builds implementation prompt and sends to target CLI

  review mode:
    1) finds current taken task for agent
    2) runs start_review <task_id> <reviewer> (write action)
    3) builds review prompt and sends to target CLI

Options:
  --target <name>        codex | claude | opencode
  --plan <path>          Path to *-execution-waves.json
  --agent <name>         Agent name
  --mode <mode>          implement (default) | review
  --reviewer <name>      Reviewer for review mode (default: --agent)
  --dry-run              Print generated prompt; no waveplan writes
  -h, --help             Show this help

Environment (opencode target):
  OPENCODE_ATTACH_URL    Server URL to attach to (default: http://127.0.0.1:4096)
  OPENCODE_AUTO_SERVE    If set to 1, auto-start `opencode serve` when unreachable
  OPENCODE_SERVER_HOST   Host for auto-serve (default: 127.0.0.1)
  OPENCODE_SERVER_PORT   Port for auto-serve (default: 4096)
  OPENCODE_SERVER_USERNAME / OPENCODE_SERVER_PASSWORD
                         Optional basic-auth credentials for attach requests

Environment:
  WAVEPLAN_CLI_BIN        Path to waveplan-cli (default: PATH lookup, then sibling file)
USAGE
}

TARGET=""
PLAN=""
AGENT=""
MODE="implement"
REVIEWER=""
DRY_RUN="0"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      TARGET="${2:-}"
      shift 2
      ;;
    --plan)
      PLAN="${2:-}"
      shift 2
      ;;
    --agent)
      AGENT="${2:-}"
      shift 2
      ;;
    --mode)
      MODE="${2:-}"
      shift 2
      ;;
    --reviewer)
      REVIEWER="${2:-}"
      shift 2
      ;;
    --dry-run)
      DRY_RUN="1"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

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

if [[ "$MODE" != "implement" && "$MODE" != "review" ]]; then
  echo "Invalid --mode: $MODE (must be implement or review)" >&2
  exit 2
fi

if [[ ! -f "$PLAN" ]]; then
  echo "Plan file not found: $PLAN" >&2
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

for bin in python3 "$TARGET"; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "Required command not found: $bin" >&2
    exit 2
  fi
done

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

select_taken_task_for_agent() {
  local plan="$1"
  local agent="$2"
  local agent_tasks_json
  agent_tasks_json="$(waveplan_cli --plan "$plan" get "$agent")"
  if json_has_error "$agent_tasks_json"; then
    return 1
  fi
  python3 -c 'import json,sys
obj=json.loads(sys.argv[1] or "{}")
tasks=obj.get("tasks", [])
taken=[t for t in tasks if t.get("status")=="taken"]
if not taken:
  print("")
  sys.exit(0)
taken.sort(key=lambda t: ((t.get("started_at") or ""), (t.get("task_id") or "")), reverse=True)
sel=taken[0]
print(sel.get("task_id", ""))
print(json.dumps(sel))
' "$agent_tasks_json"
}

send_prompt() {
  local prompt="$1"
  if [[ "$DRY_RUN" == "1" ]]; then
    printf '%s\n' "$prompt"
    return 0
  fi

  if [[ "$TARGET" == "opencode" ]]; then
    send_prompt_opencode "$prompt"
    return 0
  fi

  if ! printf '%s\n' "$prompt" | "$TARGET"; then
    "$TARGET" "$prompt"
  fi
}

opencode_server_healthy() {
  local base_url="$1"
  local user="${OPENCODE_SERVER_USERNAME:-}"
  local pass="${OPENCODE_SERVER_PASSWORD:-}"
  python3 - "$base_url" "$user" "$pass" <<'PY'
import base64
import json
import sys
import urllib.request

base = sys.argv[1].rstrip("/")
user = sys.argv[2]
pwd = sys.argv[3]
url = f"{base}/global/health"
req = urllib.request.Request(url, method="GET")
if pwd:
    token = base64.b64encode(f"{(user or 'opencode')}:{pwd}".encode("utf-8")).decode("ascii")
    req.add_header("Authorization", f"Basic {token}")
try:
    with urllib.request.urlopen(req, timeout=2.0) as r:
        body = r.read().decode("utf-8", "replace")
    data = json.loads(body)
    ok = bool(data.get("healthy")) if isinstance(data, dict) else False
    sys.exit(0 if ok else 1)
except Exception:
    sys.exit(1)
PY
}

maybe_start_opencode_server() {
  if [[ "${OPENCODE_AUTO_SERVE:-0}" != "1" ]]; then
    return 1
  fi
  local host="${OPENCODE_SERVER_HOST:-127.0.0.1}"
  local port="${OPENCODE_SERVER_PORT:-4096}"
  nohup opencode serve --hostname "$host" --port "$port" >/tmp/opencode-serve.log 2>&1 &
  for _ in $(seq 1 20); do
    sleep 0.25
    if opencode_server_healthy "${OPENCODE_ATTACH_URL:-http://127.0.0.1:4096}"; then
      return 0
    fi
  done
  return 1
}

send_prompt_opencode() {
  local prompt="$1"
  local attach_url="${OPENCODE_ATTACH_URL:-http://127.0.0.1:4096}"
  local user="${OPENCODE_SERVER_USERNAME:-}"
  local pass="${OPENCODE_SERVER_PASSWORD:-}"
  local -a cmd

  if ! opencode_server_healthy "$attach_url"; then
    maybe_start_opencode_server || true
  fi

  cmd=(opencode run --format json)
  if opencode_server_healthy "$attach_url"; then
    cmd+=(--attach "$attach_url")
    if [[ -n "$user" ]]; then
      cmd+=(--username "$user")
    fi
    if [[ -n "$pass" ]]; then
      cmd+=(--password "$pass")
    fi
  fi
  cmd+=("$prompt")

  "${cmd[@]}"
}

if [[ "$MODE" == "implement" ]]; then
  RESUME_SELECTED="$(select_taken_task_for_agent "$PLAN" "$AGENT" || true)"
  RESUME_TASK_ID="$(printf '%s\n' "$RESUME_SELECTED" | sed -n '1p')"
  RESUME_TASK_JSON="$(printf '%s\n' "$RESUME_SELECTED" | sed -n '2,$p')"

  if [[ -n "$RESUME_TASK_ID" && -n "$RESUME_TASK_JSON" ]]; then
    TASK_JSON="$RESUME_TASK_JSON"
    TASK_SOURCE="resume_taken"
  else
    if [[ "$DRY_RUN" == "1" ]]; then
      TASK_JSON="$(waveplan_cli --plan "$PLAN" peek)"
      TASK_SOURCE="peek"
    else
      TASK_JSON="$(waveplan_cli --plan "$PLAN" pop "$AGENT")"
      TASK_SOURCE="pop"
      if json_has_error "$TASK_JSON"; then
        ERR_MSG="$(json_error_message "$TASK_JSON")"
        if [[ "$ERR_MSG" == *"already taken"* || "$ERR_MSG" == *"No available tasks"* ]]; then
          RESUME_SELECTED="$(select_taken_task_for_agent "$PLAN" "$AGENT" || true)"
          RESUME_TASK_ID="$(printf '%s\n' "$RESUME_SELECTED" | sed -n '1p')"
          RESUME_TASK_JSON="$(printf '%s\n' "$RESUME_SELECTED" | sed -n '2,$p')"
          if [[ -n "$RESUME_TASK_ID" && -n "$RESUME_TASK_JSON" ]]; then
            TASK_JSON="$RESUME_TASK_JSON"
            TASK_SOURCE="resume_taken_after_pop_error"
          fi
        fi
      fi
    fi
  fi

  if json_has_error "$TASK_JSON"; then
    echo "waveplan $TASK_SOURCE returned error payload:" >&2
    echo "$TASK_JSON" >&2
    exit 1
  fi

  read -r -d '' PREAMBLE <<'TXT' || true
new task for implementation.

hard requirements:
- use superpowers skill workflow while implementing.
- waveplan-cli / waveplan-mcp usage in this task is read-only only.
- do not execute waveplan write actions: no pop, no fin, no start_review, no end_review, no plan mutations.
- allowed waveplan reads: peek, get, deptree, list_plans.

execute implementation directly in repository.
run tests relevant to changes.
return concrete file changes + verification commands/results.
TXT

  PROMPT="$PREAMBLE

plan: $PLAN
claimed_by: $AGENT
mode: implement
task_source: $TASK_SOURCE

task_json:
$TASK_JSON
"

  send_prompt "$PROMPT"
  exit 0
fi

# review mode
if [[ -z "$REVIEWER" ]]; then
  REVIEWER="$AGENT"
fi

AGENT_TASKS_JSON="$(waveplan_cli --plan "$PLAN" get "$AGENT")"
if json_has_error "$AGENT_TASKS_JSON"; then
  echo "waveplan get returned error payload:" >&2
  echo "$AGENT_TASKS_JSON" >&2
  exit 1
fi

SELECTED="$(python3 -c 'import json,sys
obj=json.loads(sys.argv[1] or "{}")
tasks=obj.get("tasks", [])
taken=[t for t in tasks if t.get("status")=="taken"]
if not taken:
  print("")
  sys.exit(0)
taken.sort(key=lambda t: ((t.get("started_at") or ""), (t.get("task_id") or "")), reverse=True)
sel=taken[0]
print(sel.get("task_id", ""))
print(json.dumps(sel))
' "$AGENT_TASKS_JSON")"

TASK_ID="$(printf '%s\n' "$SELECTED" | sed -n '1p')"
CURRENT_TASK_JSON="$(printf '%s\n' "$SELECTED" | sed -n '2,$p')"

if [[ -z "$TASK_ID" || -z "$CURRENT_TASK_JSON" ]]; then
  echo "No currently taken task found for agent '$AGENT'" >&2
  exit 1
fi

if [[ "$DRY_RUN" == "1" ]]; then
  REVIEW_RESULT_JSON="{\"dry_run\":true,\"would_run\":\"python3 $WAVEPLAN_CLI_BIN --plan $PLAN start_review $TASK_ID $REVIEWER\"}"
  TASK_AFTER_JSON="{\"dry_run\":true,\"task_id\":\"$TASK_ID\"}"
else
  REVIEW_RESULT_JSON="$(waveplan_cli --plan "$PLAN" start_review "$TASK_ID" "$REVIEWER")"
  if json_has_error "$REVIEW_RESULT_JSON"; then
    echo "waveplan start_review returned error payload:" >&2
    echo "$REVIEW_RESULT_JSON" >&2
    exit 1
  fi
  TASK_AFTER_JSON="$(waveplan_cli --plan "$PLAN" get "task-$TASK_ID")"
fi

read -r -d '' PREAMBLE <<'TXT' || true
new task for review.

hard requirements:
- use superpowers skill workflow while reviewing.
- this turn is review-focused; verify correctness, regressions, tests, and contract adherence.
- return concrete findings first (ordered by severity), then required fixes.
TXT

PROMPT="$PREAMBLE

plan: $PLAN
claimed_by: $AGENT
reviewer: $REVIEWER
mode: review
task_source: taken_by_agent

selected_task_json:
$CURRENT_TASK_JSON

start_review_result_json:
$REVIEW_RESULT_JSON

task_snapshot_after_start_review_json:
$TASK_AFTER_JSON
"

send_prompt "$PROMPT"
