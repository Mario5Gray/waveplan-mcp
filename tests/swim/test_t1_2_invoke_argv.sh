#!/usr/bin/env bash
# T1.2 — verify emitter produces canonical invoke.argv per row with parity to wp_invoke.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

PLAN="docs/plans/2026-05-05-swim-execution-waves.json"
AGENTS="tests/swim/fixtures/waveagents.json"
WAVEPLAN_CLI_BIN="${WAVEPLAN_CLI_BIN:-$REPO/waveplan-cli}"

OUT="$(mktemp)"
trap "rm -f $OUT" EXIT

WAVEPLAN_CLI_BIN="$WAVEPLAN_CLI_BIN" \
  bash wp-emit-wave-execution.sh --plan "$PLAN" --agents "$AGENTS" --task-scope all > "$OUT"

# 1) invoke.argv is array of strings, length >= 1, per row
jq -e '.execution | all(.invoke.argv | type == "array" and length >= 1)' "$OUT" >/dev/null \
  || { echo "FAIL: invoke.argv not an array of length>=1 on every row"; exit 1; }
jq -e '.execution | all(.invoke.argv | all(type == "string"))' "$OUT" >/dev/null \
  || { echo "FAIL: invoke.argv contains non-string element"; exit 1; }

# 2) wp_invoke still present (legacy debug-only)
jq -e '.execution | all(has("wp_invoke"))' "$OUT" >/dev/null \
  || { echo "FAIL: wp_invoke missing on some row"; exit 1; }

# 3) argv ↔ wp_invoke parity: shell-tokenize wp_invoke must equal invoke.argv
python3 - "$OUT" <<'PY'
import json, shlex, sys
data = json.load(open(sys.argv[1]))
for row in data["execution"]:
    expected = shlex.split(row["wp_invoke"])
    actual = row["invoke"]["argv"]
    if expected != actual:
        print(f"FAIL parity {row['step_id']}: wp_invoke→{expected} vs argv→{actual}", file=sys.stderr)
        sys.exit(1)
PY

# 4) argv[0] is the invoker (defaults to wp-plan-to-agent.sh)
jq -e '.execution | all(.invoke.argv[0] | endswith("wp-plan-to-agent.sh"))' "$OUT" >/dev/null \
  || { echo "FAIL: argv[0] is not wp-plan-to-agent.sh on some row"; exit 1; }

echo "PASS: T1.2 invoke.argv canonical + wp_invoke parity"
