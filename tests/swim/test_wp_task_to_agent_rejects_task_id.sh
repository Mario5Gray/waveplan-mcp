#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

PLAN="$TMP/plan.json"
printf '{}\n' >"$PLAN"

set +e
OUTPUT="$("$ROOT/wp-task-to-agent.sh" \
  --target codex \
  --plan "$PLAN" \
  --agent sigma \
  --task-id T1.1 2>&1)"
STATUS=$?
set -e

if [[ "$STATUS" -eq 0 ]]; then
  echo "FAIL: expected wp-task-to-agent.sh to reject --task-id"
  exit 1
fi

printf '%s' "$OUTPUT" | grep -q -- "--task-id is not supported by wp-task-to-agent.sh" || {
  echo "FAIL: expected rejection message to mention unsupported --task-id"
  exit 1
}

printf '%s' "$OUTPUT" | grep -q "wp-plan-step.sh" || {
  echo "FAIL: expected rejection message to point callers to wp-plan-step.sh"
  exit 1
}

echo "PASS: wp-task-to-agent rejects --task-id and points callers to wp-plan-step.sh"
