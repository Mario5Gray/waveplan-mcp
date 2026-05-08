#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

PLAN="$ROOT_DIR/docs/plans/2026-05-05-swim-execution-waves.json"
AGENTS="$TMP_DIR/waveagents.json"
OUT="$TMP_DIR/out.json"

cat >"$AGENTS" <<'JSON'
{
  "agents": [
    {"name": "phi", "provider": "codex"},
    {"name": "sigma", "provider": "claude"}
  ],
  "schedule": ["phi", "sigma"]
}
JSON

"$ROOT_DIR/wp-emit-wave-execution.sh" --plan "$PLAN" --agents "$AGENTS" --out "$OUT" >/dev/null

# Top-level contract
test "$(jq -r '.schema_version' "$OUT")" = "2"

# Row required keys
jq -e '.execution[0] | has("seq") and has("action") and has("requires") and has("produces") and has("step_id") and has("task_id")' "$OUT" >/dev/null

# Action enum
jq -e '.execution | map(.action) | all(. as $a | ["implement","review","end_review","finish"] | index($a))' "$OUT" >/dev/null

# seq monotonic and unique
jq -e '.execution | [.[].seq] | . == (sort | unique)' "$OUT" >/dev/null

# Canonical requires/produces mapping
jq -e '.execution | map(select(.action=="implement")) | all(.requires.task_status=="available" and .produces.task_status=="taken")' "$OUT" >/dev/null
jq -e '.execution | map(select(.action=="review")) | all(.requires.task_status=="taken" and .produces.task_status=="review_taken")' "$OUT" >/dev/null
jq -e '.execution | map(select(.action=="end_review")) | all(.requires.task_status=="review_taken" and .produces.task_status=="review_ended")' "$OUT" >/dev/null
jq -e '.execution | map(select(.action=="finish")) | all(.requires.task_status=="review_ended" and .produces.task_status=="completed")' "$OUT" >/dev/null

# step_id shape
jq -e '.execution | all(.step_id | test("^S[0-9]+_T[0-9]+\\.[0-9]+_(implement|review|end_review|finish)$"))' "$OUT" >/dev/null

# Legacy field kept
jq -e '.execution | all(has("wp_invoke"))' "$OUT" >/dev/null

echo "PASS: T1.1 schedule v2 core fields"
