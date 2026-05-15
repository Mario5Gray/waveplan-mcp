#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  wp-agent-dispatch.sh \
    --target <codex|claude|opencode> \
    --plan <plan.json> \
    --agent <agent-name> \
    --mode <implement|review|fix> \
    --task-json-file <path> \
    [--reviewer <name>] \
    [--review-result-json-file <path>] \
    [--task-after-json-file <path>] \
    [--prior-review-stdout <path>] \
    [--dry-run]

Description:
  Prompt-delivery helper for scheduled SWIM rows. This script does not mutate
  waveplan state and does not accept task selectors such as --task-id.
USAGE
}

TARGET=""
PLAN=""
AGENT=""
MODE=""
REVIEWER=""
TASK_JSON_FILE=""
REVIEW_RESULT_JSON_FILE=""
TASK_AFTER_JSON_FILE=""
PRIOR_REVIEW_STDOUT=""
DRY_RUN="0"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target) TARGET="${2:-}"; shift 2 ;;
    --plan) PLAN="${2:-}"; shift 2 ;;
    --agent) AGENT="${2:-}"; shift 2 ;;
    --mode) MODE="${2:-}"; shift 2 ;;
    --reviewer) REVIEWER="${2:-}"; shift 2 ;;
    --task-json-file) TASK_JSON_FILE="${2:-}"; shift 2 ;;
    --review-result-json-file) REVIEW_RESULT_JSON_FILE="${2:-}"; shift 2 ;;
    --task-after-json-file) TASK_AFTER_JSON_FILE="${2:-}"; shift 2 ;;
    --prior-review-stdout) PRIOR_REVIEW_STDOUT="${2:-}"; shift 2 ;;
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

if [[ -z "$TARGET" || -z "$PLAN" || -z "$AGENT" || -z "$MODE" || -z "$TASK_JSON_FILE" ]]; then
  echo "Missing required args: --target, --plan, --agent, --mode, --task-json-file" >&2
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
  exit 2
fi

if [[ ! -f "$TASK_JSON_FILE" ]]; then
  echo "Task JSON file not found: $TASK_JSON_FILE" >&2
  exit 2
fi

for bin in python3 "$TARGET"; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "Required command not found: $bin" >&2
    exit 2
  fi
done

write_dispatch_receipt() {
  local task_id="$1"
  local task_source="$2"
  local receipt_path="${SWIM_DISPATCH_RECEIPT_PATH:-}"
  if [[ -z "$receipt_path" ]]; then
    return 0
  fi
  mkdir -p "$(dirname "$receipt_path")"
  python3 - "$receipt_path" "$task_id" "$task_source" "$TARGET" "$AGENT" "$MODE" <<'PY'
import json
import os
import sys
from datetime import datetime, timezone

receipt_path, task_id, task_source, target, agent, mode = sys.argv[1:7]
payload = {
    "ok": True,
    "step_id": os.environ.get("SWIM_STEP_ID", ""),
    "task_id": task_id,
    "action": mode,
    "target": target,
    "agent": agent,
    "mode": mode,
    "task_source": task_source,
    "tool": target,
    "delivered_on": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
}
with open(receipt_path, "w", encoding="utf-8") as f:
    json.dump(payload, f, indent=2)
    f.write("\n")
PY
}

read_file_or_empty() {
  local path="$1"
  if [[ -n "$path" && -f "$path" ]]; then
    cat "$path"
  fi
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

  if [[ "$TARGET" == "codex" ]]; then
    printf '%s\n' "$prompt" | codex exec -
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

TASK_JSON="$(cat "$TASK_JSON_FILE")"
TASK_ID="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1] or "{}").get("task_id",""))' "$TASK_JSON")"
TASK_SOURCE="${SWIM_TASK_SOURCE:-provided_task_json}"

case "$MODE" in
  implement)
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
    ;;
  review)
    REVIEW_RESULT_JSON="$(read_file_or_empty "$REVIEW_RESULT_JSON_FILE")"
    TASK_AFTER_JSON="$(read_file_or_empty "$TASK_AFTER_JSON_FILE")"
    read -r -d '' PREAMBLE <<'TXT' || true
new task for review.

hard requirements:
- use superpowers skill workflow while reviewing.
- waveplan-cli / waveplan-mcp usage in this task is read-only only.
- do not execute waveplan write actions: no pop, no fin, no start_review, no end_review, no plan mutations.
- allowed waveplan reads: peek, get, deptree, list_plans.
- this turn is review-focused; verify correctness, regressions, tests, and contract adherence.
- return concrete findings first (ordered by severity), then required fixes.
TXT

    PROMPT="$PREAMBLE

plan: $PLAN
claimed_by: $AGENT
reviewer: $REVIEWER
mode: review
task_source: $TASK_SOURCE

selected_task_json:
$TASK_JSON

start_review_result_json:
${REVIEW_RESULT_JSON:-{}}

task_snapshot_after_start_review_json:
${TASK_AFTER_JSON:-{}}
"
    ;;
  fix)
    PRIOR_REVIEW_CONTENT="$(read_file_or_empty "$PRIOR_REVIEW_STDOUT")"
    read -r -d '' PREAMBLE <<'TXT' || true
fix cycle: address reviewer findings.

hard requirements:
- use superpowers skill workflow while implementing fixes.
- waveplan-cli / waveplan-mcp usage in this task is read-only only.
- do not execute waveplan write actions: no pop, no fin, no start_review, no end_review, no plan mutations.
- allowed waveplan reads: peek, get, deptree, list_plans.

apply all required fixes from the reviewer findings below.
run tests relevant to changes.
return concrete file changes + verification commands/results.
TXT

    PROMPT="$PREAMBLE

plan: $PLAN
claimed_by: $AGENT
mode: fix

task_json:
$TASK_JSON

reviewer_findings:
${PRIOR_REVIEW_CONTENT:-<no prior review stdout available>}
"
    ;;
esac

send_prompt "$PROMPT"
write_dispatch_receipt "$TASK_ID" "$TASK_SOURCE"
