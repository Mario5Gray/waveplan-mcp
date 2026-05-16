# SWIM Schedule Migrator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a deterministic migrator that upgrades an existing in-flight SWIM schedule to schema v3 without changing the schedule row stream that the current journal cursor depends on.

**Architecture:** The migrator treats the existing schedule as ground truth and rewrites only the schema/invocation contract: `schema_version`, `operation`, `invoke.argv`, and `wp_invoke`. State is never modified. Journal migration is optional and limited to validating cursor safety or writing a copy with `schedule_path` updated when the migrated schedule is written to a new path.

**Tech Stack:** Bash helper script with embedded Python JSON transformation, existing Go SWIM schema validation/runtime, JSON Schema draft 2020-12, existing shell tests under `tests/swim`.

---

## Contract Summary

Create `wp-migrate-swim-schema.sh`.

Public usage:

```bash
wp-migrate-swim-schema.sh \
  --schedule <existing-schedule.json> \
  --out <migrated-schedule.json> \
  [--journal <existing-journal.json>] \
  [--journal-out <patched-journal.json>] \
  [--state <state.json>] \
  [--plan <plan.json>] \
  [--invoker <path-or-command>]
```

Behavior:

- Preserve each row's `seq`, `step_id`, `task_id`, `action`, `requires`, `produces`, `status`, and any unrelated metadata.
- Set schedule `schema_version` to `3`.
- Infer `operation` from each row's `action` plus old `invoke.argv` or `wp_invoke`.
- Rewrite `invoke.argv` to use `wp-plan-step.sh --action ... --plan ... --task-id ...`.
- Rewrite `wp_invoke` from the new argv using shell quoting.
- Preserve old dispatch flags: `--target`, `--agent`, `--reviewer`.
- Preserve old lifecycle flags through `operation`, not only through debug argv:
  - `end_review`: `--review-note`
  - `finish`: `--git-sha`
- Validate the migrated schedule with `cmd/swim-validate`.
- If `--journal` is provided, verify `journal.cursor <= len(execution)` and fail on pending `unknown` events.
- If `--journal-out` is provided, write a journal copy with `schedule_path` set to the migrated schedule path.
- If `--state` is provided, parse it as JSON for sanity only; do not write it.
- If the input schedule is already `schema_version: 3`, copy it unchanged to `--out`, validate it, optionally patch `journal.schedule_path`, and exit successfully. This makes migration idempotent and avoids losing existing `operation` fields.
- Reject rows where the explicit row `action` contradicts legacy `--mode`.
- Warn to stderr when dropping dispatch-only flags such as `--target`, `--agent`, or `--reviewer` from state-transition rows.

Action mapping:

```text
implement   -> operation.kind=agent_dispatch, action=implement
review      -> operation.kind=agent_dispatch, action=review
fix         -> operation.kind=agent_dispatch, action=fix
review_end  -> operation.kind=state_transition, action=end_review
end_review  -> operation.kind=state_transition, action=end_review
fin         -> operation.kind=state_transition, action=finish
finish      -> operation.kind=state_transition, action=finish
```

---

## File Structure

- Modify `docs/specs/swim-schedule-schema-v3.json`: allow optional state-transition parameters on `operation`.
- Modify `internal/swim/contracts.go`: add operation fields for retained lifecycle parameters and derive argv from them.
- Modify `internal/swim/contracts_test.go`: cover lifecycle parameter retention in validation and `BuildInvokeArgv`.
- Create `wp-migrate-swim-schema.sh`: migration entrypoint and embedded Python transformer.
- Create `tests/swim/test_wp_migrate_swim_schema.sh`: end-to-end migration contract test.
- Modify `Makefile`: install `wp-migrate-swim-schema.sh` with helper scripts.
- Modify `README.md` and `docs/specs/2026-05-05-swim-ops.md`: document safe mid-flight migration.

---

### Task 1: Extend v3 Operation For Retained Lifecycle Flags

**Files:**
- Modify: `docs/specs/swim-schedule-schema-v3.json`
- Modify: `internal/swim/contracts.go`
- Modify: `internal/swim/contracts_test.go`
- Test: `go test ./internal/swim -run 'TestBuildInvokeArgv|TestValidateSchedule' -count=1`

- [ ] **Step 1: Add failing tests for state-transition operation parameters**

Add these tests to `internal/swim/contracts_test.go`:

```go
func TestBuildInvokeArgvV3RetainsReviewNoteAndGitSHA(t *testing.T) {
	tests := []struct {
		name string
		row  ScheduleRow
		want []string
	}{
		{
			name: "end_review_review_note",
			row: ScheduleRow{
				StepID: "S1_T1.1_end_review",
				TaskID: "T1.1",
				Action: "end_review",
				Operation: OperationSpec{
					Kind:       "state_transition",
					ReviewNote: "looks good",
				},
			},
			want: []string{"wp-plan-step.sh", "--action", "end_review", "--plan", "/tmp/plan.json", "--task-id", "T1.1", "--review-note", "looks good"},
		},
		{
			name: "finish_git_sha",
			row: ScheduleRow{
				StepID: "S1_T1.1_finish",
				TaskID: "T1.1",
				Action: "finish",
				Operation: OperationSpec{
					Kind:   "state_transition",
					GitSHA: "DEFERRED",
				},
			},
			want: []string{"wp-plan-step.sh", "--action", "finish", "--plan", "/tmp/plan.json", "--task-id", "T1.1", "--git-sha", "DEFERRED"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildInvokeArgv(tt.row, "/tmp/plan.json")
			if err != nil {
				t.Fatalf("BuildInvokeArgv() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("BuildInvokeArgv() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateScheduleV3AllowsLifecycleOperationParams(t *testing.T) {
	raw := []byte(`{
	  "schema_version": 3,
	  "execution": [{
	    "seq": 1,
	    "step_id": "S1_T1.1_finish",
	    "task_id": "T1.1",
	    "action": "finish",
	    "requires": {"task_status": "review_ended"},
	    "produces": {"task_status": "completed"},
	    "operation": {"kind": "state_transition", "git_sha": "DEFERRED"},
	    "invoke": {"argv": ["wp-plan-step.sh", "--action", "finish", "--plan", "/tmp/plan.json", "--task-id", "T1.1", "--git-sha", "DEFERRED"]}
	  }]
	}`)
	if err := ValidateSchedule(raw); err != nil {
		t.Fatalf("ValidateSchedule() unexpected error: %v", err)
	}
}
```

If `cmp.Diff` is not already imported in the file, use `reflect.DeepEqual` instead of adding a new dependency.

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./internal/swim -run 'TestBuildInvokeArgvV3RetainsReviewNoteAndGitSHA|TestValidateScheduleV3AllowsLifecycleOperationParams' -count=1
```

Expected: fail because `OperationSpec` does not yet expose `review_note` or `git_sha`, and `BuildInvokeArgv` drops those flags.

- [ ] **Step 3: Add operation fields**

Modify `OperationSpec` in `internal/swim/contracts.go`:

```go
type OperationSpec struct {
	Kind       string `json:"kind"`
	Target     string `json:"target,omitempty"`
	Agent      string `json:"agent,omitempty"`
	Reviewer   string `json:"reviewer,omitempty"`
	ReviewNote string `json:"review_note,omitempty"`
	GitSHA     string `json:"git_sha,omitempty"`
}
```

Update `IsZero()` so both new fields participate:

```go
func (o OperationSpec) IsZero() bool {
	return o.Kind == "" &&
		o.Target == "" &&
		o.Agent == "" &&
		o.Reviewer == "" &&
		o.ReviewNote == "" &&
		o.GitSHA == ""
}
```

- [ ] **Step 4: Derive retained lifecycle flags from operation**

In `BuildInvokeArgv`, extend the `state_transition` branch:

```go
case "state_transition":
	if row.Action == "end_review" && row.Operation.ReviewNote != "" {
		argv = append(argv, "--review-note", row.Operation.ReviewNote)
	}
	if row.Action == "finish" && row.Operation.GitSHA != "" {
		argv = append(argv, "--git-sha", row.Operation.GitSHA)
	}
```

In the existing `validateInvokeAgainstOperation` helper, keep rejecting dispatch
flags on `state_transition`, but validate retained lifecycle flags when present.
The existing `lookupFlagValue` helper returns `(value, true)` only when a flag has
a following value.

```go
if row.Action == "end_review" && row.Operation.ReviewNote != "" {
	if value, ok := lookupFlagValue(argv, "--review-note"); !ok || value != row.Operation.ReviewNote {
		return fmt.Errorf("invoke.argv contradicts operation: --review-note mismatch for %s", row.TaskID)
	}
}
if row.Action == "finish" && row.Operation.GitSHA != "" {
	if value, ok := lookupFlagValue(argv, "--git-sha"); !ok || value != row.Operation.GitSHA {
		return fmt.Errorf("invoke.argv contradicts operation: --git-sha mismatch for %s", row.TaskID)
	}
}
```

- [ ] **Step 5: Update v3 JSON schema**

In `docs/specs/swim-schedule-schema-v3.json`, add these optional properties to the `state_transition_operation` definition:

```json
"review_note": { "type": "string" },
"git_sha": { "type": "string", "minLength": 1 }
```

Keep `additionalProperties: false`.

- [ ] **Step 6: Verify tests pass**

Run:

```bash
go test ./internal/swim -run 'TestBuildInvokeArgvV3RetainsReviewNoteAndGitSHA|TestValidateScheduleV3AllowsLifecycleOperationParams' -count=1
```

Expected: pass.

---

### Task 2: Add Migrator Contract Test

**Files:**
- Create: `tests/swim/test_wp_migrate_swim_schema.sh`
- Test: `bash tests/swim/test_wp_migrate_swim_schema.sh`

- [ ] **Step 1: Write failing shell test**

Create `tests/swim/test_wp_migrate_swim_schema.sh`:

```bash
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
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
bash tests/swim/test_wp_migrate_swim_schema.sh
```

Expected: fail because `wp-migrate-swim-schema.sh` does not exist.

---

### Task 3: Implement `wp-migrate-swim-schema.sh`

**Files:**
- Create: `wp-migrate-swim-schema.sh`
- Test: `bash tests/swim/test_wp_migrate_swim_schema.sh`

- [ ] **Step 1: Create the script header and argument parser**

Create `wp-migrate-swim-schema.sh` with:

```bash
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
```

- [ ] **Step 2: Add embedded Python transformer**

Append a Python block that:

```python
import json
import os
import shlex
import sys
from pathlib import Path
```

Implement helpers:

```python
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
```

Before transforming rows, handle already-migrated schedules idempotently:

```python
schema_version = schedule.get("schema_version")
if schema_version == 3:
    write_json(out_path, schedule)
    handle_journal_copy_if_requested(len(schedule.get("execution", [])))
    sys.exit(0)
if schema_version not in (1, 2, None):
    raise SystemExit(f"unsupported source schedule schema_version: {schema_version}")
```

Implement `handle_journal_copy_if_requested(execution_len)` with the journal
checks shown in Step 3 so already-v3 schedules get the same cursor and unknown
event safety checks.

For each non-v3 row:

```python
old_argv = argv_for(row)
action = normalize_action(row, old_argv)
task_id = str(row.get("task_id") or flag(old_argv, "--task-id"))
plan = plan_override or flag(old_argv, "--plan")
if not task_id:
    raise SystemExit(f"{row.get('step_id', '<unknown>')}: missing task_id; add row.task_id or legacy --task-id")
if not plan:
    raise SystemExit(f"{row.get('step_id', '<unknown>')}: missing --plan and no --plan override provided")
```

Build the operation:

```python
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
```

Build canonical argv:

```python
new_argv = [invoker, "--action", action, "--plan", plan, "--task-id", task_id]
if operation["kind"] == "agent_dispatch":
    new_argv.extend(["--target", operation["target"], "--agent", operation["agent"]])
    if action == "review":
        new_argv.extend(["--reviewer", operation["reviewer"]])
elif action == "end_review" and operation.get("review_note"):
    new_argv.extend(["--review-note", operation["review_note"]])
elif action == "finish" and operation.get("git_sha"):
    new_argv.extend(["--git-sha", operation["git_sha"]])
```

Copy the row and update only these fields:

```python
new_row = dict(row)
new_row["action"] = action
new_row["task_id"] = task_id
new_row["operation"] = operation
new_row["invoke"] = {"argv": new_argv}
new_row["wp_invoke"] = shell_join(new_argv)
```

Preserve `seq`, `step_id`, `requires`, `produces`, `status`, and other metadata as-is.

- [ ] **Step 3: Add journal and state safety checks**

In the embedded Python:

```python
if state_path:
    with open(state_path, encoding="utf-8") as f:
        json.load(f)

if journal_path:
    with open(journal_path, encoding="utf-8") as f:
        journal = json.load(f)
    cursor = int(journal.get("cursor", 0))
    if cursor < 0 or cursor > len(new_execution):
        raise SystemExit(f"journal cursor {cursor} exceeds migrated execution length {len(new_execution)}")
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
```

`cursor == len(new_execution)` is valid and means the schedule is already fully
consumed. The migrator should still allow it because this can be useful for
archival consistency and for replacing old schedule artifacts with v3 artifacts.

- [ ] **Step 4: Write JSON atomically enough for helper usage**

Use a temporary sibling file and rename it:

```python
def write_json(path, payload):
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    tmp = target.with_name(f".{target.name}.tmp")
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(payload, f, indent=2)
        f.write("\n")
    os.replace(tmp, target)
```

- [ ] **Step 5: Validate migrated schedule from the shell wrapper**

After the Python block returns:

```bash
(
  cd "$SCRIPT_DIR"
  go run ./cmd/swim-validate --kind schedule --in "$OUT" >/dev/null
)
```

- [ ] **Step 6: Make script executable**

Run:

```bash
chmod +x wp-migrate-swim-schema.sh
```

- [ ] **Step 7: Verify test passes**

Run:

```bash
bash tests/swim/test_wp_migrate_swim_schema.sh
```

Expected: pass.

---

### Task 4: Add Negative Migration Tests

**Files:**
- Modify: `tests/swim/test_wp_migrate_swim_schema.sh`
- Test: `bash tests/swim/test_wp_migrate_swim_schema.sh`

- [ ] **Step 1: Add a missing dispatch flag case**

Append this case to the shell test:

```bash
BAD_SCHEDULE="$TMP/bad-schedule.json"
BAD_OUT="$TMP/bad-out.json"
jq 'del(.execution[0].invoke.argv[5,6])' "$OLD_SCHEDULE" >"$BAD_SCHEDULE"

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
```

- [ ] **Step 2: Add contradictory action/mode case**

Append:

```bash
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
```

- [ ] **Step 3: Add already-v3 idempotency case**

Append:

```bash
V3_AGAIN="$TMP/v3-again.json"
"$ROOT/wp-migrate-swim-schema.sh" --schedule "$NEW_SCHEDULE" --out "$V3_AGAIN"

python3 - "$NEW_SCHEDULE" "$V3_AGAIN" <<'PY'
import json, sys
left = json.load(open(sys.argv[1], encoding="utf-8"))
right = json.load(open(sys.argv[2], encoding="utf-8"))
if left != right:
    raise SystemExit("already-v3 migration was not idempotent")
PY
```

- [ ] **Step 4: Add state-transition dispatch flag warning case**

Append:

```bash
LEAK_SCHEDULE="$TMP/leak-schedule.json"
jq '.execution[2].invoke.argv += ["--target", "codex", "--agent", "phi"]' "$OLD_SCHEDULE" >"$LEAK_SCHEDULE"
"$ROOT/wp-migrate-swim-schema.sh" --schedule "$LEAK_SCHEDULE" --out "$TMP/leak-out.json" 2>"$TMP/leak.err"

grep -q "dropping dispatch flag" "$TMP/leak.err" || {
  echo "FAIL: expected warning for stripped dispatch flags on state_transition row"
  exit 1
}
jq -e '.execution[2].operation.kind == "state_transition" and (.execution[2].operation.target? | not) and (.execution[2].operation.agent? | not)' "$TMP/leak-out.json" >/dev/null
```

- [ ] **Step 5: Add a pending unknown journal case**

Append:

```bash
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
```

- [ ] **Step 6: Run the test and verify the new negative cases fail before the checks exist**

Run:

```bash
bash tests/swim/test_wp_migrate_swim_schema.sh
```

Expected: fail until the script rejects missing dispatch flags, rejects
contradictory action/mode rows, handles already-v3 schedules idempotently, warns
when stripping dispatch flags from state-transition rows, and rejects pending
unknown journal events.

- [ ] **Step 7: Implement minimal missing checks**

Patch `wp-migrate-swim-schema.sh` so:

```text
- dispatch rows require target and agent
- review rows require reviewer
- action and legacy --mode must not contradict
- already-v3 schedules copy unchanged to --out
- state_transition rows warn when dispatch-only flags are stripped
- pending unknown events fail before writing migrated output
```

- [ ] **Step 8: Verify all migrator shell tests pass**

Run:

```bash
bash tests/swim/test_wp_migrate_swim_schema.sh
```

Expected: pass.

---

### Task 5: Install And Document The Migrator

**Files:**
- Modify: `Makefile`
- Modify: `README.md`
- Modify: `docs/specs/2026-05-05-swim-ops.md`
- Test: `drift check`

- [ ] **Step 1: Install helper script with other helper scripts**

Modify `Makefile`:

```make
HELPER_SCRIPTS := waveplan-cli wp-task-to-agent.sh wp-plan-to-agent.sh wp-emit-wave-execution.sh wp-plan-step.sh wp-agent-dispatch.sh wp-migrate-swim-schema.sh
```

If `wp-plan-step.sh` and `wp-agent-dispatch.sh` are already added to this list in the branch, only append `wp-migrate-swim-schema.sh`.

- [ ] **Step 2: Add README usage**

Add a short section after the helper script docs:

````markdown
### Migrating An In-Flight SWIM Schedule

Use `wp-migrate-swim-schema.sh` when a schedule was already running before the
v3 `operation` contract landed. The migrator preserves row identity and only
rewrites the schedule invocation contract.

```bash
wp-migrate-swim-schema.sh \
  --schedule "$WAVEPLAN_SCHED" \
  --out "$WAVEPLAN_SCHED.v3.json" \
  --journal "$WAVEPLAN_JOURNAL" \
  --journal-out "$WAVEPLAN_JOURNAL.v3.json" \
  --state "$WAVEPLAN_STATE"
```

State is read-only. Journal output is optional; use it only when the migrated
schedule is written to a different path and `schedule_path` must be updated.
````

- [ ] **Step 3: Add ops-manual recovery note**

Add to `docs/specs/2026-05-05-swim-ops.md` near the cursor drift/recovery section:

```markdown
### Migrating schedule schema mid-flight

When moving a running schedule to schema v3, prefer `wp-migrate-swim-schema.sh`
over regenerating from the plan. The migrator preserves the row stream that the
journal cursor indexes and updates only `operation`, `invoke.argv`, and
`wp_invoke`.

State files are not migrated. Journal files remain schema v1; only
`schedule_path` may need to change if the migrated schedule is written to a new
path.
```

- [ ] **Step 4: Verify docs and drift**

Run:

```bash
drift check
```

Expected: `ok`.

---

### Task 6: Final Verification

**Files:**
- Test-only task.

- [ ] **Step 1: Run focused shell tests**

Run:

```bash
bash tests/swim/test_wp_migrate_swim_schema.sh
bash tests/swim/test_t1_2_invoke_argv.sh
bash tests/swim/test_wp_plan_step_contract.sh
bash tests/swim/test_wp_task_to_agent_rejects_task_id.sh
```

Expected: all pass.

- [ ] **Step 2: Run focused Go tests**

Run:

```bash
go test ./internal/swim ./cmd/swim-validate ./cmd/swim-run ./cmd/swim-step
```

Expected: pass.

- [ ] **Step 3: Run whole-repo tests only if the known raft-cli-split baseline is resolved**

Run:

```bash
go test ./...
```

Expected in the current branch context: may still fail in the unrelated `main_test.go` raft-cli-split area. Do not treat those failures as migrator regressions unless new failures appear under `internal/swim`, `cmd/swim-*`, or the new shell tests.

- [ ] **Step 4: Summarize migration safety in final handoff**

Include these points:

```text
- Existing state is read-only and reusable.
- Existing journal is reusable when row identity is preserved.
- If migrated schedule path differs, use --journal-out and run with that patched journal.
- Pending unknown journal events must be resolved before migration.
- Regeneration remains acceptable only when it is proven to emit the same row stream.
```
