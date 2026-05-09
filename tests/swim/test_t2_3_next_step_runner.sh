#!/usr/bin/env bash
# T2.3 — execute exactly one step at journal cursor.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

go test ./internal/swim/... -run 'ExecuteNextStep' -count=1

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

SCHEDULE="$TMP_DIR/schedule.json"
JOURNAL="$TMP_DIR/journal.json"

cat > "$SCHEDULE" <<'JSON'
{
  "schema_version": 2,
  "execution": [
    {
      "seq": 1,
      "step_id": "S1_T9.9_implement",
      "task_id": "T9.9",
      "action": "implement",
      "requires": {"task_status": "available"},
      "produces": {"task_status": "taken"},
      "invoke": {"argv": ["bash", "-lc", "true"]}
    }
  ]
}
JSON

go run ./cmd/swim-next --schedule "$SCHEDULE" --journal "$JOURNAL" > "$TMP_DIR/result.json"

jq -e '.outcome == "applied" and .cursor == 1 and .step_id == "S1_T9.9_implement"' "$TMP_DIR/result.json" >/dev/null
jq -e '.cursor == 1 and (.events | length) == 1 and .events[0].outcome == "applied"' "$JOURNAL" >/dev/null

echo "PASS: T2.3 next-step runner"
