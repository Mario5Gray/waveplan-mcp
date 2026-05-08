package swim

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	expectedScheduleFixture = "tests/swim/fixtures/expected-schedule.json"
	emitPlanPath            = "docs/plans/2026-05-05-swim-execution-waves.json"
	emitAgentsPath          = "tests/swim/fixtures/waveagents.json"
)

func TestValidateSchedule_Cases(t *testing.T) {
	baseRaw := loadExpectedScheduleFixture(t)
	base := decodeJSONMap(t, baseRaw)

	tests := []struct {
		name    string
		mutate  func(t *testing.T, m map[string]any)
		wantErr string
	}{
		{
			name:    "valid",
			mutate:  nil,
			wantErr: "",
		},
		{
			name: "missing required field",
			mutate: func(t *testing.T, m map[string]any) {
				row0 := mustRow(t, m, 0)
				delete(row0, "step_id")
			},
			wantErr: "schema validation failed",
		},
		{
			name: "bad enum action",
			mutate: func(t *testing.T, m map[string]any) {
				row0 := mustRow(t, m, 0)
				row0["action"] = "lol"
			},
			wantErr: "schema validation failed",
		},
		{
			name: "duplicate step_id",
			mutate: func(t *testing.T, m map[string]any) {
				row0 := mustRow(t, m, 0)
				row1 := mustRow(t, m, 1)
				row1["step_id"] = row0["step_id"]
			},
			wantErr: "duplicate step_id",
		},
		{
			name: "non-monotonic seq",
			mutate: func(t *testing.T, m map[string]any) {
				row0 := mustRow(t, m, 0)
				row1 := mustRow(t, m, 1)
				row1["seq"] = row0["seq"]
			},
			wantErr: "seq not strictly increasing",
		},
		{
			name: "malformed argv",
			mutate: func(t *testing.T, m map[string]any) {
				row0 := mustRow(t, m, 0)
				row0["invoke"] = map[string]any{"argv": []any{}}
			},
			wantErr: "schema validation failed",
		},
		{
			name: "schedule_version mismatch",
			mutate: func(t *testing.T, m map[string]any) {
				m["schema_version"] = 1
			},
			wantErr: "schema validation failed",
		},
		{
			name: "invalid step_id pattern",
			mutate: func(t *testing.T, m map[string]any) {
				row0 := mustRow(t, m, 0)
				row0["step_id"] = "bad-step-id"
			},
			wantErr: "schema validation failed",
		},
		{
			name: "requires/produces mismatch",
			mutate: func(t *testing.T, m map[string]any) {
				row0 := mustRow(t, m, 0)
				requires, ok := row0["requires"].(map[string]any)
				if !ok {
					t.Fatalf("requires not object")
				}
				requires["task_status"] = "taken"
			},
			wantErr: "requires/produces mismatch",
		},
		{
			name: "argv first token empty",
			mutate: func(t *testing.T, m map[string]any) {
				row0 := mustRow(t, m, 0)
				row0["invoke"] = map[string]any{"argv": []any{"", "--mode", "implement"}}
			},
			wantErr: "malformed argv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := deepCopyJSONMap(t, base)
			if tt.mutate != nil {
				tt.mutate(t, payload)
			}
			raw := marshalJSON(t, payload)
			err := ValidateSchedule(raw)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateSchedule() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateSchedule() expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateSchedule() error mismatch\nwant contains: %q\ngot: %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateJournal_Cases(t *testing.T) {
	baseSchedule := loadExpectedScheduleFixture(t)
	var sched Schedule
	if err := json.Unmarshal(baseSchedule, &sched); err != nil {
		t.Fatalf("decode schedule fixture: %v", err)
	}
	if len(sched.Execution) == 0 {
		t.Fatalf("expected non-empty execution in schedule fixture")
	}

	validJournal := map[string]any{
		"schema_version": 1,
		"schedule_path":  emitPlanPath,
		"cursor":         1,
		"events": []any{
			map[string]any{
				"event_id":     "E0001",
				"step_id":      sched.Execution[0].StepID,
				"seq":          1,
				"task_id":      sched.Execution[0].TaskID,
				"action":       sched.Execution[0].Action,
				"attempt":      1,
				"started_on":   "2026-05-05T12:00:00Z",
				"completed_on": "2026-05-05T12:00:01Z",
				"outcome":      "applied",
				"state_before": map[string]any{"task_status": sched.Execution[0].Requires.TaskStatus},
				"state_after":  map[string]any{"task_status": sched.Execution[0].Produces.TaskStatus},
			},
		},
	}

	validRaw := marshalJSON(t, validJournal)
	if err := ValidateJournal(validRaw); err != nil {
		t.Fatalf("ValidateJournal(valid) unexpected error: %v", err)
	}

	waivedMissingOperator := deepCopyJSONMap(t, validJournal)
	events, ok := waivedMissingOperator["events"].([]any)
	if !ok || len(events) == 0 {
		t.Fatalf("invalid events payload")
	}
	e0, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("event[0] not object")
	}
	e0["outcome"] = "waived"
	e0["reason"] = "approved override"
	e0["waived_on"] = "2026-05-05T12:00:02Z"
	delete(e0, "operator")

	badRaw := marshalJSON(t, waivedMissingOperator)
	err := ValidateJournal(badRaw)
	if err == nil {
		t.Fatalf("ValidateJournal() expected waived-without-operator error")
	}
	if !strings.Contains(err.Error(), "schema validation failed") {
		t.Fatalf("ValidateJournal() unexpected error: %v", err)
	}
}

func TestEmitterGoldenSchedule(t *testing.T) {
	root := mustRepoRoot(t)
	expectedPath := filepath.Join(root, expectedScheduleFixture)
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected fixture: %v", err)
	}

	cmd := exec.Command("bash", "wp-emit-wave-execution.sh", "--plan", emitPlanPath, "--agents", emitAgentsPath, "--task-scope", "all")
	cmd.Dir = root
	actual, err := cmd.Output()
	if err != nil {
		t.Fatalf("run emitter: %v", err)
	}

	if !bytes.Equal(actual, expected) {
		t.Fatalf("golden mismatch\nactual_sha256=%s\nexpected_sha256=%s", sha256Hex(actual), sha256Hex(expected))
	}

	if err := ValidateSchedule(actual); err != nil {
		t.Fatalf("ValidateSchedule(golden emitter output) unexpected error: %v", err)
	}
}

func mustRow(t *testing.T, m map[string]any, idx int) map[string]any {
	t.Helper()
	execRows, ok := m["execution"].([]any)
	if !ok {
		t.Fatalf("execution is not array")
	}
	if idx < 0 || idx >= len(execRows) {
		t.Fatalf("row index out of range: %d", idx)
	}
	row, ok := execRows[idx].(map[string]any)
	if !ok {
		t.Fatalf("execution[%d] is not object", idx)
	}
	return row
}

func loadExpectedScheduleFixture(t *testing.T) []byte {
	t.Helper()
	root := mustRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, expectedScheduleFixture))
	if err != nil {
		t.Fatalf("read expected schedule fixture: %v", err)
	}
	return raw
}

func decodeJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode JSON map failed: %v", err)
	}
	return out
}

func deepCopyJSONMap(t *testing.T, in map[string]any) map[string]any {
	t.Helper()
	raw := marshalJSON(t, in)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("deep copy decode failed: %v", err)
	}
	return out
}

func marshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return raw
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot failed: %v", err)
	}
	return root
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
