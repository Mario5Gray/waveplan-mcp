#!/usr/bin/env bash
# T1.4 — schema artifacts exist and schedule output validates.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
OUT="$TMP_DIR/schedule.json"
JOURNAL="$TMP_DIR/journal.json"

python3 -c "import json; json.load(open('docs/specs/swim-schedule-schema-v2.json', encoding='utf-8'))"
python3 -c "import json; json.load(open('docs/specs/swim-journal-schema-v1.json', encoding='utf-8'))"

bash wp-emit-wave-execution.sh \
  --plan docs/plans/2026-05-05-swim-execution-waves.json \
  --agents tests/swim/fixtures/waveagents.json \
  --task-scope all > "$OUT"

go run ./cmd/swim-validate --kind schedule --in "$OUT" >/dev/null

cat > "$JOURNAL" <<'JSON'
{
  "schema_version": 1,
  "schedule_path": "docs/plans/2026-05-05-swim-execution-waves.json",
  "cursor": 1,
  "last_event": {
    "event_id": "E0001",
    "step_id": "S1_T1.1_implement",
    "seq": 1,
    "task_id": "T1.1",
    "action": "implement",
    "attempt": 1,
    "started_on": "2026-05-05T12:00:00Z",
    "completed_on": "2026-05-05T12:00:01Z",
    "outcome": "applied",
    "state_before": {"task_status": "available"},
    "state_after": {"task_status": "taken"}
  },
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
      "outcome": "applied",
      "state_before": {"task_status": "available"},
      "state_after": {"task_status": "taken"}
    }
  ]
}
JSON

go run ./cmd/swim-validate --kind journal --in "$JOURNAL" >/dev/null

echo "PASS: T1.4 schema artifacts"
