package swim

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
			wantErr: "unsupported schedule schema_version",
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

	inflight := deepCopyJSONMap(t, validJournal)
	events2, ok := inflight["events"].([]any)
	if !ok || len(events2) == 0 {
		t.Fatalf("invalid inflight events payload")
	}
	e1, ok := events2[0].(map[string]any)
	if !ok {
		t.Fatalf("event[0] not object")
	}
	delete(e1, "completed_on")
	delete(e1, "outcome")
	inflightRaw := marshalJSON(t, inflight)
	if err := ValidateJournal(inflightRaw); err != nil {
		t.Fatalf("ValidateJournal(inflight) unexpected error: %v", err)
	}

	badTerminalPair := deepCopyJSONMap(t, validJournal)
	events3, ok := badTerminalPair["events"].([]any)
	if !ok || len(events3) == 0 {
		t.Fatalf("invalid terminal-pair events payload")
	}
	e2, ok := events3[0].(map[string]any)
	if !ok {
		t.Fatalf("event[0] not object")
	}
	delete(e2, "completed_on")
	badTerminalPairRaw := marshalJSON(t, badTerminalPair)
	err = ValidateJournal(badTerminalPairRaw)
	if err == nil {
		t.Fatalf("ValidateJournal() expected outcome/completed_on pair violation")
	}
	if !strings.Contains(err.Error(), "schema validation failed") {
		t.Fatalf("ValidateJournal() unexpected pair violation error: %v", err)
	}
}

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

func TestValidateSchedule_FixRequiresProducesCheck(t *testing.T) {
	badRow := []ScheduleRow{
		{Seq: 1, StepID: "S1_T1.1_fix", TaskID: "T1.1", Action: "fix",
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
	    "invoke": {"argv": ["wp-plan-step.sh", "--action", "implement", "--plan", "/tmp/plan.json", "--task-id", "T1.1", "--target", "opencode", "--agent", "phi"]}
	  }]
	}`)
	if err := ValidateSchedule(raw); err != nil {
		t.Fatalf("ValidateSchedule(v3) unexpected error: %v", err)
	}
}

func TestValidateScheduleRejectsInvalidV3OperationPair(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "implement cannot be state transition",
			raw: `{"schema_version":3,"execution":[{"seq":1,"step_id":"S1_T1.1_implement","task_id":"T1.1","action":"implement","requires":{"task_status":"available"},"produces":{"task_status":"taken"},"operation":{"kind":"state_transition"},"invoke":{"argv":["wp-plan-step.sh","--action","implement","--plan","/tmp/plan.json","--task-id","T1.1"]}}]}`,
			want: "implement requires agent_dispatch",
		},
		{
			name: "finish cannot be agent dispatch",
			raw: `{"schema_version":3,"execution":[{"seq":1,"step_id":"S1_T1.1_finish","task_id":"T1.1","action":"finish","requires":{"task_status":"review_ended"},"produces":{"task_status":"completed"},"operation":{"kind":"agent_dispatch","target":"opencode","agent":"phi"},"invoke":{"argv":["wp-plan-step.sh","--action","finish","--plan","/tmp/plan.json","--task-id","T1.1","--target","opencode","--agent","phi"]}}]}`,
			want: "finish requires state_transition",
		},
		{
			name: "review requires reviewer",
			raw: `{"schema_version":3,"execution":[{"seq":1,"step_id":"S1_T1.1_review","task_id":"T1.1","action":"review","requires":{"task_status":"taken"},"produces":{"task_status":"review_taken"},"operation":{"kind":"agent_dispatch","target":"claude","agent":"phi"},"invoke":{"argv":["wp-plan-step.sh","--action","review","--plan","/tmp/plan.json","--task-id","T1.1","--target","claude","--agent","phi"]}}]}`,
			want: "review requires agent, target, and reviewer",
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

func TestValidateReviewScheduleSidecar_Cases(t *testing.T) {
	baseRaw := loadExpectedScheduleFixture(t)
	var base Schedule
	if err := json.Unmarshal(baseRaw, &base); err != nil {
		t.Fatalf("decode schedule fixture: %v", err)
	}
	if len(base.Execution) < 2 {
		t.Fatalf("expected at least two rows in base schedule fixture")
	}
	valid := validReviewScheduleSidecarFixture(t, base)
	validRaw := marshalJSON(t, valid)
	if err := ValidateReviewScheduleSidecar(validRaw, &base); err != nil {
		t.Fatalf("ValidateReviewScheduleSidecar(valid) unexpected error: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*ReviewScheduleSidecar)
		wantErr string
	}{
		{
			name: "unknown anchor",
			mutate: func(s *ReviewScheduleSidecar) {
				s.Insertions[0].AfterStepID = "S999_T9.9_review"
			},
			wantErr: "unknown after_step_id",
		},
		{
			name: "duplicate insertion id",
			mutate: func(s *ReviewScheduleSidecar) {
				s.Insertions[1].ID = s.Insertions[0].ID
			},
			wantErr: "duplicate insertion id",
		},
		{
			name: "duplicate insertion step_id against base",
			mutate: func(s *ReviewScheduleSidecar) {
				s.Insertions[0].StepID = base.Execution[0].StepID
			},
			wantErr: "duplicate step_id with base schedule",
		},
		{
			name: "anchor cycle",
			mutate: func(s *ReviewScheduleSidecar) {
				s.Insertions[0].AfterStepID = "X2"
				s.Insertions[1].AfterStepID = "X1"
			},
			wantErr: "anchor cycle detected",
		},
		{
			name: "requires produces mismatch",
			mutate: func(s *ReviewScheduleSidecar) {
				s.Insertions[0].Requires.TaskStatus = "taken"
			},
			wantErr: "requires/produces mismatch",
		},
		{
			name: "malformed argv",
			mutate: func(s *ReviewScheduleSidecar) {
				s.Insertions[1].Invoke.Argv = []string{"   "}
			},
			wantErr: "malformed argv",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := cloneReviewScheduleSidecar(valid)
			tc.mutate(&payload)
			raw := marshalJSON(t, payload)
			err := ValidateReviewScheduleSidecar(raw, &base)
			if err == nil {
				t.Fatalf("ValidateReviewScheduleSidecar() expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateReviewScheduleSidecar() error mismatch\nwant contains: %q\ngot: %v", tc.wantErr, err)
			}
		})
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

func validReviewScheduleSidecarFixture(t *testing.T, base Schedule) ReviewScheduleSidecar {
	t.Helper()
	anchor := base.Execution[1]
	return ReviewScheduleSidecar{
		SchemaVersion:    2,
		BaseSchedulePath: emitPlanPath,
		Insertions: []ReviewScheduleInsertion{
			{
				ID:            "X1",
				AfterStepID:   anchor.StepID,
				StepID:        "S900_T1.1_fix_r1",
				SeqHint:       1,
				TaskID:        "T1.1",
				Action:        "fix",
				Requires:      StatusWrapper{TaskStatus: "review_taken"},
				Produces:      StatusWrapper{TaskStatus: "taken"},
				Operation:     OperationSpec{Kind: "agent_dispatch", Target: "codex", Agent: "phi"},
				Invoke:        InvokeSpec{Argv: []string{"wp-plan-step.sh", "--action", "fix", "--plan", emitPlanPath, "--task-id", "T1.1", "--target", "codex", "--agent", "phi"}},
				Reason:        "review findings require rework",
				SourceEventID: "E0001",
			},
			{
				ID:            "X2",
				AfterStepID:   "X1",
				StepID:        "S901_T1.1_review_r2",
				SeqHint:       2,
				TaskID:        "T1.1",
				Action:        "review",
				Requires:      StatusWrapper{TaskStatus: "taken"},
				Produces:      StatusWrapper{TaskStatus: "review_taken"},
				Operation:     OperationSpec{Kind: "agent_dispatch", Target: "claude", Agent: "phi", Reviewer: "sigma"},
				Invoke:        InvokeSpec{Argv: []string{"wp-plan-step.sh", "--action", "review", "--plan", emitPlanPath, "--task-id", "T1.1", "--target", "claude", "--agent", "phi", "--reviewer", "sigma"}},
				Reason:        "follow-up review after fix",
				SourceEventID: "E0002",
			},
		},
	}
}

func cloneReviewScheduleSidecar(in ReviewScheduleSidecar) ReviewScheduleSidecar {
	out := in
	out.Insertions = append([]ReviewScheduleInsertion(nil), in.Insertions...)
	for i := range out.Insertions {
		out.Insertions[i].Invoke.Argv = append([]string(nil), in.Insertions[i].Invoke.Argv...)
	}
	return out
}

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
			if !reflect.DeepEqual(tt.want, got) {
				t.Fatalf("BuildInvokeArgv() mismatch\nwant: %v\ngot:  %v", tt.want, got)
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
