#!/usr/bin/env bash
# T7.2 — deterministic swim insert-fix-loop sidecar writer.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
CLI="$ROOT_DIR/waveplan-cli"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
unset WAVEPLAN_SCHED_REVIEW

cat >"$TMP_DIR/schedule.json" <<'JSON'
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
      "invoke": {"argv": ["wp-plan-to-agent.sh", "--mode", "implement", "--target", "codex", "--plan", "demo-plan.json", "--agent", "phi"]}
    },
    {
      "seq": 2,
      "step_id": "S1_T1.1_review",
      "task_id": "T1.1",
      "action": "review",
      "requires": {"task_status": "taken"},
      "produces": {"task_status": "review_taken"},
      "invoke": {"argv": ["wp-plan-to-agent.sh", "--mode", "review", "--target", "claude", "--plan", "demo-plan.json", "--agent", "phi", "--reviewer", "sigma"]}
    },
    {
      "seq": 3,
      "step_id": "S1_T1.1_end_review",
      "task_id": "T1.1",
      "action": "end_review",
      "requires": {"task_status": "review_taken"},
      "produces": {"task_status": "review_ended"},
      "invoke": {"argv": ["wp-plan-to-agent.sh", "--mode", "review_end", "--plan", "demo-plan.json", "--task-id", "T1.1", "--review-note", "${WP_COMMENT:-}"]}
    },
    {
      "seq": 4,
      "step_id": "S1_T1.1_finish",
      "task_id": "T1.1",
      "action": "finish",
      "requires": {"task_status": "review_ended"},
      "produces": {"task_status": "completed"},
      "invoke": {"argv": ["wp-plan-to-agent.sh", "--mode", "fin", "--plan", "demo-plan.json", "--task-id", "T1.1", "--git-sha", "${GIT_SHA:-DEFERRED}"]}
    }
  ]
}
JSON

"$CLI" swim insert-fix-loop --help >"$TMP_DIR/insert-help.txt"
grep -q -- '--schedule' "$TMP_DIR/insert-help.txt"
grep -q -- '--review-schedule' "$TMP_DIR/insert-help.txt"
grep -q -- '--task' "$TMP_DIR/insert-help.txt"
grep -q -- '--after-step' "$TMP_DIR/insert-help.txt"
grep -q -- '--round' "$TMP_DIR/insert-help.txt"

set +e
"$CLI" swim insert-fix-loop \
  --schedule "$TMP_DIR/schedule.json" \
  --task T1.1 \
  --after-step S1_T1.1_review >"$TMP_DIR/missing-review.json" 2>"$TMP_DIR/missing-review.err"
status=$?
set -e
test "$status" -ne 0
jq -e '.ok == false and .subcommand == "insert-fix-loop" and (.error | test("missing review schedule"))' \
  "$TMP_DIR/missing-review.json" >/dev/null

"$CLI" swim insert-fix-loop \
  --schedule "$TMP_DIR/schedule.json" \
  --review-schedule "$TMP_DIR/review-sidecar.json" \
  --task T1.1 \
  --after-step S1_T1.1_review >"$TMP_DIR/insert-1.json"

jq -e '.ok == true and .subcommand == "insert-fix-loop" and .round == 1 and (.inserted | length == 2)' \
  "$TMP_DIR/insert-1.json" >/dev/null
jq -e '.schema_version == 1 and (.insertions | length == 2)' "$TMP_DIR/review-sidecar.json" >/dev/null
jq -e '.insertions[0].id == "X1" and .insertions[0].after_step_id == "S1_T1.1_review" and .insertions[0].step_id == "S1_T1.1_fix_r1" and .insertions[0].action == "fix" and .insertions[0].source_event_id == "E0001"' \
  "$TMP_DIR/review-sidecar.json" >/dev/null
jq -e '.insertions[0].invoke.argv | index("--mode") as $i | $i != null and .[$i+1] == "fix"' \
  "$TMP_DIR/review-sidecar.json" >/dev/null
jq -e '.insertions[1].id == "X2" and .insertions[1].after_step_id == "X1" and .insertions[1].step_id == "S1_T1.1_review_r2" and .insertions[1].action == "review" and .insertions[1].source_event_id == "E0002"' \
  "$TMP_DIR/review-sidecar.json" >/dev/null
jq -e '.insertions[1].invoke.argv | index("--mode") as $i | $i != null and .[$i+1] == "review"' \
  "$TMP_DIR/review-sidecar.json" >/dev/null

"$CLI" swim insert-fix-loop \
  --schedule "$TMP_DIR/schedule.json" \
  --review-schedule "$TMP_DIR/review-sidecar.json" \
  --task T1.1 \
  --after-step X2 >"$TMP_DIR/insert-2.json"
jq -e '.ok == true and .round == 2 and .inserted[0].id == "X3" and .inserted[1].id == "X4"' \
  "$TMP_DIR/insert-2.json" >/dev/null
jq -e '.insertions[2].step_id == "S1_T1.1_fix_r2" and .insertions[2].source_event_id == "E0003" and .insertions[3].step_id == "S1_T1.1_review_r3" and .insertions[3].source_event_id == "E0004"' \
  "$TMP_DIR/review-sidecar.json" >/dev/null

"$CLI" swim insert-fix-loop \
  --schedule "$TMP_DIR/schedule.json" \
  --review-schedule "$TMP_DIR/review-sidecar.json" \
  --task T1.1 \
  --after-step X4 \
  --round 5 >"$TMP_DIR/insert-3.json"
jq -e '.ok == true and .round == 5 and .inserted[0].id == "X5" and .inserted[1].id == "X6"' \
  "$TMP_DIR/insert-3.json" >/dev/null
jq -e '.insertions[4].step_id == "S1_T1.1_fix_r5" and .insertions[5].step_id == "S1_T1.1_review_r6"' \
  "$TMP_DIR/review-sidecar.json" >/dev/null

cat >"$TMP_DIR/wrong-base-sidecar.json" <<'JSON'
{
  "schema_version": 1,
  "base_schedule_path": "/tmp/other-schedule.json",
  "insertions": []
}
JSON
set +e
"$CLI" swim insert-fix-loop \
  --schedule "$TMP_DIR/schedule.json" \
  --review-schedule "$TMP_DIR/wrong-base-sidecar.json" \
  --task T1.1 \
  --after-step S1_T1.1_review >"$TMP_DIR/base-mismatch.json" 2>"$TMP_DIR/base-mismatch.err"
status=$?
set -e
test "$status" -ne 0
jq -e '.ok == false and .subcommand == "insert-fix-loop" and (.error | test("base_schedule_path"))' \
  "$TMP_DIR/base-mismatch.json" >/dev/null

echo "PASS: T7.2 insert-fix-loop"
