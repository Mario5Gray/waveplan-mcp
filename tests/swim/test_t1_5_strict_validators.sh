#!/usr/bin/env bash
# T1.5 — strict validators and golden tests.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

go test ./internal/swim/... -count=1

go run ./cmd/swim-validate --kind schedule --in tests/swim/fixtures/expected-schedule.json >/dev/null

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

BAD_SCHEDULE="$TMP_DIR/bad-schedule.json"
BAD_JOURNAL="$TMP_DIR/bad-journal.json"

jq '.execution[1].step_id = .execution[0].step_id' tests/swim/fixtures/expected-schedule.json > "$BAD_SCHEDULE"
if go run ./cmd/swim-validate --kind schedule --in "$BAD_SCHEDULE" >/dev/null 2>&1; then
  echo "FAIL: expected bad schedule to be rejected"
  exit 1
fi

cat > "$BAD_JOURNAL" <<'JSON'
{
  "schema_version": 1,
  "schedule_path": "docs/plans/2026-05-05-swim-execution-waves.json",
  "cursor": 1,
  "events": [
    {
      "event_id": "E0001",
      "step_id": "S1_T1.1_implement",
      "seq": 1,
      "task_id": "T1.1",
      "action": "implement",
      "attempt": 1,
      "started_on": "2026-05-05T12:00:00Z",
      "completed_on": "2026-05-05T12:00:01Z",
      "outcome": "waived",
      "state_before": {"task_status": "available"},
      "state_after": {"task_status": "taken"},
      "reason": "operator intentionally omitted",
      "waived_on": "2026-05-05T12:00:02Z"
    }
  ]
}
JSON

if go run ./cmd/swim-validate --kind journal --in "$BAD_JOURNAL" >/dev/null 2>&1; then
  echo "FAIL: expected bad journal to be rejected"
  exit 1
fi

echo "PASS: T1.5 strict validators"
