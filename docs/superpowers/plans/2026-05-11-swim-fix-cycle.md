# SWIM Fix-Cycle Action Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `fix` dispatch action to the SWIM state machine that transitions `review_taken → taken`, enabling pre-planned review/fix cycles in the execution schedule.

**Architecture:** The `fix` action is added as a receipt-bearing dispatch step — identical semantics to `implement` and `review`. The schedule schema and Go state machine gain the new transition. The launcher (`wp-task-to-agent.sh`) gains a `--mode fix` that finds the currently taken task, attaches prior review stdout as context, and delivers a fix prompt. The waveplan MCP `start_fix` command (which clears `review_entered_at` so `StatusOf` returns `taken`) is a named dependency, stubbed in all tests here.

**Tech Stack:** Go 1.22+, `bash` (shell launcher), JSON Schema draft-2020-12. Test helpers: `writeSchedule`, `snapshotWithTaskStatus`, `writeJournal` from `internal/swim/runner_test.go` / `evaluator_test.go`.

**Out of scope (separate plan):** waveplan MCP `start_fix` command implementation; `state_adapter.go` changes to persist fix audit trail; dynamic fix-round insertion at runtime.

---

## File Map

| File | Change |
|------|--------|
| `docs/specs/swim-schedule-schema-v2.json` | Add `fix` to `action` enum; extend `step_id` pattern to allow `_r<N>` suffix |
| `internal/swim/contracts.go` | Add `fix` to `statusByAction` map |
| `internal/swim/dispatch.go` | Add `fix` to `isDispatchAction` |
| `internal/swim/safe_runner.go` | Set `SWIM_PRIOR_STDOUT_PATH` in `extraEnv` for `fix` steps |
| `internal/swim/contracts_test.go` | Add `TestValidateSchedule_FixCycle` |
| `internal/swim/apply_test.go` | Add `TestApply_FixAction_HappyPath`, `TestApply_FixAction_IncompleteDispatch` |
| `internal/swim/safe_runner_test.go` | Add `TestExecuteNextStepSafe_FixInjectsPriorStdoutPath` |
| `wp-task-to-agent.sh` | Add `fix` to mode validation and `--mode fix` implementation block |
| `tests/swim/test_wp_task_to_agent_fix_mode.sh` | New integration test for fix mode dispatch |

---

## Task 1: Schema — allow `fix` action and `_r<N>` step_id suffix

**Files:**
- Modify: `docs/specs/swim-schedule-schema-v2.json`

- [ ] **Step 1: Write failing test**

In `internal/swim/contracts_test.go`, add a new test function after `TestValidateJournal_Cases`:

```go
func TestValidateSchedule_FixCycle(t *testing.T) {
	rows := []ScheduleRow{
		{Seq: 1, StepID: "S1_T1.1_implement", TaskID: "T1.1", Action: "implement",
			Requires: StatusWrapper{TaskStatus: "available"}, Produces: StatusWrapper{TaskStatus: "taken"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 2, StepID: "S2_T1.1_review", TaskID: "T1.1", Action: "review",
			Requires: StatusWrapper{TaskStatus: "taken"}, Produces: StatusWrapper{TaskStatus: "review_taken"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 3, StepID: "S3_T1.1_fix", TaskID: "T1.1", Action: "fix",
			Requires: StatusWrapper{TaskStatus: "review_taken"}, Produces: StatusWrapper{TaskStatus: "taken"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 4, StepID: "S4_T1.1_review_r2", TaskID: "T1.1", Action: "review",
			Requires: StatusWrapper{TaskStatus: "taken"}, Produces: StatusWrapper{TaskStatus: "review_taken"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 5, StepID: "S5_T1.1_end_review", TaskID: "T1.1", Action: "end_review",
			Requires: StatusWrapper{TaskStatus: "review_taken"}, Produces: StatusWrapper{TaskStatus: "review_ended"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 6, StepID: "S6_T1.1_finish", TaskID: "T1.1", Action: "finish",
			Requires: StatusWrapper{TaskStatus: "review_ended"}, Produces: StatusWrapper{TaskStatus: "completed"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
	}
	raw := marshalJSON(t, Schedule{SchemaVersion: 2, Execution: rows})
	if err := ValidateSchedule(raw); err != nil {
		t.Fatalf("ValidateSchedule fix cycle: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/darkbit1001/workspace/waveplan-mcp
go test ./internal/swim -run TestValidateSchedule_FixCycle -v -count=1
```

Expected: FAIL with `schema validation failed` (action `fix` not in enum, `step_id` `S3_T1.1_fix` not matching pattern, `_r2` suffix not matching pattern).

- [ ] **Step 3: Update the schema**

In `docs/specs/swim-schedule-schema-v2.json`, make two changes:

Change the `step_id` pattern from:
```json
"pattern": "^S[0-9]+_[A-Za-z0-9._\\-]+_(implement|review|end_review|finish)$"
```
to:
```json
"pattern": "^S[0-9]+_[A-Za-z0-9._\\-]+_(implement|review|end_review|finish|fix)(_r[0-9]+)?$"
```

Change the `action` enum from:
```json
"enum": ["implement", "review", "end_review", "finish"]
```
to:
```json
"enum": ["implement", "review", "end_review", "finish", "fix"]
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/swim -run TestValidateSchedule_FixCycle -v -count=1
```

Expected: PASS.

- [ ] **Step 5: Verify no schema regressions**

```bash
go test ./internal/swim -run TestValidateSchedule_Cases -v -count=1
```

Expected: all subtests PASS (existing cases unaffected).

- [ ] **Step 6: Commit**

```bash
git add docs/specs/swim-schedule-schema-v2.json internal/swim/contracts_test.go
git commit -m "feat(swim): schema v2 allows fix action and _rN step_id suffix"
```

---

## Task 2: `contracts.go` — register `fix` in the state machine

**Files:**
- Modify: `internal/swim/contracts.go:32-40`

- [ ] **Step 1: Write failing test**

Add a second test function to `internal/swim/contracts_test.go`:

```go
func TestValidateSchedule_FixRequiresProducesCheck(t *testing.T) {
	badRow := []ScheduleRow{
		{Seq: 1, StepID: "S1_T1.1_fix", TaskID: "T1.1", Action: "fix",
			// wrong: fix must require review_taken, not taken
			Requires: StatusWrapper{TaskStatus: "taken"},
			Produces: StatusWrapper{TaskStatus: "taken"},
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
	}
	raw := marshalJSON(t, Schedule{SchemaVersion: 2, Execution: badRow})
	err := ValidateSchedule(raw)
	if err == nil {
		t.Fatal("expected requires/produces mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "requires/produces mismatch") {
		t.Fatalf("error = %v, want requires/produces mismatch", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/swim -run TestValidateSchedule_FixRequiresProducesCheck -v -count=1
```

Expected: FAIL — `ValidateSchedule` returns nil (no `fix` entry in `statusByAction` yet, so the `invalid action` check fires instead of `requires/produces mismatch`). The error message won't match.

- [ ] **Step 3: Add `fix` to `statusByAction`**

In `internal/swim/contracts.go`, change:

```go
var statusByAction = map[string]struct {
	requires string
	produces string
}{
	"implement":  {requires: "available", produces: "taken"},
	"review":     {requires: "taken", produces: "review_taken"},
	"end_review": {requires: "review_taken", produces: "review_ended"},
	"finish":     {requires: "review_ended", produces: "completed"},
}
```

to:

```go
var statusByAction = map[string]struct {
	requires string
	produces string
}{
	"implement":  {requires: "available", produces: "taken"},
	"review":     {requires: "taken", produces: "review_taken"},
	"fix":        {requires: "review_taken", produces: "taken"},
	"end_review": {requires: "review_taken", produces: "review_ended"},
	"finish":     {requires: "review_ended", produces: "completed"},
}
```

- [ ] **Step 4: Run both contracts tests**

```bash
go test ./internal/swim -run 'TestValidateSchedule_(FixCycle|FixRequiresProducesCheck)' -v -count=1
```

Expected: both PASS.

- [ ] **Step 5: Run full suite to check regressions**

```bash
go test ./internal/swim -count=1
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/swim/contracts.go internal/swim/contracts_test.go
git commit -m "feat(swim): add fix action to state machine (review_taken → taken)"
```

---

## Task 3: `dispatch.go` — `fix` is a receipt-bearing dispatch action

**Files:**
- Modify: `internal/swim/dispatch.go:8-15`

- [ ] **Step 1: Write failing test**

Add to `internal/swim/apply_test.go`:

```go
func TestApply_FixAction_HappyPath(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "fix-happy-schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_fix",
			TaskID:   "T1.1",
			Action:   "fix",
			Requires: StatusWrapper{TaskStatus: string(StatusReviewTaken)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
		},
	})

	var reads int
	report, err := Apply(ApplyOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			if reads < 3 {
				return snapshotWithTaskStatus("T1.1", StatusReviewTaken), nil
			}
			return snapshotWithTaskStatus("T1.1", StatusTaken), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			receiptPath := os.Getenv("SWIM_DISPATCH_RECEIPT_PATH")
			if receiptPath == "" {
				t.Fatal("missing SWIM_DISPATCH_RECEIPT_PATH for fix step")
			}
			if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
				t.Fatalf("MkdirAll receipt dir: %v", err)
			}
			if err := os.WriteFile(receiptPath, []byte(`{"ok":true}`+"\n"), 0o644); err != nil {
				t.Fatalf("WriteFile receipt: %v", err)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "applied" {
		t.Fatalf("status = %q, reason = %q, want applied", report.Status, report.Reason)
	}
	if report.StepID != "S1_T1.1_fix" {
		t.Fatalf("step_id = %q, want S1_T1.1_fix", report.StepID)
	}

	j := readJournal(t, journalPath)
	if j.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", j.Cursor)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/swim -run TestApply_FixAction_HappyPath -v -count=1
```

Expected: FAIL — `Apply` returns `status = "blocked"` with `postcondition_mismatch` because `fix` is not yet in `isDispatchAction`, so no receipt env var is set, and the postcondition check sees the state returned `taken` but receipt is absent... actually it blocks because the `fix` action's `Predict(row)` is now `taken` (Task 2 done), but since `isDispatchAction("fix")` is false, no receipt path is injected and no receipt validation happens. The test will fail because `SWIM_DISPATCH_RECEIPT_PATH` is empty in `InvokeFn`, triggering `t.Fatal`.

- [ ] **Step 3: Add `fix` to `isDispatchAction`**

In `internal/swim/dispatch.go`, change:

```go
func isDispatchAction(action string) bool {
	switch action {
	case "implement", "review":
		return true
	default:
		return false
	}
}
```

to:

```go
func isDispatchAction(action string) bool {
	switch action {
	case "implement", "review", "fix":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/swim -run TestApply_FixAction_HappyPath -v -count=1
```

Expected: PASS.

- [ ] **Step 5: Add incomplete_dispatch test for fix**

Append to `internal/swim/apply_test.go`:

```go
func TestApply_FixAction_IncompleteDispatch(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "fix-incomplete-schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_fix",
			TaskID:   "T1.1",
			Action:   "fix",
			Requires: StatusWrapper{TaskStatus: string(StatusReviewTaken)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "exit 1"}},
		},
	})

	var reads int
	report, err := Apply(ApplyOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			if reads < 3 {
				return snapshotWithTaskStatus("T1.1", StatusReviewTaken), nil
			}
			// state advanced (waveplan start_fix succeeded) but receipt absent
			return snapshotWithTaskStatus("T1.1", StatusTaken), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			return exitError(1)
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Status != "incomplete_dispatch" {
		t.Fatalf("status = %q, reason = %q, want incomplete_dispatch", report.Status, report.Reason)
	}
	if !strings.Contains(report.Reason, "receipt_missing") {
		t.Fatalf("reason = %q, want receipt_missing", report.Reason)
	}

	j := readJournal(t, journalPath)
	if j.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", j.Cursor)
	}
}
```

- [ ] **Step 6: Run both fix apply tests**

```bash
go test ./internal/swim -run 'TestApply_FixAction' -v -count=1
```

Expected: both PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/swim/dispatch.go internal/swim/apply_test.go
git commit -m "feat(swim): fix is a receipt-bearing dispatch action"
```

---

## Task 4: `safe_runner.go` — inject `SWIM_PRIOR_STDOUT_PATH` for fix steps

**Files:**
- Modify: `internal/swim/safe_runner.go:144-152`
- Test: `internal/swim/safe_runner_test.go`

- [ ] **Step 1: Write failing test**

Add to `internal/swim/safe_runner_test.go`:

```go
func TestExecuteNextStepSafe_FixInjectsPriorStdoutPath(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "fix-prior-stdout-schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_fix",
			TaskID:   "T1.1",
			Action:   "fix",
			Requires: StatusWrapper{TaskStatus: string(StatusReviewTaken)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
		},
	})

	// Pre-populate journal with a completed review event that has a stdout_path.
	priorStdout := ".waveplan/swim/fix-prior-stdout-schedule/logs/S0_T1.1_review.1.stdout.log"
	writeJournal(t, journalPath, Journal{
		SchemaVersion: 1,
		SchedulePath:  schedulePath,
		Cursor:        0,
		Events: []JournalEvent{
			{
				EventID:     "E0001",
				StepID:      "S0_T1.1_review",
				Seq:         0,
				TaskID:      "T1.1",
				Action:      "review",
				Attempt:     1,
				StartedOn:   "2026-05-11T00:00:00Z",
				CompletedOn: "2026-05-11T00:00:01Z",
				Outcome:     "applied",
				StateBefore: StatusWrapper{TaskStatus: string(StatusTaken)},
				StateAfter:  StatusWrapper{TaskStatus: string(StatusReviewTaken)},
				StdoutPath:  priorStdout,
			},
		},
	})

	var capturedPriorPath string
	var reads int
	_, err := ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			reads++
			if reads < 3 {
				return snapshotWithTaskStatus("T1.1", StatusReviewTaken), nil
			}
			return snapshotWithTaskStatus("T1.1", StatusTaken), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			capturedPriorPath = os.Getenv("SWIM_PRIOR_STDOUT_PATH")
			receiptPath := os.Getenv("SWIM_DISPATCH_RECEIPT_PATH")
			if receiptPath != "" {
				_ = os.MkdirAll(filepath.Dir(receiptPath), 0o755)
				_ = os.WriteFile(receiptPath, []byte(`{"ok":true}`+"\n"), 0o644)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteNextStepSafe: %v", err)
	}

	wantSuffix := priorStdout
	if !strings.HasSuffix(capturedPriorPath, wantSuffix) {
		t.Fatalf("SWIM_PRIOR_STDOUT_PATH = %q, want path ending in %q", capturedPriorPath, wantSuffix)
	}
	if !filepath.IsAbs(capturedPriorPath) {
		t.Fatalf("SWIM_PRIOR_STDOUT_PATH = %q, want absolute path", capturedPriorPath)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/swim -run TestExecuteNextStepSafe_FixInjectsPriorStdoutPath -v -count=1
```

Expected: FAIL — `capturedPriorPath` is empty because `SWIM_PRIOR_STDOUT_PATH` is not yet set.

- [ ] **Step 3: Implement `SWIM_PRIOR_STDOUT_PATH` injection**

In `internal/swim/safe_runner.go`, find the `extraEnv` block (around line 144):

```go
	extraEnv := map[string]string{}
	if isDispatchAction(decision.Row.Action) {
		receiptPath = deriveDispatchReceiptPath(opts.SchedulePath, decision.Row.StepID, event.Attempt)
		extraEnv["SWIM_DISPATCH_RECEIPT_PATH"] = dispatchReceiptAbsPath(opts.SchedulePath, decision.Row.StepID, event.Attempt)
		extraEnv["SWIM_STEP_ID"] = decision.Row.StepID
		extraEnv["SWIM_TASK_ID"] = decision.Row.TaskID
		extraEnv["SWIM_ACTION"] = decision.Row.Action
	}
```

Change it to:

```go
	extraEnv := map[string]string{}
	if isDispatchAction(decision.Row.Action) {
		receiptPath = deriveDispatchReceiptPath(opts.SchedulePath, decision.Row.StepID, event.Attempt)
		extraEnv["SWIM_DISPATCH_RECEIPT_PATH"] = dispatchReceiptAbsPath(opts.SchedulePath, decision.Row.StepID, event.Attempt)
		extraEnv["SWIM_STEP_ID"] = decision.Row.StepID
		extraEnv["SWIM_TASK_ID"] = decision.Row.TaskID
		extraEnv["SWIM_ACTION"] = decision.Row.Action
	}
	if decision.Row.Action == "fix" {
		for i := len(journal.Events) - 1; i >= 0; i-- {
			ev := journal.Events[i]
			if ev.Action == "review" && ev.StdoutPath != "" {
				extraEnv["SWIM_PRIOR_STDOUT_PATH"] = filepath.Join(filepath.Dir(opts.SchedulePath), ev.StdoutPath)
				break
			}
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/swim -run TestExecuteNextStepSafe_FixInjectsPriorStdoutPath -v -count=1
```

Expected: PASS.

- [ ] **Step 5: Run full suite**

```bash
go test ./internal/swim -count=1
go test ./cmd/swim-step -count=1
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/swim/safe_runner.go internal/swim/safe_runner_test.go
git commit -m "feat(swim): inject SWIM_PRIOR_STDOUT_PATH for fix dispatch steps"
```

---

## Task 5: `wp-task-to-agent.sh` — `--mode fix` implementation

**Files:**
- Modify: `wp-task-to-agent.sh`

The fix mode:
1. Validates `--mode fix` is accepted (currently rejected).
2. Calls `waveplan_cli --plan "$PLAN" start_fix "$TASK_ID"` to transition the task `review_taken → taken`.  
   **Dependency:** `start_fix` must exist in the waveplan MCP/CLI. Stub it in tests to return `{"ok":true}`.
3. Reads `SWIM_PRIOR_STDOUT_PATH` to include reviewer findings in the prompt (the file may not exist if the prior review produced no stdout; the script must handle this gracefully).
4. Delivers a fix prompt to the target CLI.
5. Writes a dispatch receipt.

- [ ] **Step 1: Write failing shell test**

Create `tests/swim/test_wp_task_to_agent_fix_mode.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

PLAN="$TMP/plan.json"
CLI_STUB="$TMP/waveplan-cli-stub.py"
CODEX_STUB="$TMP/codex"
ARGS_OUT="$TMP/codex-args.txt"
STDIN_OUT="$TMP/codex-stdin.txt"
RECEIPT_OUT="$TMP/receipt.json"
PRIOR_STDOUT="$TMP/prior-review.txt"

printf '{}\n' >"$PLAN"
printf 'reviewer found: missing error handling in auth module\n' >"$PRIOR_STDOUT"

cat >"$CLI_STUB" <<'PY'
#!/usr/bin/env python3
import json, sys

args = sys.argv[1:]
if "get" in args:
    print(json.dumps({"tasks": [{"task_id": "T1.1", "title": "stub", "status": "taken"}]}))
elif "start_fix" in args:
    print(json.dumps({"ok": True, "task_id": args[-1]}))
else:
    print(json.dumps({"ok": True}))
PY
chmod +x "$CLI_STUB"

cat >"$CODEX_STUB" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" >"$ARGS_OUT"
cat >"$STDIN_OUT" || true
SH
chmod +x "$CODEX_STUB"

export PATH="$TMP:$PATH"
export WAVEPLAN_CLI_BIN="$CLI_STUB"
export SWIM_DISPATCH_RECEIPT_PATH="$RECEIPT_OUT"
export SWIM_PRIOR_STDOUT_PATH="$PRIOR_STDOUT"

"$ROOT/wp-task-to-agent.sh" \
  --target codex \
  --plan "$PLAN" \
  --agent sigma \
  --mode fix

if [[ ! -f "$ARGS_OUT" ]]; then
  echo "FAIL: codex stub was not invoked"
  exit 1
fi

first_arg="$(sed -n '1p' "$ARGS_OUT")"
if [[ "$first_arg" != "exec" ]]; then
  echo "FAIL: expected codex exec, got: ${first_arg:-<empty>}"
  exit 1
fi

grep -q "fix" "$STDIN_OUT" || {
  echo "FAIL: fix prompt not delivered to codex"
  exit 1
}

grep -q "reviewer found" "$STDIN_OUT" || {
  echo "FAIL: prior review content not included in fix prompt"
  exit 1
}

test -f "$RECEIPT_OUT" || {
  echo "FAIL: dispatch receipt was not written"
  exit 1
}

grep -q '"ok": true' "$RECEIPT_OUT" || {
  echo "FAIL: receipt does not contain ok=true"
  exit 1
}

echo "PASS: wp-task-to-agent fix mode dispatches prompt with review context and writes receipt"
```

```bash
chmod +x tests/swim/test_wp_task_to_agent_fix_mode.sh
```

- [ ] **Step 2: Run test to verify it fails**

```bash
bash tests/swim/test_wp_task_to_agent_fix_mode.sh
```

Expected: FAIL — `Invalid --mode: fix`.

- [ ] **Step 3: Add `fix` to mode validation**

In `wp-task-to-agent.sh`, change:

```bash
if [[ "$MODE" != "implement" && "$MODE" != "review" ]]; then
  echo "Invalid --mode: $MODE (must be implement or review)" >&2
  exit 2
fi
```

to:

```bash
if [[ "$MODE" != "implement" && "$MODE" != "review" && "$MODE" != "fix" ]]; then
  echo "Invalid --mode: $MODE (must be implement, review, or fix)" >&2
  exit 2
fi
```

Also update the usage block's `--mode` line from:
```
    [--mode implement|review] \
```
to:
```
    [--mode implement|review|fix] \
```

And the `--mode` description in the usage body:
```
  --mode <mode>          implement (default) | review | fix
```

- [ ] **Step 4: Add `fix` mode implementation block**

In `wp-task-to-agent.sh`, after the `review` mode block (after the final `write_dispatch_receipt` call), this is currently the end of the file. The `fix` block goes before the `review` block (after `implement`, before `review`). In practice, add the fix block right after the `if [[ "$MODE" == "implement" ]]; then ... fi` block and before the `# review mode` comment:

```bash
# fix mode
if [[ "$MODE" == "fix" ]]; then
  SELECTED="$(select_taken_task_for_agent "$PLAN" "$AGENT" || true)"
  TASK_ID="$(printf '%s\n' "$SELECTED" | sed -n '1p')"
  CURRENT_TASK_JSON="$(printf '%s\n' "$SELECTED" | sed -n '2,$p')"

  if [[ -z "$TASK_ID" || -z "$CURRENT_TASK_JSON" ]]; then
    echo "No currently taken task found for agent '$AGENT'" >&2
    exit 1
  fi

  if [[ "$DRY_RUN" == "1" ]]; then
    FIX_RESULT_JSON="{\"dry_run\":true,\"would_run\":\"python3 $WAVEPLAN_CLI_BIN --plan $PLAN start_fix $TASK_ID\"}"
  else
    FIX_RESULT_JSON="$(waveplan_cli --plan "$PLAN" start_fix "$TASK_ID")"
    if json_has_error "$FIX_RESULT_JSON"; then
      echo "waveplan start_fix returned error payload:" >&2
      echo "$FIX_RESULT_JSON" >&2
      exit 1
    fi
  fi

  PRIOR_REVIEW_CONTENT=""
  PRIOR_STDOUT_PATH="${SWIM_PRIOR_STDOUT_PATH:-}"
  if [[ -n "$PRIOR_STDOUT_PATH" && -f "$PRIOR_STDOUT_PATH" ]]; then
    PRIOR_REVIEW_CONTENT="$(cat "$PRIOR_STDOUT_PATH")"
  fi

  read -r -d '' PREAMBLE <<'TXT' || true
fix cycle: address reviewer findings.

hard requirements:
- use superpowers skill workflow while implementing fixes.
- waveplan-cli / waveplan-mcp usage in this task is read-only only.
- do not execute waveplan write actions: no pop, no fin, no start_review, no end_review, no plan mutations.
- allowed waveplan reads: peek, get, deptree, list_plans.

apply all required fixes from the reviewer findings below.
run tests relevant to changes.
return concrete file changes + verification commands/results.
TXT

  PROMPT="$PREAMBLE

plan: $PLAN
claimed_by: $AGENT
mode: fix

task_json:
$CURRENT_TASK_JSON

reviewer_findings:
${PRIOR_REVIEW_CONTENT:-<no prior review stdout available>}
"

  send_prompt "$PROMPT"
  write_dispatch_receipt "$TASK_ID" "fix_taken"
  exit 0
fi
```

- [ ] **Step 5: Run shell test to verify it passes**

```bash
bash tests/swim/test_wp_task_to_agent_fix_mode.sh
```

Expected: `PASS: wp-task-to-agent fix mode dispatches prompt with review context and writes receipt`.

- [ ] **Step 6: Run existing shell tests to verify no regressions**

```bash
bash tests/swim/test_wp_task_to_agent_codex_exec.sh
bash tests/swim/test_t4_2_cli_wired.sh
bash tests/swim/test_installed_anydir.sh
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add wp-task-to-agent.sh tests/swim/test_wp_task_to_agent_fix_mode.sh
git commit -m "feat(swim): add --mode fix to wp-task-to-agent.sh with reviewer context injection"
```

---

## Task 6: `run.go` — `until: "fix"` support in `matchesUntil`

**Files:**
- Modify: `internal/swim/run.go:152-158` (parseUntil actions list)
- Modify: `internal/swim/run.go:177-189` (matchesUntil for action kind)

Without this change, `until: "fix"` stops correctly (step_ids ending `_fix` match `HasSuffix`), but step_ids with `_r<N>` suffixes like `S3_T1.1_fix_r1` do not. Fix it.

- [ ] **Step 1: Write failing test**

Add to `internal/swim/run_test.go` (find the existing `TestRun_*` tests and append):

```go
func TestRun_UntilFixWithRoundSuffix(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")
	statePath := filepath.Join(dir, "state.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{Seq: 1, StepID: "S1_T1.1_implement", TaskID: "T1.1", Action: "implement",
			Requires: StatusWrapper{TaskStatus: "available"}, Produces: StatusWrapper{TaskStatus: "taken"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 2, StepID: "S2_T1.1_review", TaskID: "T1.1", Action: "review",
			Requires: StatusWrapper{TaskStatus: "taken"}, Produces: StatusWrapper{TaskStatus: "review_taken"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
		{Seq: 3, StepID: "S3_T1.1_fix_r1", TaskID: "T1.1", Action: "fix",
			Requires: StatusWrapper{TaskStatus: "review_taken"}, Produces: StatusWrapper{TaskStatus: "taken"},
			Invoke: InvokeSpec{Argv: []string{"bash", "-lc", "true"}}},
	})

	var reads int
	statuses := []Status{StatusAvailable, StatusAvailable, StatusTaken, StatusTaken, StatusReviewTaken, StatusReviewTaken, StatusTaken}
	report, err := Run(RunOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		StatePath:    statePath,
		Until:        "fix",
		ReadSnapshotFn: func(_ string) (*StateSnapshot, error) {
			idx := reads
			if idx >= len(statuses) {
				idx = len(statuses) - 1
			}
			reads++
			return snapshotWithTaskStatus("T1.1", statuses[idx]), nil
		},
		InvokeFn: func(_ []string, _ string) error {
			receiptPath := os.Getenv("SWIM_DISPATCH_RECEIPT_PATH")
			if receiptPath != "" {
				_ = os.MkdirAll(filepath.Dir(receiptPath), 0o755)
				_ = os.WriteFile(receiptPath, []byte(`{"ok":true}`+"\n"), 0o644)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Stopped != "until" {
		t.Fatalf("stopped = %q, want until", report.Stopped)
	}
	if !report.ReachedUntil {
		t.Fatal("expected ReachedUntil=true")
	}
	// Should have stopped after the fix step (seq 3), not before.
	if len(report.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(report.Steps))
	}
	last := report.Steps[len(report.Steps)-1]
	if last.StepID != "S3_T1.1_fix_r1" {
		t.Fatalf("last step_id = %q, want S3_T1.1_fix_r1", last.StepID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/swim -run TestRun_UntilFixWithRoundSuffix -v -count=1
```

Expected: FAIL — run stops early because `matchesUntil` with `action=fix` doesn't match `S3_T1.1_fix_r1`.

- [ ] **Step 3: Update `parseUntil` and `matchesUntil`**

In `internal/swim/run.go`, change `parseUntil` action list:

```go
switch raw {
case "implement", "review", "end_review", "finish":
```

to:

```go
switch raw {
case "implement", "review", "end_review", "finish", "fix":
```

Change `matchesUntil`:

```go
case untilAction:
    return strings.HasSuffix(step.StepID, "_"+cond.action)
```

to:

```go
case untilAction:
    suffix := "_" + cond.action
    return strings.HasSuffix(step.StepID, suffix) ||
        strings.Contains(step.StepID, suffix+"_r")
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/swim -run TestRun_UntilFixWithRoundSuffix -v -count=1
```

Expected: PASS.

- [ ] **Step 5: Run full suite**

```bash
go test ./internal/swim -count=1
go test ./cmd/swim-step -count=1
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/swim/run.go internal/swim/run_test.go
git commit -m "feat(swim): until:fix stops at fix steps including _rN suffixed ids"
```

---

## Self-Review Checklist

### Spec coverage

| Spec requirement | Task |
|---|---|
| `fix` action: `review_taken → taken` | Task 2 |
| `fix` is receipt-bearing | Task 3 |
| Schema allows fix + `_rN` step_ids | Task 1 |
| `SWIM_PRIOR_STDOUT_PATH` injected for fix steps | Task 4 |
| Launcher `--mode fix` with findings attachment | Task 5 |
| `until: "fix"` works with round suffixes | Task 6 |
| `incomplete_dispatch` for fix steps | Task 3, step 5 |
| Receipt path convention unchanged | Covered by existing receipt tests + Task 3 |

### Dependency

`waveplan_cli start_fix` is called in `wp-task-to-agent.sh` fix mode and stubbed in the shell test. Before end-to-end use, the waveplan MCP server must implement:

- `start_fix(task_id)` — transitions the task from `review_taken` to `taken` in the state file (mechanism: clear `review_entered_at`, or add `fix_opened_at` — see `state_adapter.go` for `StatusOf` logic).

This is a separate plan item.
