#!/usr/bin/env bash
# swim-adopt-drift -- force-adopt a cursor_drift row so swim run can continue.
#
# Use when: swim run blocks with cursor_drift because a review agent called
# end_review during its dispatch, advancing state past the expected postcondition.
#
# What it does:
#   1. Verifies the current cursor row is actually in cursor_drift.
#   2. Appends an idempotent_adopt applied event for that row.
#   3. Advances the journal cursor by 1.
#   4. Re-checks: next row should now be drift with actual==Predict (auto-adopt).
#
# Usage:
#   swim-adopt-drift.sh --schedule SCHED --journal JOURNAL --state STATE [--dry-run]
set -euo pipefail

SCHEDULE=""
JOURNAL=""
STATE=""
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --schedule) SCHEDULE="${2:-}"; shift 2 ;;
    --journal)  JOURNAL="${2:-}";  shift 2 ;;
    --state)    STATE="${2:-}";    shift 2 ;;
    --dry-run)  DRY_RUN=1; shift ;;
    -h|--help)
      sed -n '/^# /s/^# //p' "$0"
      exit 0 ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$SCHEDULE" || -z "$JOURNAL" || -z "$STATE" ]]; then
  echo "Missing required: --schedule --journal --state" >&2
  exit 2
fi

for f in "$SCHEDULE" "$JOURNAL" "$STATE"; do
  if [[ ! -f "$f" ]]; then
    echo "File not found: $f" >&2
    exit 2
  fi
done

# Read current decision from swim-step.
DECISION="$(swim-step --schedule "$SCHEDULE" --journal "$JOURNAL" --state "$STATE" 2>&1 || true)"
ACTION="$(printf '%s' "$DECISION" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("action",""))' 2>/dev/null || true)"
CODE="$(printf '%s' "$DECISION" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("code",""))' 2>/dev/null || true)"

if [[ "$CODE" != "cursor_drift" ]]; then
  echo "Not a cursor_drift situation (action=$ACTION code=$CODE). Nothing to do." >&2
  echo "$DECISION"
  exit 1
fi

STEP_ID="$(printf '%s' "$DECISION" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("row",{}).get("step_id",""))')"
SEQ="$(printf '%s' "$DECISION" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("row",{}).get("seq",""))')"
TASK_ID="$(printf '%s' "$DECISION" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("row",{}).get("task_id",""))')"
ACTION_NAME="$(printf '%s' "$DECISION" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("row",{}).get("action",""))')"
REQUIRES="$(printf '%s' "$DECISION" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("row",{}).get("requires",{}).get("task_status",""))')"
PRODUCES="$(printf '%s' "$DECISION" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("row",{}).get("produces",{}).get("task_status",""))')"
CURSOR="$(printf '%s' "$DECISION" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("cursor",""))')"

echo "cursor_drift detected:"
echo "  step:    $STEP_ID  (seq $SEQ)"
echo "  task:    $TASK_ID"
echo "  action:  $ACTION_NAME  requires=$REQUIRES produces=$PRODUCES"
echo "  cursor:  $CURSOR  →  $((CURSOR+1))"

if [[ "$DRY_RUN" == "1" ]]; then
  echo "[dry-run] would append idempotent_adopt event and advance cursor to $((CURSOR+1))"
  exit 0
fi

# Backup journal before mutating.
BACKUP="${JOURNAL}.bak-$(date +%Y%m%d-%H%M%S)"
cp "$JOURNAL" "$BACKUP"
echo "  backup:  $BACKUP"

python3 - "$JOURNAL" "$STEP_ID" "$SEQ" "$TASK_ID" "$ACTION_NAME" "$REQUIRES" "$PRODUCES" <<'PYEOF'
import json, sys, time

journal_path, step_id, seq, task_id, action, requires, produces = sys.argv[1:]
seq = int(seq)

with open(journal_path) as f:
    j = json.load(f)

# Next attempt number for this step_id.
attempt = sum(1 for e in j["events"] if e.get("step_id") == step_id) + 1
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

event = {
    "event_id": f"E{len(j['events'])+1:04d}",
    "step_id":   step_id,
    "seq":        seq,
    "task_id":   task_id,
    "action":    action,
    "attempt":   attempt,
    "started_on":  now,
    "completed_on": now,
    "outcome":   "applied",
    "state_before": {"task_status": requires},
    "state_after":  {"task_status": produces},
    "reason": f"idempotent_adopt: action={action} actual exceeds produces={produces}; operator advance via swim-adopt-drift",
}

j["events"].append(event)
j["cursor"] = j["cursor"] + 1
j["last_event"] = event

with open(journal_path, "w") as f:
    json.dump(j, f, indent=2)

print(f"  appended: {event['event_id']}  cursor now {j['cursor']}")
PYEOF

# Verify next step.
echo ""
echo "Next decision:"
swim-step --schedule "$SCHEDULE" --journal "$JOURNAL" --state "$STATE" 2>&1 || true
