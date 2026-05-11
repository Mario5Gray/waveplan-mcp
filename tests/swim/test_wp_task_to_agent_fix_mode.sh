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
PRIOR_STDOUT="$TMP/prior-review.txt"

printf '{}\n' >"$PLAN"
printf 'reviewer found: missing error handling in auth module\n' >"$PRIOR_STDOUT"

cat >"$CLI_STUB" <<'PY'
#!/usr/bin/env python3
import json, sys

args = sys.argv[1:]
if "get" in args:
    print(json.dumps({"tasks": [{"task_id": "T1.1", "title": "stub", "status": "taken"}]}))
elif "start_fix" in args:
    print(json.dumps({"ok": True, "task_id": args[-1]}))
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
export PATH="$TMP:$PATH"
export WAVEPLAN_CLI_BIN="$CLI_STUB"
export SWIM_DISPATCH_RECEIPT_PATH="$RECEIPT_OUT"
export SWIM_PRIOR_STDOUT_PATH="$PRIOR_STDOUT"

"$ROOT/wp-task-to-agent.sh" \
  --target codex \
  --plan "$PLAN" \
  --agent sigma \
  --mode fix

if [[ ! -f "$ARGS_OUT" ]]; then
  echo "FAIL: codex stub was not invoked"
  exit 1
fi

first_arg="$(sed -n '1p' "$ARGS_OUT")"
if [[ "$first_arg" != "exec" ]]; then
  echo "FAIL: expected codex exec, got: ${first_arg:-<empty>}"
  exit 1
fi

grep -q "fix" "$STDIN_OUT" || {
  echo "FAIL: fix prompt not delivered to codex"
  exit 1
}

grep -q "reviewer found" "$STDIN_OUT" || {
  echo "FAIL: prior review content not included in fix prompt"
  exit 1
}

test -f "$RECEIPT_OUT" || {
  echo "FAIL: dispatch receipt was not written"
  exit 1
}

grep -q '"ok": true' "$RECEIPT_OUT" || {
  echo "FAIL: receipt does not contain ok=true"
  exit 1
}

echo "PASS: wp-task-to-agent fix mode dispatches prompt with review context and writes receipt"
