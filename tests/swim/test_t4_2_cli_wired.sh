#!/usr/bin/env bash
# T4.2 — wired swim CLI handlers.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
CLI="$ROOT_DIR/waveplan-cli"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
export XDG_CONFIG_HOME="$TMP_DIR/xdg"
mkdir -p "$XDG_CONFIG_HOME/waveplan-mcp"

cat >"$XDG_CONFIG_HOME/waveplan-mcp/waveagents.json" <<'JSON'
{
  "agents": [
    {"name": "phi", "provider": "codex"},
    {"name": "sigma", "provider": "claude"}
  ],
  "schedule": ["phi", "sigma"]
}
JSON

"$CLI" swim compile-schedule \
  --plan "$ROOT_DIR/docs/plans/2026-05-05-swim-execution-waves.json" >"$TMP_DIR/schedule.json"
jq -e '.schema_version == 2 and (.execution | length > 0)' "$TMP_DIR/schedule.json" >/dev/null

BOOT_PLAN="$TMP_DIR/bootstrap-execution-waves.json"
cat >"$BOOT_PLAN" <<'JSON'
{
  "schema_version": 1,
  "generated_on": "2026-05-05",
  "plan": {"name": "boot"},
  "doc_index": {},
  "fp_index": {},
  "tasks": {"T1": {"title": "Base task", "plan_line": 1, "doc_refs": []}},
  "units": {"T1.1": {"task": "T1", "title": "Base unit", "kind": "impl", "wave": 1, "plan_line": 2, "depends_on": [], "doc_refs": [], "fp_refs": []}},
  "waves": [{"wave": 1, "units": ["T1.1"]}]
}
JSON

"$CLI" swim compile-schedule \
  --plan "$BOOT_PLAN" \
  --out "$TMP_DIR/bootstrap-schedule.json" \
  --bootstrap-state >"$TMP_DIR/bootstrap-report.json"
jq -e '.ok == true and .state_bootstrapped == true' "$TMP_DIR/bootstrap-report.json" >/dev/null
test -f "$BOOT_PLAN.state.json"

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

"$CLI" swim next \
  --schedule "$ROOT_DIR/tests/swim/fixtures/expected-schedule.json" \
  --state "$TMP_DIR/missing-state.json" >"$TMP_DIR/next-missing-state.json"
jq -e '.action == "ready" and .row.step_id == "S1_T1.1_implement"' "$TMP_DIR/next-missing-state.json" >/dev/null

cat >"$TMP_DIR/apply-state.json" <<'JSON'
{
  "plan": "demo.json",
  "taken": {},
  "completed": {}
}
JSON

cat >"$TMP_DIR/apply_mutate.py" <<PY
import json, os, pathlib
path = pathlib.Path(r"$TMP_DIR/apply-state.json")
data = json.loads(path.read_text(encoding="utf-8"))
data["taken"]["T1.1"] = {"taken_by": "phi", "started_at": "2026-05-09T00:00:00Z"}
path.write_text(json.dumps(data), encoding="utf-8")
receipt = pathlib.Path(os.environ["SWIM_DISPATCH_RECEIPT_PATH"])
receipt.parent.mkdir(parents=True, exist_ok=True)
receipt.write_text(json.dumps({"ok": True}) + "\\n", encoding="utf-8")
PY

python3 - <<PY
import json, shlex
tmp = r"$TMP_DIR"
cmd = "python3 " + shlex.quote(f"{tmp}/apply_mutate.py")
schedule = {
    "schema_version": 2,
    "execution": [{
        "seq": 1,
        "step_id": "S1_T1.1_implement",
        "task_id": "T1.1",
        "action": "implement",
        "requires": {"task_status": "available"},
        "produces": {"task_status": "taken"},
        "invoke": {"argv": ["/bin/sh", "-c", cmd]},
    }],
}
with open(f"{tmp}/apply-schedule.json", "w", encoding="utf-8") as f:
    json.dump(schedule, f, indent=2)
    f.write("\\n")
PY

if ! "$CLI" swim step --apply \
  --schedule "$TMP_DIR/apply-schedule.json" \
  --journal "$TMP_DIR/apply-journal.json" \
  --state "$TMP_DIR/apply-state.json" >"$TMP_DIR/apply.json" 2>"$TMP_DIR/apply.stderr"; then
  cat "$TMP_DIR/apply.json" >&2 || true
  cat "$TMP_DIR/apply.stderr" >&2 || true
  exit 1
fi
jq -e '.status == "applied" and .step_id == "S1_T1.1_implement"' "$TMP_DIR/apply.json" >/dev/null

cat >"$TMP_DIR/apply-tail-state.json" <<'JSON'
{
  "plan": "demo.json",
  "taken": {},
  "completed": {}
}
JSON

cat >"$TMP_DIR/apply_tail_mutate.py" <<PY
import json, os, pathlib
path = pathlib.Path(r"$TMP_DIR/apply-tail-state.json")
data = json.loads(path.read_text(encoding="utf-8"))
data["taken"]["T1.2"] = {"taken_by": "phi", "started_at": "2026-05-09T00:00:00Z"}
path.write_text(json.dumps(data), encoding="utf-8")
receipt = pathlib.Path(os.environ["SWIM_DISPATCH_RECEIPT_PATH"])
receipt.parent.mkdir(parents=True, exist_ok=True)
receipt.write_text(json.dumps({"ok": True}) + "\\n", encoding="utf-8")
PY

python3 - <<PY
import json, shlex
tmp = r"$TMP_DIR"
cmd = "printf 'tail-stdout\\n'; printf 'tail-stderr\\n' >&2; python3 " + shlex.quote(f"{tmp}/apply_tail_mutate.py")
schedule = {
    "schema_version": 2,
    "execution": [{
        "seq": 1,
        "step_id": "S1_T1.2_implement",
        "task_id": "T1.2",
        "action": "implement",
        "requires": {"task_status": "available"},
        "produces": {"task_status": "taken"},
        "invoke": {"argv": ["/bin/sh", "-c", cmd]},
    }],
}
with open(f"{tmp}/apply-tail-schedule.json", "w", encoding="utf-8") as f:
    json.dump(schedule, f, indent=2)
    f.write("\\n")
PY

"$CLI" swim step --apply --tail-logs --tail-lines 1 \
  --schedule "$TMP_DIR/apply-tail-schedule.json" \
  --journal "$TMP_DIR/apply-tail-journal.json" \
  --state "$TMP_DIR/apply-tail-state.json" \
  >"$TMP_DIR/apply-tail.json" 2>"$TMP_DIR/apply-tail.stderr"
jq -e '.status == "applied" and .step_id == "S1_T1.2_implement"' "$TMP_DIR/apply-tail.json" >/dev/null
grep -q 'stdout:' "$TMP_DIR/apply-tail.stderr" || { echo "FAIL: missing stdout tail header"; exit 1; }
grep -q 'stderr:' "$TMP_DIR/apply-tail.stderr" || { echo "FAIL: missing stderr tail header"; exit 1; }
grep -q 'tail-stdout' "$TMP_DIR/apply-tail.stderr" || { echo "FAIL: missing tailed stdout content"; exit 1; }
grep -q 'tail-stderr' "$TMP_DIR/apply-tail.stderr" || { echo "FAIL: missing tailed stderr content"; exit 1; }

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
