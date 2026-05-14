#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

MISSING_PLAN="$TMP/missing-plan.json"
REAL_PLAN="$TMP/real-plan.json"
TASK_STUB="$TMP/wp-task-to-agent-stub.sh"
ARGS_OUT="$TMP/args.txt"

printf '{}\n' >"$REAL_PLAN"

cat >"$TASK_STUB" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" >"$ARGS_OUT"
SH
chmod +x "$TASK_STUB"

export ARGS_OUT
export WAVEPLAN_PLAN="$REAL_PLAN"

WP_TASK_TO_AGENT_BIN="$TASK_STUB" "$ROOT/wp-plan-to-agent.sh" \
  --mode implement \
  --target codex \
  --plan "$MISSING_PLAN" \
  --agent sigma \
  --dry-run

if [[ ! -f "$ARGS_OUT" ]]; then
  echo "FAIL: wp-task-to-agent stub was not invoked"
  exit 1
fi

actual_plan="$(awk 'found { print; exit } $0 == "--plan" { found = 1 }' "$ARGS_OUT")"
if [[ "$actual_plan" != "$REAL_PLAN" ]]; then
  echo "FAIL: expected fallback plan $REAL_PLAN, got ${actual_plan:-<empty>}"
  exit 1
fi

echo "PASS: wp-plan-to-agent falls back to WAVEPLAN_PLAN when embedded --plan is stale"
