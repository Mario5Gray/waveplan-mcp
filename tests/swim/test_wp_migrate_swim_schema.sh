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

# --- Negative test: missing dispatch flag ---
BAD_SCHEDULE="$TMP/bad-schedule.json"
BAD_OUT="$TMP/bad-out.json"
jq 'del(.execution[0].invoke.argv[3,4])' "$OLD_SCHEDULE" >"$BAD_SCHEDULE"

set +e
BAD_OUTPUT="$("$ROOT/wp-migrate-swim-schema.sh" --schedule "$BAD_SCHEDULE" --out "$BAD_OUT" 2>&1)"
BAD_STATUS=$?
set -e

if [[ "$BAD_STATUS" -eq 0 ]]; then
  echo "FAIL: migrator accepted implement row without --target"
  exit 1
fi
printf '%s' "$BAD_OUTPUT" | grep -q "missing dispatch flag" || {
  echo "FAIL: missing dispatch flag error was not reported"
  exit 1
}

# --- Negative test: contradictory action/mode ---
CONFLICT_SCHEDULE="$TMP/conflict-schedule.json"
jq '.execution[0].invoke.argv[2] = "fin"' "$OLD_SCHEDULE" >"$CONFLICT_SCHEDULE"

set +e
CONFLICT_OUTPUT="$("$ROOT/wp-migrate-swim-schema.sh" --schedule "$CONFLICT_SCHEDULE" --out "$TMP/conflict-out.json" 2>&1)"
CONFLICT_STATUS=$?
set -e

if [[ "$CONFLICT_STATUS" -eq 0 ]]; then
  echo "FAIL: migrator accepted contradictory action and --mode"
  exit 1
fi
printf '%s' "$CONFLICT_OUTPUT" | grep -q "contradicts legacy --mode" || {
  echo "FAIL: contradictory action/mode error was not reported"
  exit 1
}

# --- Negative test: already-v3 idempotency ---
V3_AGAIN="$TMP/v3-again.json"
"$ROOT/wp-migrate-swim-schema.sh" --schedule "$NEW_SCHEDULE" --out "$V3_AGAIN"

python3 - "$NEW_SCHEDULE" "$V3_AGAIN" <<'PY'
import json, sys
left = json.load(open(sys.argv[1], encoding="utf-8"))
right = json.load(open(sys.argv[2], encoding="utf-8"))
if left != right:
    raise SystemExit("already-v3 migration was not idempotent")
PY

# --- Negative test: state-transition dispatch flag warning ---
LEAK_SCHEDULE="$TMP/leak-schedule.json"
jq '.execution[2].invoke.argv += ["--target", "codex", "--agent", "phi"]' "$OLD_SCHEDULE" >"$LEAK_SCHEDULE"
"$ROOT/wp-migrate-swim-schema.sh" --schedule "$LEAK_SCHEDULE" --out "$TMP/leak-out.json" 2>"$TMP/leak.err"

grep -q "dropping dispatch flag" "$TMP/leak.err" || {
  echo "FAIL: expected warning for stripped dispatch flags on state_transition row"
  exit 1
}
jq -e '.execution[2].operation.kind == "state_transition" and (.execution[2].operation.target? | not) and (.execution[2].operation.agent? | not)' "$TMP/leak-out.json" >/dev/null

# --- Negative test: pending unknown journal event ---
UNKNOWN_JOURNAL="$TMP/unknown-journal.json"
cat >"$UNKNOWN_JOURNAL" <<JSON
{
  "schema_version": 1,
  "schedule_path": "$OLD_SCHEDULE",
  "cursor": 1,
  "events": [
    {
      "event_id": "E0002",
      "step_id": "S1_T1.1_review",
      "seq": 2,
      "task_id": "T1.1",
      "action": "review",
      "attempt": 1,
      "started_on": "2026-05-15T12:00:00Z",
      "outcome": "unknown",
      "state_before": {"task_status": "taken"},
      "state_after": {"task_status": "taken"}
    }
  ]
}
JSON

set +e
UNKNOWN_OUTPUT="$("$ROOT/wp-migrate-swim-schema.sh" --schedule "$OLD_SCHEDULE" --out "$TMP/unknown-out.json" --journal "$UNKNOWN_JOURNAL" 2>&1)"
UNKNOWN_STATUS=$?
set -e

if [[ "$UNKNOWN_STATUS" -eq 0 ]]; then
  echo "FAIL: migrator accepted pending unknown journal event"
  exit 1
fi
printf '%s' "$UNKNOWN_OUTPUT" | grep -q "pending unknown event" || {
  echo "FAIL: pending unknown error was not reported"
  exit 1
}

echo "PASS: all negative migration tests"