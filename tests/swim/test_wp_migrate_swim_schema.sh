#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

OLD_SCHEDULE="$TMP/old-schedule.json"
NEW_SCHEDULE="$TMP/new-schedule.json"
OLD_JOURNAL="$TMP/old-journal.json"
NEW_JOURNAL="$TMP/new-journal.json"
STATE="$TMP/state.json"

cat >"$OLD_SCHEDULE" <<'JSON'
{
  "schema_version": 2,
  "execution": [
    {
      "seq": 1,
      "step_id": "S1_T1.1_implement",
      "task_id": "T1.1",
      "action": "implement",
      "requires": {"task_status": "available"},
      "produces": {"task_status": "taken"},
      "invoke": {"argv": ["wp-plan-to-agent.sh", "--mode", "implement", "--target", "codex", "--plan", "/tmp/plan.json", "--agent", "phi"]},
      "wp_invoke": "wp-plan-to-agent.sh --mode implement --target codex --plan /tmp/plan.json --agent phi",
      "status": "available"
    },
    {
      "seq": 2,
      "step_id": "S1_T1.1_review",
      "task_id": "T1.1",
      "action": "review",
      "requires": {"task_status": "taken"},
      "produces": {"task_status": "review_taken"},
      "invoke": {"argv": ["wp-plan-to-agent.sh", "--mode", "review", "--target", "claude", "--plan", "/tmp/plan.json", "--agent", "phi", "--reviewer", "sigma"]},
      "wp_invoke": "wp-plan-to-agent.sh --mode review --target claude --plan /tmp/plan.json --agent phi --reviewer sigma",
      "status": "taken"
    },
    {
      "seq": 3,
      "step_id": "S1_T1.1_end_review",
      "task_id": "T1.1",
      "action": "end_review",
      "requires": {"task_status": "review_taken"},
      "produces": {"task_status": "review_ended"},
      "invoke": {"argv": ["wp-plan-to-agent.sh", "--mode", "review_end", "--plan", "/tmp/plan.json", "--task-id", "T1.1", "--review-note", "looks good"]},
      "wp_invoke": "wp-plan-to-agent.sh --mode review_end --plan /tmp/plan.json --task-id T1.1 --review-note 'looks good'",
      "status": "review_taken"
    },
    {
      "seq": 4,
      "step_id": "S1_T1.1_finish",
      "task_id": "T1.1",
      "action": "finish",
      "requires": {"task_status": "review_ended"},
      "produces": {"task_status": "completed"},
      "invoke": {"argv": ["wp-plan-to-agent.sh", "--mode", "fin", "--plan", "/tmp/plan.json", "--task-id", "T1.1", "--git-sha", "DEFERRED"]},
      "wp_invoke": "wp-plan-to-agent.sh --mode fin --plan /tmp/plan.json --task-id T1.1 --git-sha DEFERRED",
      "status": "review_ended"
    }
  ]
}
JSON

cat >"$OLD_JOURNAL" <<JSON
{
  "schema_version": 1,
  "schedule_path": "$OLD_SCHEDULE",
  "cursor": 2,
  "events": []
}
JSON

cat >"$STATE" <<'JSON'
{
  "plan": "plan.json",
  "taken": {"T1.1": {"taken_by": "phi", "started_at": "2026-05-15 12:00", "review_entered_at": "2026-05-15 12:10", "reviewer": "sigma"}},
  "completed": {},
  "tail": {}
}
JSON

"$ROOT/wp-migrate-swim-schema.sh" \
  --schedule "$OLD_SCHEDULE" \
  --out "$NEW_SCHEDULE" \
  --journal "$OLD_JOURNAL" \
  --journal-out "$NEW_JOURNAL" \
  --state "$STATE"

jq -e '.schema_version == 3' "$NEW_SCHEDULE" >/dev/null
jq -e '.execution | length == 4' "$NEW_SCHEDULE" >/dev/null
jq -e '.execution[0].step_id == "S1_T1.1_implement" and .execution[0].seq == 1' "$NEW_SCHEDULE" >/dev/null
jq -e '.execution[0].operation == {"kind":"agent_dispatch","target":"codex","agent":"phi"}' "$NEW_SCHEDULE" >/dev/null
jq -e '.execution[1].operation == {"kind":"agent_dispatch","target":"claude","agent":"phi","reviewer":"sigma"}' "$NEW_SCHEDULE" >/dev/null
jq -e '.execution[2].operation.review_note == "looks good"' "$NEW_SCHEDULE" >/dev/null
jq -e '.execution[3].operation.git_sha == "DEFERRED"' "$NEW_SCHEDULE" >/dev/null
jq -e '.execution | all(.invoke.argv[0] == "wp-plan-step.sh")' "$NEW_SCHEDULE" >/dev/null
jq -e '.execution | all(.invoke.argv | index("--task-id") != null)' "$NEW_SCHEDULE" >/dev/null

python3 - "$NEW_SCHEDULE" <<'PY'
import json, shlex, sys
schedule = json.load(open(sys.argv[1], encoding="utf-8"))
for row in schedule["execution"]:
    if shlex.split(row["wp_invoke"]) != row["invoke"]["argv"]:
        raise SystemExit(f"wp_invoke parity failed for {row['step_id']}")
PY

# The compatibility string is intentionally defined by shlex.join/split.
# Runtime uses operation as the source of truth, not wp_invoke.

go run ./cmd/swim-validate --kind schedule --in "$NEW_SCHEDULE" >/dev/null
jq -e --arg p "$NEW_SCHEDULE" '.schedule_path == $p and .cursor == 2' "$NEW_JOURNAL" >/dev/null

echo "PASS: migrates legacy SWIM schedule to v3 without changing row identity"