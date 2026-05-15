#!/usr/bin/env bash
# T7.3 — thread review-schedule through swim next/step/run subprocess invocations.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
CLI="$ROOT_DIR/waveplan-cli"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cd "$ROOT_DIR"
FIXTURE_DIR="tests/swim/fixtures"
SCHEDULE="$FIXTURE_DIR/review-loop-schedule.json"
REVIEW_SIDECAR="$FIXTURE_DIR/review-loop-sidecar.json"
STATE="$FIXTURE_DIR/review-loop-state.json"
JOURNAL="$FIXTURE_DIR/review-loop-journal.json"
JOURNAL_RUNTIME="$TMP_DIR/review-loop-journal.runtime.json"

jq --arg schedule "$ROOT_DIR/$SCHEDULE" '.schedule_path = $schedule' "$JOURNAL" >"$JOURNAL_RUNTIME"

"$CLI" swim next \
  --schedule "$SCHEDULE" \
  --review-schedule "$REVIEW_SIDECAR" \
  --state "$STATE" \
  --journal "$JOURNAL_RUNTIME" >"$TMP_DIR/next.json"
jq -e '.action == "ready" and .row.step_id == "S1_T1.1_fix_r1"' "$TMP_DIR/next.json" >/dev/null

"$CLI" swim step \
  --schedule "$SCHEDULE" \
  --review-schedule "$REVIEW_SIDECAR" \
  --state "$STATE" \
  --journal "$JOURNAL_RUNTIME" \
  --step-id "S1_T1.1_fix_r1" >"$TMP_DIR/step.json"
jq -e '.action == "ready" and .row.step_id == "S1_T1.1_fix_r1"' "$TMP_DIR/step.json" >/dev/null

"$CLI" swim run \
  --schedule "$SCHEDULE" \
  --review-schedule "$REVIEW_SIDECAR" \
  --state "$STATE" \
  --journal "$JOURNAL_RUNTIME" \
  --until "step:S1_T1.1_fix_r1" \
  --dry-run >"$TMP_DIR/run.json"
jq -e '.dry_run == true and .reached_until == true and (.steps[0].step_id == "S1_T1.1_fix_r1")' "$TMP_DIR/run.json" >/dev/null

"$CLI" swim journal \
  --schedule "$SCHEDULE" \
  --review-schedule "$REVIEW_SIDECAR" \
  --journal "$JOURNAL_RUNTIME" \
  --tail 1 >"$TMP_DIR/journal-view.json"
jq -e '.cursor == 2 and (.events | length == 0)' "$TMP_DIR/journal-view.json" >/dev/null
jq -e '(.merged_execution | map(.step_id) | index("S1_T1.1_fix_r1")) != null' "$TMP_DIR/journal-view.json" >/dev/null
jq -e '(.merged_execution | map(.step_id) | index("S1_T1.1_review_r2")) != null' "$TMP_DIR/journal-view.json" >/dev/null

echo "PASS: T7.3 review-schedule passthrough"
