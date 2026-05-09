#!/usr/bin/env bash
# T4.2 — wired swim CLI handlers.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
CLI="$ROOT_DIR/waveplan-cli"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cat >"$TMP_DIR/waveagents.json" <<'JSON'
{
  "agents": [
    {"name": "phi", "provider": "codex"},
    {"name": "sigma", "provider": "claude"}
  ],
  "schedule": ["phi", "sigma"]
}
JSON

"$CLI" swim compile-schedule \
  --plan "$ROOT_DIR/docs/plans/2026-05-05-swim-execution-waves.json" \
  --agents "$TMP_DIR/waveagents.json" >"$TMP_DIR/schedule.json"
jq -e '.schema_version == 2 and (.execution | length > 0)' "$TMP_DIR/schedule.json" >/dev/null

cat >"$TMP_DIR/state.json" <<'JSON'
{
  "plan": "demo.json",
  "taken": {},
  "completed": {}
}
JSON

WAVEPLAN_STATE="$TMP_DIR/state.json" \
  "$CLI" swim next --schedule "$ROOT_DIR/tests/swim/fixtures/expected-schedule.json" >"$TMP_DIR/next.json"
jq -e '.action == "ready" and .row.step_id == "S1_T1.1_implement"' "$TMP_DIR/next.json" >/dev/null

cat >"$TMP_DIR/apply-state.json" <<'JSON'
{
  "plan": "demo.json",
  "taken": {},
  "completed": {}
}
JSON

cat >"$TMP_DIR/apply-schedule.json" <<JSON
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
      "invoke": {
        "argv": [
          "/bin/sh",
          "-c",
          "python3 - <<'PY'\nimport json\npath = r'$TMP_DIR/apply-state.json'\nwith open(path, 'r', encoding='utf-8') as f:\n    data = json.load(f)\ndata['taken']['T1.1'] = {'taken_by': 'phi', 'started_at': '2026-05-09T00:00:00Z'}\nwith open(path, 'w', encoding='utf-8') as f:\n    json.dump(data, f)\nPY"
        ]
      }
    }
  ]
}
JSON

"$CLI" swim step --apply \
  --schedule "$TMP_DIR/apply-schedule.json" \
  --journal "$TMP_DIR/apply-journal.json" \
  --state "$TMP_DIR/apply-state.json" >"$TMP_DIR/apply.json"
jq -e '.status == "applied" and .step_id == "S1_T1.1_implement"' "$TMP_DIR/apply.json" >/dev/null

cat >"$TMP_DIR/unknown-journal.json" <<'JSON'
{
  "schema_version": 1,
  "schedule_path": "demo-schedule.json",
  "cursor": 0,
  "events": [
    {
      "event_id": "E0001",
      "step_id": "S1_T1.1_implement",
      "seq": 1,
      "task_id": "T1.1",
      "action": "implement",
      "attempt": 1,
      "started_on": "2026-05-09T00:00:00Z",
      "completed_on": "2026-05-09T00:00:01Z",
      "outcome": "unknown",
      "state_before": {"task_status": "available"},
      "state_after": {"task_status": "taken"}
    }
  ]
}
JSON

"$CLI" swim step --journal "$TMP_DIR/unknown-journal.json" \
  --ack-unknown S1_T1.1_implement --as waived >"$TMP_DIR/ack-waived.json"
jq -e '.ok == true and .outcome == "waived"' "$TMP_DIR/ack-waived.json" >/dev/null
jq -e '.events[0].outcome == "waived"' "$TMP_DIR/unknown-journal.json" >/dev/null

cat >"$TMP_DIR/unknown-journal-retry.json" <<'JSON'
{
  "schema_version": 1,
  "schedule_path": "demo-schedule.json",
  "cursor": 0,
  "events": [
    {
      "event_id": "E0001",
      "step_id": "S1_T1.1_implement",
      "seq": 1,
      "task_id": "T1.1",
      "action": "implement",
      "attempt": 1,
      "started_on": "2026-05-09T00:00:00Z",
      "completed_on": "2026-05-09T00:00:01Z",
      "outcome": "unknown",
      "state_before": {"task_status": "available"},
      "state_after": {"task_status": "taken"}
    }
  ]
}
JSON

"$CLI" swim step --journal "$TMP_DIR/unknown-journal-retry.json" \
  --ack-unknown S1_T1.1_implement --as failed >"$TMP_DIR/ack-failed.json"
jq -e '.ok == true and .outcome == "failed"' "$TMP_DIR/ack-failed.json" >/dev/null
jq -e '.events[0].outcome == "failed"' "$TMP_DIR/unknown-journal-retry.json" >/dev/null

"$CLI" swim run \
  --schedule "$ROOT_DIR/tests/swim/fixtures/expected-schedule.json" \
  --state "$TMP_DIR/state.json" \
  --until finish --dry-run >"$TMP_DIR/run.json"
jq -e '.dry_run == true and (.steps | length == 4) and .steps[0].status == "would_apply"' "$TMP_DIR/run.json" >/dev/null

cat >"$TMP_DIR/journal-tail.json" <<'JSON'
{
  "schema_version": 1,
  "schedule_path": "demo-schedule.json",
  "cursor": 5,
  "events": [
    {"event_id":"E0001","step_id":"S1_T1.1_implement","seq":1,"task_id":"T1.1","action":"implement","attempt":1,"started_on":"2026-05-09T00:00:00Z","completed_on":"2026-05-09T00:00:01Z","outcome":"applied","state_before":{"task_status":"available"},"state_after":{"task_status":"taken"}},
    {"event_id":"E0002","step_id":"S1_T1.1_review","seq":2,"task_id":"T1.1","action":"review","attempt":1,"started_on":"2026-05-09T00:00:00Z","completed_on":"2026-05-09T00:00:01Z","outcome":"applied","state_before":{"task_status":"taken"},"state_after":{"task_status":"review_taken"}},
    {"event_id":"E0003","step_id":"S1_T1.1_end_review","seq":3,"task_id":"T1.1","action":"end_review","attempt":1,"started_on":"2026-05-09T00:00:00Z","completed_on":"2026-05-09T00:00:01Z","outcome":"applied","state_before":{"task_status":"review_taken"},"state_after":{"task_status":"review_ended"}},
    {"event_id":"E0004","step_id":"S1_T1.1_finish","seq":4,"task_id":"T1.1","action":"finish","attempt":1,"started_on":"2026-05-09T00:00:00Z","completed_on":"2026-05-09T00:00:01Z","outcome":"applied","state_before":{"task_status":"review_ended"},"state_after":{"task_status":"completed"}},
    {"event_id":"E0005","step_id":"S1_T1.2_implement","seq":5,"task_id":"T1.2","action":"implement","attempt":1,"started_on":"2026-05-09T00:00:00Z","completed_on":"2026-05-09T00:00:01Z","outcome":"applied","state_before":{"task_status":"available"},"state_after":{"task_status":"taken"}}
  ]
}
JSON

"$CLI" swim journal --schedule "$TMP_DIR/apply-schedule.json" --journal "$TMP_DIR/journal-tail.json" --tail 3 >"$TMP_DIR/journal.json"
jq -e '.cursor == 5 and (.events | length == 3) and .events[0].event_id == "E0003"' "$TMP_DIR/journal.json" >/dev/null

"$CLI" swim validate --kind schedule --in "$ROOT_DIR/tests/swim/fixtures/expected-schedule.json" >"$TMP_DIR/validate.json"
jq -e '.ok == true and .kind == "schedule"' "$TMP_DIR/validate.json" >/dev/null

echo "PASS: T4.2 cli wired"
