#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

PLAN="$TMP/plan.json"
CLI_STUB="$TMP/waveplan-cli-stub.py"
CODEX_STUB="$TMP/codex"
ARGS_OUT="$TMP/codex-args.txt"
STDIN_OUT="$TMP/codex-stdin.txt"
RECEIPT_OUT="$TMP/receipt.json"

printf '{}\n' >"$PLAN"

cat >"$CLI_STUB" <<'PY'
#!/usr/bin/env python3
import json
import sys

args = sys.argv[1:]
if "peek" in args:
    print(json.dumps({"task_id": "T1.1", "title": "stub task", "status": "available"}))
elif "pop" in args:
    print(json.dumps({"task_id": "T1.1", "title": "stub task", "status": "taken"}))
else:
    print(json.dumps({"ok": True}))
PY
chmod +x "$CLI_STUB"

cat >"$CODEX_STUB" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" >"$ARGS_OUT"
cat >"$STDIN_OUT" || true
SH
chmod +x "$CODEX_STUB"

export ARGS_OUT STDIN_OUT
export SWIM_DISPATCH_RECEIPT_PATH="$RECEIPT_OUT"
export PATH="$TMP:$PATH"
export WAVEPLAN_CLI_BIN="$CLI_STUB"

"$ROOT/wp-plan-step.sh" \
  --action implement \
  --target codex \
  --plan "$PLAN" \
  --task-id T1.1 \
  --agent sigma

if [[ ! -f "$ARGS_OUT" ]]; then
  echo "FAIL: codex stub was not invoked"
  exit 1
fi

first_arg="$(sed -n '1p' "$ARGS_OUT")"
if [[ "$first_arg" != "exec" ]]; then
  echo "FAIL: expected codex to be invoked as 'codex exec', got first arg: ${first_arg:-<empty>}"
  exit 1
fi

grep -q "new task for implementation" "$STDIN_OUT" || {
  echo "FAIL: codex exec did not receive implementation prompt on stdin"
  exit 1
}

test -f "$RECEIPT_OUT" || {
  echo "FAIL: dispatch receipt was not written"
  exit 1
}

grep -q '"ok": true' "$RECEIPT_OUT" || {
  echo "FAIL: dispatch receipt does not contain ok=true"
  exit 1
}

echo "PASS: wp-plan-step implement dispatches via codex after task precheck"
