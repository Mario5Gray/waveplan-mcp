# SWIM Plan Step Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the ambiguous script/argv schedule contract with a typed SWIM plan-step contract where scheduled rows execute through one plan-step facade and agent dispatch is an internal prompt-delivery helper.

**Architecture:** SWIM schedules become schema v3 documents with a structured `operation` object as the source of truth. `wp-plan-step.sh` owns scheduled lifecycle actions for one `task_id`; `wp-agent-dispatch.sh` only sends prepared prompts to `codex`, `claude`, or `opencode`. Legacy script names remain as compatibility wrappers for one release.

**Tech Stack:** Go SWIM runtime and validation, JSON Schema draft 2020-12, Bash support scripts, jq/python helper snippets already used by existing scripts, existing shell and Go test suites.

---

## Offline Execution Boundary

Do not execute this plan through `waveplan-cli swim run`. Implement it in a normal branch/worktree with local tests. Use these docs as contract references:

- `docs/specs/swim-schedule-schema-v2.json`
- `docs/specs/swim-review-schedule-schema-v1.json`
- `docs/specs/2026-05-05-swim-ops.md`
- `wp-plan-to-agent.sh`
- `wp-task-to-agent.sh`
- `wp-emit-wave-execution.sh`
- `internal/swim/contracts.go`
- `internal/swim/runner.go`
- `internal/swim/safe_runner.go`

## File Structure

- Create `docs/specs/swim-schedule-schema-v3.json`: new canonical schedule schema with `operation`.
- Create `docs/specs/swim-review-schedule-schema-v2.json`: review/fix sidecar schema aligned with schedule v3 rows.
- Modify `docs/specs/2026-05-05-swim-ops.md`: document v3 operations and script responsibilities.
- Modify `internal/swim/contracts.go`: add operation structs, schema constants, strict validation.
- Modify `internal/swim/runner.go` and `internal/swim/run.go`: execute derived argv from operation for v3 rows.
- Modify `internal/swim/safe_runner.go`: pass receipt/env behavior through the v3 derived invoke path.
- Modify `wp-emit-wave-execution.sh`: emit v3 rows with `operation`; keep `invoke.argv` and `wp_invoke` derived.
- Create `wp-plan-step.sh`: canonical public schedule executor.
- Create `wp-agent-dispatch.sh`: internal agent prompt delivery helper.
- Modify `wp-plan-to-agent.sh`: compatibility wrapper to `wp-plan-step.sh`.
- Modify `wp-task-to-agent.sh`: compatibility wrapper to `wp-agent-dispatch.sh` with guarded legacy behavior.
- Modify tests under `internal/swim/*_test.go` and `tests/swim/*.sh`.

---

### Task 1: Document The v3 Contract

**Files:**
- Create: `docs/specs/swim-schedule-schema-v3.json`
- Create: `docs/specs/swim-review-schedule-schema-v2.json`
- Modify: `docs/specs/2026-05-05-swim-ops.md`
- Test: `internal/swim/contracts_test.go`

- [ ] **Step 1: Add a failing schema-validation test for v3 operation rows**

Add a test in `internal/swim/contracts_test.go` that builds a minimal v3 schedule and validates it:

```go
func TestValidateScheduleV3OperationContract(t *testing.T) {
	raw := []byte(`{
	  "schema_version": 3,
	  "execution": [{
	    "seq": 1,
	    "step_id": "S1_T1.1_implement",
	    "task_id": "T1.1",
	    "action": "implement",
	    "requires": {"task_status": "available"},
	    "produces": {"task_status": "taken"},
	    "operation": {
	      "kind": "agent_dispatch",
	      "target": "opencode",
	      "agent": "phi"
	    },
	    "invoke": {"argv": ["wp-plan-step.sh", "--action", "implement", "--task-id", "T1.1", "--target", "opencode", "--agent", "phi"]}
	  }]
	}`)
	if err := ValidateSchedule(raw); err != nil {
		t.Fatalf("ValidateSchedule(v3) unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
go test ./internal/swim -run TestValidateScheduleV3OperationContract -count=1
```

Expected: fail because `schema_version: 3` and `operation` are not yet accepted.

- [ ] **Step 3: Create `swim-schedule-schema-v3.json`**

Create `docs/specs/swim-schedule-schema-v3.json` from v2 with these changes:

```json
{
  "schema_version": { "type": "integer", "const": 3 },
  "$defs": {
    "operation": {
      "oneOf": [
        { "$ref": "#/$defs/agent_dispatch_operation" },
        { "$ref": "#/$defs/state_transition_operation" }
      ]
    },
    "agent_dispatch_operation": {
      "type": "object",
      "required": ["kind", "target", "agent"],
      "properties": {
        "kind": { "const": "agent_dispatch" },
        "target": { "enum": ["codex", "claude", "opencode"] },
        "agent": { "type": "string", "minLength": 1 },
        "reviewer": { "type": "string", "minLength": 1 }
      },
      "additionalProperties": false
    },
    "state_transition_operation": {
      "type": "object",
      "required": ["kind"],
      "properties": {
        "kind": { "const": "state_transition" }
      },
      "additionalProperties": false
    }
  }
}
```

In the full schema, add `operation` to `execution_row.required`.

- [ ] **Step 4: Create `swim-review-schedule-schema-v2.json`**

Create a review sidecar schema mirroring v1 but requiring `operation` on each insertion. Keep `invoke` for derived/debug compatibility. Set:

```json
{
  "schema_version": { "type": "integer", "const": 2 }
}
```

- [ ] **Step 5: Update SWIM ops documentation**

Add a section to `docs/specs/2026-05-05-swim-ops.md`:

```markdown
### Schedule v3 operation contract

`operation` is the source of truth for execution. `invoke.argv` and `wp_invoke`
are derived compatibility/debug fields.

`agent_dispatch` actions: `implement`, `review`, `fix`.
`state_transition` actions: `end_review`, `finish`.

All scheduled rows execute through `wp-plan-step.sh`. Agent prompt delivery is
internal and goes through `wp-agent-dispatch.sh`.
```

- [ ] **Step 6: Run documentation/schema tests**

Run:

```bash
go test ./internal/swim -run 'TestValidateScheduleV3OperationContract|TestEmitterGoldenSchedule' -count=1
```

Expected after Task 1: v3 validation test passes after Go support in Task 2; emitter golden may still fail until Task 3.

---

### Task 2: Add Go Operation Types And Validation

**Files:**
- Modify: `internal/swim/contracts.go`
- Modify: `internal/swim/contracts_test.go`
- Test: `go test ./internal/swim -run TestValidateSchedule -count=1`

- [ ] **Step 1: Add failing validation tests for invalid operation/action pairs**

Add tests:

```go
func TestValidateScheduleRejectsInvalidV3OperationPair(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "implement cannot be state transition",
			raw: `{"schema_version":3,"execution":[{"seq":1,"step_id":"S1_T1.1_implement","task_id":"T1.1","action":"implement","requires":{"task_status":"available"},"produces":{"task_status":"taken"},"operation":{"kind":"state_transition"},"invoke":{"argv":["wp-plan-step.sh"]}}]}`,
			want: "implement requires agent_dispatch",
		},
		{
			name: "finish cannot be agent dispatch",
			raw: `{"schema_version":3,"execution":[{"seq":1,"step_id":"S1_T1.1_finish","task_id":"T1.1","action":"finish","requires":{"task_status":"review_ended"},"produces":{"task_status":"completed"},"operation":{"kind":"agent_dispatch","target":"opencode","agent":"phi"},"invoke":{"argv":["wp-plan-step.sh"]}}]}`,
			want: "finish requires state_transition",
		},
		{
			name: "review requires reviewer",
			raw: `{"schema_version":3,"execution":[{"seq":1,"step_id":"S1_T1.1_review","task_id":"T1.1","action":"review","requires":{"task_status":"taken"},"produces":{"task_status":"review_taken"},"operation":{"kind":"agent_dispatch","target":"claude","agent":"phi"},"invoke":{"argv":["wp-plan-step.sh"]}}]}`,
			want: "review requires reviewer",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSchedule([]byte(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateSchedule() error=%v, want contains %q", err, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```bash
go test ./internal/swim -run 'TestValidateScheduleV3OperationContract|TestValidateScheduleRejectsInvalidV3OperationPair' -count=1
```

Expected: fail until operation structs and v3 schema loading exist.

- [ ] **Step 3: Add operation structs**

In `internal/swim/contracts.go`, add:

```go
const scheduleSchemaV3RelPath = "docs/specs/swim-schedule-schema-v3.json"

type OperationSpec struct {
	Kind     string `json:"kind"`
	Target   string `json:"target,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
}
```

Update `ScheduleRow` and `ReviewScheduleInsertion`:

```go
Operation OperationSpec `json:"operation,omitempty"`
```

- [ ] **Step 4: Load schema by version**

Change `ValidateSchedule` to decode `schema_version` first:

```go
var header struct {
	SchemaVersion int `json:"schema_version"`
}
if err := json.Unmarshal(data, &header); err != nil {
	return fmt.Errorf("schedule decode failed: %w", err)
}
switch header.SchemaVersion {
case 2:
	// existing v2 schema
case 3:
	// new v3 schema
default:
	return fmt.Errorf("unsupported schedule schema_version: %d", header.SchemaVersion)
}
```

- [ ] **Step 5: Add strict operation invariants**

Add helper:

```go
func validateOperationForAction(row ScheduleRow) error {
	switch row.Action {
	case "implement", "fix":
		if row.Operation.Kind != "agent_dispatch" {
			return fmt.Errorf("%s requires agent_dispatch operation", row.Action)
		}
		if row.Operation.Agent == "" || row.Operation.Target == "" {
			return fmt.Errorf("%s requires agent and target", row.Action)
		}
	case "review":
		if row.Operation.Kind != "agent_dispatch" {
			return fmt.Errorf("review requires agent_dispatch operation")
		}
		if row.Operation.Agent == "" || row.Operation.Target == "" || row.Operation.Reviewer == "" {
			return fmt.Errorf("review requires agent, target, and reviewer")
		}
	case "end_review", "finish":
		if row.Operation.Kind != "state_transition" {
			return fmt.Errorf("%s requires state_transition operation", row.Action)
		}
	}
	return nil
}
```

Call it only for `schema_version == 3`.

- [ ] **Step 6: Verify validation tests**

Run:

```bash
go test ./internal/swim -run 'TestValidateScheduleV3OperationContract|TestValidateScheduleRejectsInvalidV3OperationPair' -count=1
```

Expected: pass.

---

### Task 3: Derive Execution Argv From Operation

**Files:**
- Modify: `internal/swim/contracts.go`
- Modify: `internal/swim/runner.go`
- Modify: `internal/swim/run.go`
- Modify: `internal/swim/safe_runner.go`
- Test: `internal/swim/runner_test.go`, `internal/swim/run_test.go`, `internal/swim/safe_runner_test.go`

- [ ] **Step 1: Add failing tests for v3 derived argv**

Add a unit test:

```go
func TestBuildInvokeArgvFromV3Operation(t *testing.T) {
	row := ScheduleRow{
		TaskID: "T1.1",
		Action: "review",
		Operation: OperationSpec{
			Kind: "agent_dispatch", Target: "claude", Agent: "phi", Reviewer: "theta",
		},
	}
	got, err := BuildInvokeArgv(row, "/tmp/plan.json")
	if err != nil {
		t.Fatalf("BuildInvokeArgv() error: %v", err)
	}
	want := []string{"wp-plan-step.sh", "--action", "review", "--plan", "/tmp/plan.json", "--task-id", "T1.1", "--target", "claude", "--agent", "phi", "--reviewer", "theta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}
```

- [ ] **Step 2: Implement `BuildInvokeArgv`**

Add:

```go
func BuildInvokeArgv(row ScheduleRow, planPath string) ([]string, error) {
	if row.Operation.Kind == "" {
		return row.Invoke.Argv, nil
	}
	argv := []string{"wp-plan-step.sh", "--action", row.Action, "--plan", planPath, "--task-id", row.TaskID}
	switch row.Operation.Kind {
	case "agent_dispatch":
		argv = append(argv, "--target", row.Operation.Target, "--agent", row.Operation.Agent)
		if row.Action == "review" {
			argv = append(argv, "--reviewer", row.Operation.Reviewer)
		}
	case "state_transition":
		// no target/agent flags
	default:
		return nil, fmt.Errorf("unsupported operation kind: %s", row.Operation.Kind)
	}
	return argv, nil
}
```

- [ ] **Step 3: Route runner execution through `BuildInvokeArgv`**

In `ExecuteNextStep`, `Apply`, and dry-run reporting, replace direct reads of:

```go
row.Invoke.Argv
```

with:

```go
argv, err := BuildInvokeArgv(row, opts.SchedulePath)
if err != nil {
	return nil, err
}
```

Use `argv` for `invokeArgv` and dry-run `Reason`.

- [ ] **Step 4: Verify existing v2 behavior still works**

Run:

```bash
go test ./internal/swim -run 'TestExecuteNextStep|TestRun|TestApply' -count=1
```

Expected: existing v2 tests pass because rows without `operation` still use `invoke.argv`.

---

### Task 4: Emit v3 Schedules

**Files:**
- Modify: `wp-emit-wave-execution.sh`
- Modify: `tests/swim/test_t1_2_invoke_argv.sh`
- Modify: `tests/swim/fixtures/expected-schedule.json`
- Modify: `internal/swim/contracts_test.go`
- Test: `tests/swim/test_t1_2_invoke_argv.sh`, `go test ./internal/swim -run TestEmitterGoldenSchedule -count=1`

- [ ] **Step 1: Add failing shell assertions for v3 rows**

In `tests/swim/test_t1_2_invoke_argv.sh`, add:

```bash
jq -e '.schema_version == 3' "$OUT" >/dev/null \
  || { echo "FAIL: schedule schema_version is not 3"; exit 1; }

jq -e '.execution | all(has("operation"))' "$OUT" >/dev/null \
  || { echo "FAIL: operation missing on some row"; exit 1; }

jq -e '.execution | all(if (.action == "implement" or .action == "review" or .action == "fix") then .operation.kind == "agent_dispatch" else .operation.kind == "state_transition" end)' "$OUT" >/dev/null \
  || { echo "FAIL: operation.kind does not match action"; exit 1; }

jq -e '.execution | all(.invoke.argv[0] | endswith("wp-plan-step.sh"))' "$OUT" >/dev/null \
  || { echo "FAIL: argv[0] is not wp-plan-step.sh"; exit 1; }
```

- [ ] **Step 2: Run the shell test and verify it fails**

Run:

```bash
bash tests/swim/test_t1_2_invoke_argv.sh
```

Expected: fail because emitter still outputs v2 rows using `wp-plan-to-agent.sh`.

- [ ] **Step 3: Update emitter output**

In `wp-emit-wave-execution.sh`, change emitted root:

```python
print(json.dumps({"schema_version": 3, "execution": execution}, indent=2))
```

For each row, include `operation`:

```python
if action in {"implement", "fix"}:
    operation = {"kind": "agent_dispatch", "target": pop_provider, "agent": pop_agent}
elif action == "review":
    operation = {"kind": "agent_dispatch", "target": review_provider, "agent": pop_agent, "reviewer": review_agent}
else:
    operation = {"kind": "state_transition"}
```

Set `argv[0]` to `wp-plan-step.sh` and include `--task-id` on every row.

- [ ] **Step 4: Regenerate the expected fixture**

Run:

```bash
WAVEPLAN_CLI_BIN="$PWD/waveplan-cli" bash wp-emit-wave-execution.sh \
  --plan docs/plans/2026-05-05-swim-execution-waves.json \
  --agents tests/swim/fixtures/waveagents.json \
  --task-scope all > tests/swim/fixtures/expected-schedule.json
```

- [ ] **Step 5: Verify emitter tests**

Run:

```bash
bash tests/swim/test_t1_2_invoke_argv.sh
go test ./internal/swim -run TestEmitterGoldenSchedule -count=1
```

Expected: pass.

---

### Task 5: Add Canonical Script Split

**Files:**
- Create: `wp-plan-step.sh`
- Create: `wp-agent-dispatch.sh`
- Modify: `wp-plan-to-agent.sh`
- Modify: `wp-task-to-agent.sh`
- Test: `tests/swim/test_wp_plan_step_contract.sh`

- [ ] **Step 1: Add failing script contract test**

Create `tests/swim/test_wp_plan_step_contract.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

./wp-plan-step.sh --help | grep -q -- '--task-id'
./wp-agent-dispatch.sh --help | grep -q -- '--task-json-file'

if ./wp-agent-dispatch.sh --target opencode --plan /tmp/p --agent phi --mode implement --task-id T1.1 2>/tmp/wp-agent-dispatch.err; then
  echo "FAIL: wp-agent-dispatch accepted --task-id"
  exit 1
fi
grep -q 'Unknown argument: --task-id' /tmp/wp-agent-dispatch.err \
  || { echo "FAIL: missing unknown --task-id error"; exit 1; }

echo "PASS: plan-step/agent-dispatch contract"
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
bash tests/swim/test_wp_plan_step_contract.sh
```

Expected: fail because scripts do not exist.

- [ ] **Step 3: Create `wp-agent-dispatch.sh`**

Move prompt construction and target delivery from `wp-task-to-agent.sh` into `wp-agent-dispatch.sh`. Its public usage:

```text
wp-agent-dispatch.sh --target <codex|claude|opencode>
                     --plan <plan.json>
                     --agent <agent>
                     --mode <implement|review|fix>
                     --task-json-file <path>
                     [--reviewer <name>]
                     [--review-result-json-file <path>]
                     [--task-after-json-file <path>]
                     [--prior-review-stdout <path>]
                     [--dry-run]
```

It must not call `waveplan_cli pop`, `start_review`, `start_fix`, `end_review`, or `fin`.

- [ ] **Step 4: Create `wp-plan-step.sh`**

Public usage:

```text
wp-plan-step.sh --action <implement|review|fix|end_review|finish>
                --plan <plan.json>
                --task-id <Tn.m>
                [--target <codex|claude|opencode>]
                [--agent <name>]
                [--reviewer <name>]
                [--review-note <text>]
                [--git-sha <sha|DEFERRED>]
                [--dry-run]
```

Responsibilities:

```text
implement:
  run waveplan pop <agent>
  assert returned task_id equals --task-id
  write selected task JSON to a temp file
  call wp-agent-dispatch.sh

review:
  assert waveplan get task-<task_id> is taken
  run waveplan start_review <task_id> <reviewer>
  write selected/result JSON files
  call wp-agent-dispatch.sh

fix:
  assert task is review_taken or fixable
  run waveplan start_fix <task_id>
  write selected/result JSON files
  call wp-agent-dispatch.sh

end_review:
  run waveplan end_review <task_id> [review_note]

finish:
  run waveplan fin <task_id> [git_sha]
```

The `pop + assert` behavior is a compatibility bridge until Waveplan has an exact `claim <task_id> <agent>` command.

- [ ] **Step 5: Convert legacy scripts to wrappers**

Replace `wp-plan-to-agent.sh` body with a compatibility parser that maps:

```text
--mode -> --action
fin -> finish
review_end -> end_review
```

and then execs `wp-plan-step.sh`.

Replace `wp-task-to-agent.sh` with a guarded wrapper:

```text
- If --task-id is present: fail with a message pointing to wp-plan-step.sh.
- If legacy mode needs task selection, delegate to wp-plan-step.sh after deriving task_id from current state.
- Otherwise call wp-agent-dispatch.sh only when task JSON input is available.
```

- [ ] **Step 6: Verify script contract**

Run:

```bash
bash tests/swim/test_wp_plan_step_contract.sh
bash tests/swim/test_wp_task_to_agent_fix_mode.sh
bash tests/swim/test_wp_task_to_agent_codex_exec.sh
```

Expected: new contract test passes; legacy tests either pass unchanged or are updated to assert wrapper behavior.

---

### Task 6: Add Schedule Preflight Rejections

**Files:**
- Modify: `internal/swim/contracts.go`
- Modify: `internal/swim/contracts_test.go`
- Test: `go test ./internal/swim -run TestValidateSchedule -count=1`

- [ ] **Step 1: Add failing tests for contradictory `invoke.argv`**

Add:

```go
func TestValidateScheduleV3RejectsContradictoryInvoke(t *testing.T) {
	raw := []byte(`{
	  "schema_version": 3,
	  "execution": [{
	    "seq": 1,
	    "step_id": "S1_T1.1_finish",
	    "task_id": "T1.1",
	    "action": "finish",
	    "requires": {"task_status": "review_ended"},
	    "produces": {"task_status": "completed"},
	    "operation": {"kind": "state_transition"},
	    "invoke": {"argv": ["wp-agent-dispatch.sh", "--task-id", "T1.1"]}
	  }]
	}`)
	err := ValidateSchedule(raw)
	if err == nil || !strings.Contains(err.Error(), "invoke.argv contradicts operation") {
		t.Fatalf("ValidateSchedule() error=%v, want contradiction", err)
	}
}
```

- [ ] **Step 2: Implement contradiction checks**

For v3 rows, reject:

```text
argv[0] ending in wp-agent-dispatch.sh
argv containing --task-id while argv[0] ends in wp-agent-dispatch.sh
state_transition rows containing --target, --agent, or --reviewer
agent_dispatch rows missing --target or --agent in derived argv
```

Prefer checking `operation` first and treating `invoke` as derived/debug. Error wording should include `invoke.argv contradicts operation`.

- [ ] **Step 3: Verify preflight tests**

Run:

```bash
go test ./internal/swim -run 'TestValidateScheduleV3RejectsContradictoryInvoke|TestValidateScheduleRejectsInvalidV3OperationPair' -count=1
```

Expected: pass.

---

### Task 7: Update SWIM Runtime Docs And MCP Surface Text

**Files:**
- Modify: `docs/specs/2026-05-05-swim-ops.md`
- Modify: `README.md`
- Modify: `main.go`
- Test: `go test ./...`

- [ ] **Step 1: Update operator examples**

Replace examples that show `wp-plan-to-agent.sh` as the schedule invoker with `wp-plan-step.sh`.

Add the compatibility note:

```markdown
`wp-plan-to-agent.sh` and `wp-task-to-agent.sh` remain compatibility wrappers.
New schedules should use `wp-plan-step.sh`; direct use of `wp-agent-dispatch.sh`
is reserved for tests and support tooling.
```

- [ ] **Step 2: Update MCP tool descriptions if they mention v2-only schedule rows**

In `main.go`, adjust SWIM compile/run descriptions to say that schedule v3 uses structured `operation` and derived invocation.

- [ ] **Step 3: Run full Go tests**

Run:

```bash
go test ./...
```

Expected: pass.

---

### Task 8: Final Verification

**Files:**
- No new files unless test fixtures update.
- Test: whole affected surface.

- [ ] **Step 1: Run targeted shell tests**

Run:

```bash
bash tests/swim/test_t1_2_invoke_argv.sh
bash tests/swim/test_wp_plan_step_contract.sh
bash tests/swim/test_wp_task_to_agent_fix_mode.sh
bash tests/swim/test_wp_task_to_agent_codex_exec.sh
```

Expected: all pass.

- [ ] **Step 2: Run Go SWIM tests**

Run:

```bash
go test ./internal/swim ./cmd/swim-run ./cmd/swim-step ./cmd/swim-validate
```

Expected: pass.

- [ ] **Step 3: Run full test suite**

Run:

```bash
go test ./...
```

Expected: pass.

- [ ] **Step 4: Validate generated schedule**

Run:

```bash
WAVEPLAN_CLI_BIN="$PWD/waveplan-cli" bash wp-emit-wave-execution.sh \
  --plan docs/plans/2026-05-05-swim-execution-waves.json \
  --agents tests/swim/fixtures/waveagents.json \
  --task-scope all > /tmp/swim-v3-schedule.json

go test ./internal/swim -run TestEmitterGoldenSchedule -count=1
```

Expected: emitted schedule validates as schema v3 and uses only `wp-plan-step.sh` in derived `invoke.argv`.

---

## Completion Criteria

- New schedules are `schema_version: 3`.
- Every schedule row has `operation`.
- `operation` is the source of truth; `invoke.argv` is derived/debug.
- `swim run` invokes only `wp-plan-step.sh` for schedule rows.
- `wp-agent-dispatch.sh` never receives `--task-id` and never mutates Waveplan state.
- Legacy scripts still provide clear compatibility behavior.
- Validation catches the original failure mode before execution.
- Documentation explains the distinction between scheduled plan steps and internal agent dispatch.
