package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPlanDecodesExecutionWavesPlan(t *testing.T) {
	path := writeTempFile(t, "plan.json", `{
  "schema_version": 1,
  "generated_on": "2026-05-12",
  "plan": {
    "id": "waveplan-ps",
    "title": "observer",
    "plan_doc": {"path": "docs/plan.md", "line": 1},
    "spec_doc": {"path": "docs/spec.md", "line": 2}
  },
  "fp_index": {"FP-WPPS-T1": "FP-WPPS-T1"},
  "doc_index": {"plan_md": {"path": "docs/plan.md", "line": 1, "kind": "plan"}},
  "tasks": {
    "T1": {
      "title": "Add Go dependencies and domain model loaders",
      "plan_line": 62,
      "doc_refs": ["plan_md"],
      "files": ["go.mod", "internal/model/plan.go"]
    }
  },
  "units": {
    "T1.1": {
      "task": "T1",
      "title": "Add Go dependencies and domain model loaders",
      "kind": "impl",
      "wave": 1,
      "plan_line": 62,
      "depends_on": [],
      "doc_refs": ["plan_md"],
      "fp_refs": ["FP-WPPS-T1"],
      "notes": ["root module only; do not create nested go.mod"],
      "command": "go test ./..."
    }
  },
  "waves": [{"wave": 1, "units": ["T1.1"]}]
}`)

	plan, err := LoadPlan(path)
	if err != nil {
		t.Fatalf("LoadPlan() error = %v", err)
	}

	if plan.Plan.ID != "waveplan-ps" {
		t.Fatalf("plan ID = %q, want waveplan-ps", plan.Plan.ID)
	}
	if plan.Tasks["T1"].Files[1] != "internal/model/plan.go" {
		t.Fatalf("task files = %#v", plan.Tasks["T1"].Files)
	}
	if got := plan.Units["T1.1"].Notes[0]; got != "root module only; do not create nested go.mod" {
		t.Fatalf("unit note = %q", got)
	}
	if got := plan.Waves[0].Units[0]; got != "T1.1" {
		t.Fatalf("wave unit = %q", got)
	}
}

func TestLoadStateDecodesLifecycleMaps(t *testing.T) {
	path := writeTempFile(t, "state.json", `{
  "plan": "waveplan-ps-execution-waves.json",
  "taken": {
    "T1.1": {
      "taken_by": "phi",
      "started_at": "2026-05-12 12:25",
      "review_entered_at": "",
      "review_ended_at": "",
      "reviewer": "",
      "git_sha": "df46da6",
      "finished_at": ""
    }
  },
  "completed": {},
  "tail": {
    "T0.1": {
      "taken_by": "sigma",
      "started_at": "2026-05-12 12:00",
      "review_entered_at": "2026-05-12 12:05",
      "review_ended_at": "2026-05-12 12:10",
      "reviewer": "theta",
      "review_note": "ok",
      "git_sha": "abc123",
      "finished_at": "2026-05-12 12:11"
    }
  }
}`)

	state, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if got := state.Taken["T1.1"].TakenBy; got != "phi" {
		t.Fatalf("taken_by = %q, want phi", got)
	}
	if got := state.StatusOf("T1.1"); got != StatusTaken {
		t.Fatalf("StatusOf(T1.1) = %q, want %q", got, StatusTaken)
	}
	if got := state.StatusOf("T9.9"); got != StatusAvailable {
		t.Fatalf("StatusOf(T9.9) = %q, want %q", got, StatusAvailable)
	}
}

func TestLoadJournalDecodesEvents(t *testing.T) {
	path := writeTempFile(t, "journal.json", `{
  "schema_version": 1,
  "schedule_path": "/tmp/schedule.json",
  "cursor": 1,
  "last_event": {
    "event_id": "E0001",
    "step_id": "S1_T1.1_implement",
    "seq": 1,
    "task_id": "T1.1",
    "action": "implement",
    "attempt": 1,
    "started_on": "2026-05-12T19:25:44Z",
    "state_before": {"task_status": "available"},
    "state_after": {"task_status": "taken"},
    "stdout_path": ".waveplan/logs/S1_T1.1_implement.1.stdout.log",
    "stderr_path": ".waveplan/logs/S1_T1.1_implement.1.stderr.log"
  },
  "events": [
    {
      "event_id": "E0001",
      "step_id": "S1_T1.1_implement",
      "seq": 1,
      "task_id": "T1.1",
      "action": "implement",
      "attempt": 1,
      "started_on": "2026-05-12T19:25:44Z",
      "state_before": {"task_status": "available"},
      "state_after": {"task_status": "taken"},
      "stdout_path": ".waveplan/logs/S1_T1.1_implement.1.stdout.log",
      "stderr_path": ".waveplan/logs/S1_T1.1_implement.1.stderr.log"
    }
  ]
}`)

	journal, err := LoadJournal(path)
	if err != nil {
		t.Fatalf("LoadJournal() error = %v", err)
	}

	if journal.LastEvent == nil {
		t.Fatal("LastEvent is nil")
	}
	if got := journal.LastEvent.StateAfter.TaskStatus; got != StatusTaken {
		t.Fatalf("last state after = %q, want %q", got, StatusTaken)
	}
	if got := journal.Events[0].StdoutPath; got != ".waveplan/logs/S1_T1.1_implement.1.stdout.log" {
		t.Fatalf("stdout path = %q", got)
	}
}

func TestLoadReviewScheduleDecodesInsertions(t *testing.T) {
	path := writeTempFile(t, "review-schedule.json", `{
  "schema_version": 1,
  "base_schedule_path": "/tmp/schedule.json",
  "insertions": [
    {
      "id": "X1",
      "after_step_id": "S1_T1.1_review",
      "step_id": "S1_T1.1_fix_r1",
      "seq_hint": 1,
      "task_id": "T1.1",
      "action": "fix",
      "requires": {"task_status": "review_taken"},
      "produces": {"task_status": "taken"},
      "reason": "review finding requires fix",
      "source_event_id": "E0001"
    }
  ]
}`)

	reviewSchedule, err := LoadReviewSchedule(path)
	if err != nil {
		t.Fatalf("LoadReviewSchedule() error = %v", err)
	}

	if got := reviewSchedule.BaseSchedulePath; got != "/tmp/schedule.json" {
		t.Fatalf("base schedule path = %q, want /tmp/schedule.json", got)
	}
	if got := reviewSchedule.Insertions[0].StepID; got != "S1_T1.1_fix_r1" {
		t.Fatalf("insertion step = %q, want S1_T1.1_fix_r1", got)
	}
	if got := reviewSchedule.Insertions[0].SourceEventID; got != "E0001" {
		t.Fatalf("source_event_id = %q, want E0001", got)
	}
}

func TestParseLogPathExtractsStepAttemptAndStream(t *testing.T) {
	logRef, err := ParseLogPath("/tmp/S12_T2.1_review_r2.3.stderr.log")
	if err != nil {
		t.Fatalf("ParseLogPath() error = %v", err)
	}

	if logRef.StepID != "S12_T2.1_review_r2" {
		t.Fatalf("StepID = %q", logRef.StepID)
	}
	if logRef.Attempt != 3 {
		t.Fatalf("Attempt = %d", logRef.Attempt)
	}
	if logRef.Stream != LogStreamStderr {
		t.Fatalf("Stream = %q", logRef.Stream)
	}
}

func TestParseLogPathExtractsStepWithNumericSegments(t *testing.T) {
	logRef, err := ParseLogPath("/tmp/S2_T2.1_fix_r1.2.stdout.log")
	if err != nil {
		t.Fatalf("ParseLogPath() error = %v", err)
	}

	if logRef.StepID != "S2_T2.1_fix_r1" {
		t.Fatalf("StepID = %q", logRef.StepID)
	}
	if logRef.Attempt != 2 {
		t.Fatalf("Attempt = %d", logRef.Attempt)
	}
	if logRef.Stream != LogStreamStdout {
		t.Fatalf("Stream = %q", logRef.Stream)
	}
}

func TestParseLogPathRejectsAmbiguousNumericDotSegment(t *testing.T) {
	_, err := ParseLogPath("/tmp/S1_T1.1_implement.1.2.stdout.log")
	if err == nil {
		t.Fatal("ParseLogPath() error = nil, want invalid log filename error")
	}
}

func TestLoadNotesParsesTxtstoreSections(t *testing.T) {
	path := writeTempFile(t, "notes.md", `<!-- INDEX -->
- [OAuth](#oauth)
<!-- /INDEX -->

## T1 > Implementation > OAuth
First line.
Second line.

## Review
Looks good.
`)

	notes, err := LoadNotes(path)
	if err != nil {
		t.Fatalf("LoadNotes() error = %v", err)
	}

	if len(notes.Sections) != 2 {
		t.Fatalf("len(Sections) = %d, want 2", len(notes.Sections))
	}
	first := notes.Sections[0]
	if first.Unit != "T1" || first.Section != "Implementation" || first.Title != "OAuth" {
		t.Fatalf("first section = %#v", first)
	}
	if first.Content != "First line.\nSecond line." {
		t.Fatalf("content = %q", first.Content)
	}
}

func writeTempFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
