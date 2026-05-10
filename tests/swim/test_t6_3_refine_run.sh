#!/usr/bin/env bash
# T6.3 — exercise RefineRun via the swim-refine-run binary and the Go unit
# suite. Verifies the locked semantics: fine steps run in array order, parent
# rolls up only when all targeted children are terminal AND coarse
# postcondition is satisfied, dry-run never mutates, lock_busy surfaces with
# holder hint, and reruns are idempotent no-ops.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

# 1) Go unit tests for the refine_run package
go test ./internal/swim/... -run 'RefineRun' -count=1

# 2) Build the binary so exit codes propagate verbatim
TMP="$(mktemp -d)"
trap "rm -rf $TMP" EXIT
BIN="$TMP/swim-refine-run"
go build -o "$BIN" ./cmd/swim-refine-run

# 3) Build a tiny coarse plan + matching refinement sidecar in $TMP
cat >"$TMP/coarse.json" <<'JSON'
{
  "schema_version": 1,
  "generated_on": "2026-05-09",
  "plan_version": 1,
  "plan_generation": "2026-05-09T00:00:00Z",
  "plan": {"id": "t6-3-test"},
  "fp_index": {},
  "doc_index": {},
  "tasks": {
    "T1": {"title": "task one", "files": ["a.go", "b.go"]}
  },
  "units": {
    "T1.1": {"task": "T1", "title": "u1", "kind": "impl", "wave": 1, "plan_line": 1, "depends_on": []}
  }
}
JSON

cat >"$TMP/refine.json" <<JSON
{
  "schema_version": 1,
  "coarse_plan": "$TMP/coarse.json",
  "profile": "8k",
  "generated_on": "2026-05-09",
  "targets": ["T1.1"],
  "units": [
    {
      "parent_unit": "T1.1",
      "step_id": "F1_T1.1_s1",
      "seq": 1,
      "context_budget": "8k",
      "depends_on": [],
      "requires": {"task_status": "taken"},
      "produces": {"task_status": "review_taken"},
      "invoke": {"argv": ["true"]}
    }
  ]
}
JSON

# 4) Initial state has T1.1 already in review_taken (simulates the post-fine
#    coarse state observed externally). Rollup gate validates against this.
cat >"$TMP/state.json" <<'JSON'
{
  "plan": "demo",
  "taken": {
    "T1.1": {
      "taken_by": "phi",
      "started_at": "2026-05-09 10:00",
      "review_entered_at": "2026-05-09 10:30"
    }
  },
  "completed": {}
}
JSON

# 5) Dry-run does not mutate journal or state
"$BIN" --refine "$TMP/refine.json" --state "$TMP/state.json" --dry-run >"$TMP/dry.json"
jq -e '.dry_run == true and .steps[0].would_apply == true' "$TMP/dry.json" >/dev/null \
  || { echo "FAIL: dry-run did not surface would_apply"; cat "$TMP/dry.json"; exit 1; }
test ! -e "$TMP/refine.journal.json" \
  || { echo "FAIL: dry-run wrote a fine journal"; exit 1; }

# 6) Live run with a real /bin/true invoke + coarse-journal path → rollup fires
"$BIN" \
  --refine "$TMP/refine.json" \
  --refine-journal "$TMP/refine.journal.json" \
  --coarse-journal "$TMP/coarse.journal.json" \
  --state "$TMP/state.json" >"$TMP/run1.json"

jq -e '.stopped == "done" and (.parents_completed | length) == 1 and .parents_completed[0] == "T1.1"' \
  "$TMP/run1.json" >/dev/null \
  || { echo "FAIL: rollup did not fire"; cat "$TMP/run1.json"; exit 1; }

# 7) Coarse journal contains the synthetic rollup event
jq -e '.events | map(select(.step_id == "PARENT_T1.1_rollup")) | length == 1' \
  "$TMP/coarse.journal.json" >/dev/null \
  || { echo "FAIL: synthetic rollup event missing in coarse journal"; cat "$TMP/coarse.journal.json"; exit 1; }

# 8) Idempotent rerun — no new events, no rollup
"$BIN" \
  --refine "$TMP/refine.json" \
  --refine-journal "$TMP/refine.journal.json" \
  --coarse-journal "$TMP/coarse.journal.json" \
  --state "$TMP/state.json" >"$TMP/run2.json"

jq -e '(.steps | length) == 0 and (.parents_completed | length) == 0 and .protocol_note == "idempotent_noop"' \
  "$TMP/run2.json" >/dev/null \
  || { echo "FAIL: rerun was not an idempotent no-op"; cat "$TMP/run2.json"; exit 1; }

# 9) Coarse journal still contains exactly one rollup event
jq -e '.events | map(select(.step_id == "PARENT_T1.1_rollup")) | length == 1' \
  "$TMP/coarse.journal.json" >/dev/null \
  || { echo "FAIL: rollup duplicated on rerun"; exit 1; }

# 10) Missing required arg produces exit 2
set +e
"$BIN" --state "$TMP/state.json" >/dev/null 2>"$TMP/err.no-refine"
status=$?
set -e
if [[ "$status" -ne 2 ]]; then
  echo "FAIL: expected exit 2 on missing --refine, got $status"
  exit 1
fi

echo "PASS: T6.3 refine-run"
