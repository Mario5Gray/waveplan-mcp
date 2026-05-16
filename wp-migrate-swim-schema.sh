#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
INVOKER="${WP_PLAN_STEP_BIN:-wp-plan-step.sh}"

usage() {
  cat <<'USAGE'
Usage:
  wp-migrate-swim-schema.sh --schedule <existing-schedule.json> --out <migrated-schedule.json>
                            [--journal <existing-journal.json>] [--journal-out <patched-journal.json>]
                            [--state <state.json>] [--plan <plan.json>] [--invoker <path-or-command>]

Description:
  Upgrade an existing SWIM schedule to schema v3 without changing row identity.
  State is read-only. Journal migration is optional and only patches schedule_path
  when --journal-out is provided.
USAGE
}

SCHEDULE=""
OUT=""
JOURNAL=""
JOURNAL_OUT=""
STATE=""
PLAN_OVERRIDE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --schedule) SCHEDULE="${2:-}"; shift 2 ;;
    --out) OUT="${2:-}"; shift 2 ;;
    --journal) JOURNAL="${2:-}"; shift 2 ;;
    --journal-out) JOURNAL_OUT="${2:-}"; shift 2 ;;
    --state) STATE="${2:-}"; shift 2 ;;
    --plan) PLAN_OVERRIDE="${2:-}"; shift 2 ;;
    --invoker) INVOKER="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ -z "$SCHEDULE" || -z "$OUT" ]]; then
  echo "Missing required args: --schedule and --out" >&2
  usage
  exit 2
fi

for path in "$SCHEDULE" ${JOURNAL:+"$JOURNAL"} ${STATE:+"$STATE"}; do
  if [[ ! -f "$path" ]]; then
    echo "Input file not found: $path" >&2
    exit 2
  fi
done

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found" >&2
  exit 2
fi

# Phase 1: Python transforms schedule and writes to a temp file.
# It also writes journal-out if requested, but does NOT write the schedule output file.
TMP_MIGRATED=$(mktemp)
trap 'rm -f "$TMP_MIGRATED"' EXIT

set +e
python3 - "$SCHEDULE" "$OUT" "$JOURNAL" "$JOURNAL_OUT" "$STATE" "$PLAN_OVERRIDE" "$INVOKER" "$TMP_MIGRATED" <<'PYEOF'
import json
import os
import shlex
import sys
from pathlib import Path

schedule_path = sys.argv[1]
out_path = sys.argv[2]
journal_path = sys.argv[3] if sys.argv[3] else None
journal_out = sys.argv[4] if sys.argv[4] else None
state_path = sys.argv[5] if sys.argv[5] else None
plan_override = sys.argv[6] if sys.argv[6] else None
invoker = sys.argv[7] if sys.argv[7] else "wp-plan-step.sh"
tmp_path = sys.argv[8]

with open(schedule_path, encoding="utf-8") as f:
    schedule = json.load(f)

def write_json(path, payload):
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    tmp = target.with_name(f".{target.name}.tmp")
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(payload, f, indent=2)
        f.write("\n")
    os.replace(tmp, target)

def handle_journal_copy_if_requested(execution_len):
    if not journal_path:
        return
    with open(journal_path, encoding="utf-8") as f:
        journal = json.load(f)
    cursor = int(journal.get("cursor", 0))
    if cursor < 0 or cursor > execution_len:
        raise SystemExit(f"journal cursor {cursor} exceeds migrated execution length {execution_len}")
    unknown = [
        e for e in journal.get("events", [])
        if isinstance(e, dict) and e.get("outcome") == "unknown" and cursor < int(e.get("seq", 0))
    ]
    if unknown:
        first = unknown[0]
        raise SystemExit(f"journal has pending unknown event: {first.get('event_id')} {first.get('step_id')}")
    if journal_out:
        journal_copy = dict(journal)
        journal_copy["schedule_path"] = out_path
        write_json(journal_out, journal_copy)

schema_version = schedule.get("schema_version")
if schema_version == 3:
    # Write to temp file for validation, then copy to out_path after validation.
    with open(tmp_path, "w", encoding="utf-8") as f:
        json.dump(schedule, f, indent=2)
        f.write("\n")
    handle_journal_copy_if_requested(len(schedule.get("execution", [])))
    sys.exit(0)
if schema_version not in (1, 2, None):
    raise SystemExit(f"unsupported source schedule schema_version: {schema_version}")

def argv_for(row):
    invoke = row.get("invoke") or {}
    argv = invoke.get("argv")
    if isinstance(argv, list) and all(isinstance(x, str) for x in argv):
        return argv
    wp = row.get("wp_invoke")
    if isinstance(wp, str) and wp.strip():
        return shlex.split(wp)
    return []

def flag(argv, name):
    for i, token in enumerate(argv):
        if token == name and i + 1 < len(argv):
            return argv[i + 1]
    return ""

def canonical_action(value):
    aliases = {"review_end": "end_review", "fin": "finish"}
    value = str(value or "")
    return aliases.get(value, value)

def normalize_action(row, argv):
    step_id = row.get("step_id", "<unknown>")
    action = canonical_action(row.get("action"))
    mode = canonical_action(flag(argv, "--mode"))
    if action and mode and action != mode:
        raise SystemExit(f"{step_id}: action {action!r} contradicts legacy --mode {mode!r}")
    if action:
        return action
    if mode:
        return mode
    raise SystemExit(f"{step_id}: missing action and legacy --mode")

def shell_join(argv):
    return shlex.join(argv)

new_execution = []
for row in schedule.get("execution", []):
    old_argv = argv_for(row)
    action = normalize_action(row, old_argv)
    task_id = str(row.get("task_id") or flag(old_argv, "--task-id"))
    plan = plan_override or flag(old_argv, "--plan")
    if not task_id:
        raise SystemExit(f"{row.get('step_id', '<unknown>')}: missing task_id; add row.task_id or legacy --task-id")
    if not plan:
        raise SystemExit(f"{row.get('step_id', '<unknown>')}: missing --plan and no --plan override provided")

    if action in {"implement", "review", "fix"}:
        operation = {
            "kind": "agent_dispatch",
            "target": flag(old_argv, "--target"),
            "agent": flag(old_argv, "--agent"),
        }
        if action == "review":
            operation["reviewer"] = flag(old_argv, "--reviewer")
        missing = [k for k, v in operation.items() if k != "kind" and not v]
        if missing:
            raise SystemExit(f"{row.get('step_id', '<unknown>')}: missing dispatch flag(s): {', '.join(missing)}")
    else:
        operation = {"kind": "state_transition"}
        review_note = flag(old_argv, "--review-note")
        git_sha = flag(old_argv, "--git-sha")
        stripped = [name for name in ("--target", "--agent", "--reviewer") if flag(old_argv, name)]
        if stripped:
            print(
                f"{row.get('step_id', '<unknown>')}: warning: dropping dispatch flag(s) from state_transition row: {', '.join(stripped)}",
                file=sys.stderr,
            )
        if action == "end_review" and review_note:
            operation["review_note"] = review_note
        if action == "finish" and git_sha:
            operation["git_sha"] = git_sha

    new_argv = [invoker, "--action", action, "--plan", plan, "--task-id", task_id]
    if operation["kind"] == "agent_dispatch":
        new_argv.extend(["--target", operation["target"], "--agent", operation["agent"]])
        if action == "review":
            new_argv.extend(["--reviewer", operation["reviewer"]])
    elif action == "end_review" and operation.get("review_note"):
        new_argv.extend(["--review-note", operation["review_note"]])
    elif action == "finish" and operation.get("git_sha"):
        new_argv.extend(["--git-sha", operation["git_sha"]])

    new_row = dict(row)
    new_row["action"] = action
    new_row["task_id"] = task_id
    new_row["operation"] = operation
    new_row["invoke"] = {"argv": new_argv}
    new_row["wp_invoke"] = shell_join(new_argv)
    new_execution.append(new_row)

new_schedule = dict(schedule)
new_schedule["schema_version"] = 3
new_schedule["execution"] = new_execution

# Write to temp file for shell-side validation.
with open(tmp_path, "w", encoding="utf-8") as f:
    json.dump(new_schedule, f, indent=2)
    f.write("\n")

# State is read-only; validate it exists.
if state_path:
    with open(state_path, encoding="utf-8") as f:
        json.load(f)

# Journal checks are deferred until after validation succeeds.
if journal_path:
    print(f"JOURNAL_CHECK:{journal_path}:{journal_out or ''}", file=sys.stderr)
sys.exit(0)
PYEOF
_PY_EXIT=$?
set -e

# Check Python exit code before proceeding.
if [[ $_PY_EXIT -ne 0 ]]; then
  exit 1
fi

# Phase 2: Validate the migrated JSON before writing to disk.
if ! (cd "$SCRIPT_DIR" && go run ./cmd/swim-validate --kind schedule --in "$TMP_MIGRATED" >/dev/null 2>&1); then
  # Fallback for installed script: use swim-validate from PATH.
  if command -v swim-validate >/dev/null 2>&1; then
    if ! swim-validate --kind schedule --in "$TMP_MIGRATED" >/dev/null 2>&1; then
      echo "ERROR: migrated schedule failed v3 validation" >&2
      exit 1
    fi
  else
    echo "WARNING: swim-validate not found; skipping migrated schedule validation" >&2
  fi
fi

# Phase 3: Validation passed — write the output file.
cp "$TMP_MIGRATED" "$OUT"

# Phase 4: Journal operations (only after successful validation + write).
if [[ -n "$JOURNAL" ]]; then
  python3 - "$JOURNAL" "$JOURNAL_OUT" "$OUT" <<'PYEOF2'
import json
import os
import sys
from pathlib import Path

journal_path = sys.argv[1]
journal_out = sys.argv[2] if sys.argv[2] else None
out_path = sys.argv[3]

with open(journal_path, encoding="utf-8") as f:
    journal = json.load(f)
cursor = int(journal.get("cursor", 0))
execution_len = len(json.load(open(out_path, encoding="utf-8"))["execution"])
if cursor < 0 or cursor > execution_len:
    raise SystemExit(f"journal cursor {cursor} exceeds migrated execution length {execution_len}")
unknown = [
    e for e in journal.get("events", [])
    if isinstance(e, dict) and e.get("outcome") == "unknown" and cursor < int(e.get("seq", 0))
]
if unknown:
    first = unknown[0]
    raise SystemExit(f"journal has pending unknown event: {first.get('event_id')} {first.get('step_id')}")
if journal_out:
    target = Path(journal_out)
    target.parent.mkdir(parents=True, exist_ok=True)
    tmp = target.with_name(f".{target.name}.tmp")
    with open(tmp, "w", encoding="utf-8") as f:
        journal["schedule_path"] = out_path
        json.dump(journal, f, indent=2)
        f.write("\n")
    os.replace(tmp, target)
PYEOF2
fi