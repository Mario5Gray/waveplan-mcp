package integration_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompiledBinaryOnceRendersSnapshotOutput(t *testing.T) {
	repoRoot := repositoryRoot(t)
	binaryPath := filepath.Join(t.TempDir(), "waveplan-ps")

	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/waveplan-ps")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "gocache"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/waveplan-ps error = %v\n%s", err, output)
	}

	fixtureRoot := t.TempDir()
	logDir := filepath.Join(fixtureRoot, ".waveplan", "logs")
	writeFile(t, filepath.Join(fixtureRoot, "2026-demo-execution-waves.json"), demoPlanJSON())
	writeFile(t, filepath.Join(fixtureRoot, "2026-demo-execution-waves.json.state.json"), demoStateJSON())
	writeFile(t, filepath.Join(fixtureRoot, "2026-demo-execution-schedule.json.journal.json"), demoJournalJSON())
	writeFile(t, filepath.Join(logDir, "S1_T1.1_implement.1.stdout.log"), "model ready\n")
	writeFile(t, filepath.Join(logDir, "S2_T2.1_implement.1.stderr.log"), "render retry\n")

	run := exec.Command(
		binaryPath,
		"--once",
		"--plan", filepath.Join(fixtureRoot, "2026-demo-execution-waves.json"),
		"--state", filepath.Join(fixtureRoot, "2026-demo-execution-waves.json.state.json"),
		"--journal", filepath.Join(fixtureRoot, "2026-demo-execution-schedule.json.journal.json"),
		"--log-dir", filepath.Join(fixtureRoot, ".waveplan"),
	)
	var stdout bytes.Buffer
	run.Stdout = &stdout
	run.Stderr = &stdout
	if err := run.Run(); err != nil {
		t.Fatalf("waveplan-ps --once error = %v\n%s", err, stdout.String())
	}

	rendered := stdout.String()
	for _, want := range []string{
		"Loaded:",
		"demo - Compiled binary fixture",
		"Wave 1",
		"T1.1 [completed] Bootstrap model (logs: 1)",
		"Wave 2",
		"T2.1 [taken] Render once snapshot (logs: 1)",
		"Tail",
		"T0.1 [completed] sigma",
		"Journals",
		"S2_T2.1_implement T2.1 implement taken -> taken",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("snapshot output missing %q:\n%s", want, rendered)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func demoPlanJSON() string {
	return `{
  "schema_version": 1,
  "generated_on": "2026-05-12",
  "plan": {
    "id": "demo",
    "title": "Compiled binary fixture",
    "plan_doc": {"path": "docs/plan.md", "line": 1},
    "spec_doc": {"path": "docs/spec.md", "line": 1}
  },
  "fp_index": {},
  "doc_index": {},
  "tasks": {
    "T1": {"title": "Bootstrap model", "plan_line": 1, "doc_refs": [], "files": []},
    "T2": {"title": "Render once snapshot", "plan_line": 2, "doc_refs": [], "files": []}
  },
  "units": {
    "T1.1": {"task":"T1","title":"Bootstrap model","kind":"impl","wave":1,"plan_line":1},
    "T2.1": {"task":"T2","title":"Render once snapshot","kind":"impl","wave":2,"plan_line":2,"depends_on":["T1.1"]}
  },
  "waves": [
    {"wave": 1, "units": ["T1.1"]},
    {"wave": 2, "units": ["T2.1"]}
  ]
}`
}

func demoStateJSON() string {
	return `{
  "plan": "2026-demo-execution-waves.json",
  "taken": {
    "T2.1": {"taken_by": "phi", "started_at": "2026-05-12 15:11"}
  },
  "completed": {
    "T1.1": {"taken_by": "theta", "started_at": "2026-05-12 14:00", "finished_at": "2026-05-12 14:10"}
  },
  "tail": {
    "T0.1": {"taken_by": "sigma", "started_at": "2026-05-12 13:00", "finished_at": "2026-05-12 13:10"}
  }
}`
}

func demoJournalJSON() string {
	return `{
  "schema_version": 1,
  "schedule_path": "2026-demo-execution-schedule.json",
  "cursor": 2,
  "events": [
    {
      "event_id": "E0001",
      "step_id": "S1_T1.1_implement",
      "seq": 1,
      "task_id": "T1.1",
      "action": "implement",
      "attempt": 1,
      "started_on": "2026-05-12T21:00:00Z",
      "state_before": {"task_status": "available"},
      "state_after": {"task_status": "completed"}
    },
    {
      "event_id": "E0002",
      "step_id": "S2_T2.1_implement",
      "seq": 2,
      "task_id": "T2.1",
      "action": "implement",
      "attempt": 1,
      "started_on": "2026-05-12T21:11:00Z",
      "state_before": {"task_status": "taken"},
      "state_after": {"task_status": "taken"}
    }
  ]
}`
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create parent for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
