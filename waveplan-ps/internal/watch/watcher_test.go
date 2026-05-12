package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/model"
)

func TestPollOnceLoadsSnapshotFromExplicitPathsAndLogDirs(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "2026-demo-execution-waves.json")
	statePath := filepath.Join(root, "2026-demo-execution-waves.json.state.json")
	journalPath := filepath.Join(root, "2026-demo-execution-schedule.json.journal.json")
	notePath := filepath.Join(root, "notes.md")
	logDir := filepath.Join(root, "logs")
	logPath := filepath.Join(logDir, "S1_T1.1_implement.1.stdout.log")

	writeFile(t, planPath, planJSON("demo", "Initial title"))
	writeFile(t, statePath, `{"plan":"2026-demo-execution-waves.json","taken":{"T1.1":{"taken_by":"phi","started_at":"2026-05-12 14:40"}},"completed":{}}`)
	writeFile(t, journalPath, `{"schema_version":1,"schedule_path":"schedule.json","cursor":1,"events":[{"event_id":"E1","step_id":"S1_T1.1_implement","seq":1,"task_id":"T1.1","action":"implement","attempt":1,"started_on":"2026-05-12T21:40:00Z","state_before":{"task_status":"available"},"state_after":{"task_status":"taken"}}]}`)
	writeFile(t, notePath, "## T1.1 > Implementation\nLoaded note.\n")
	writeFile(t, logPath, "stdout")

	snapshot, err := PollOnce(Options{
		PlanPaths:    []string{planPath},
		StatePaths:   []string{statePath},
		JournalPaths: []string{journalPath},
		NotePaths:    []string{notePath},
		LogDirs:      []string{logDir},
	})
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}

	if len(snapshot.Plans) != 1 || snapshot.Plans[0].Path != planPath {
		t.Fatalf("Plans = %#v, want one loaded plan at %q", snapshot.Plans, planPath)
	}
	if got := snapshot.Plans[0].Plan.Plan.Title; got != "Initial title" {
		t.Fatalf("plan title = %q, want Initial title", got)
	}
	if got := snapshot.States[0].State.StatusOf("T1.1"); got != model.StatusTaken {
		t.Fatalf("state status = %q, want %q", got, model.StatusTaken)
	}
	if got := snapshot.Journals[0].Journal.Events[0].StepID; got != "S1_T1.1_implement" {
		t.Fatalf("journal step = %q", got)
	}
	if got := snapshot.Notes[0].Notes.Sections[0].Content; got != "Loaded note." {
		t.Fatalf("note content = %q", got)
	}
	if len(snapshot.Logs) != 1 || snapshot.Logs[0].Path != logPath {
		t.Fatalf("Logs = %#v, want one discovered log at %q", snapshot.Logs, logPath)
	}
	if snapshot.LoadedAt.IsZero() {
		t.Fatal("LoadedAt is zero")
	}
}

func TestWatcherRunPollsUntilContextCanceled(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "2026-demo-execution-waves.json")
	writeFile(t, planPath, planJSON("demo", "First title"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snapshots := make(chan Snapshot, 4)
	errs := make(chan error, 1)
	watcher := New(Options{PlanPaths: []string{planPath}}, 10*time.Millisecond)
	polls := 0

	go func() {
		errs <- watcher.Run(ctx, func(snapshot Snapshot) error {
			polls++
			snapshots <- snapshot
			if polls == 1 {
				writeFile(t, planPath, planJSON("demo", "Second title"))
			}
			if polls == 2 {
				cancel()
			}
			return nil
		})
	}()

	first := receiveSnapshot(t, snapshots)
	if got := first.Plans[0].Plan.Plan.Title; got != "First title" {
		t.Fatalf("first title = %q, want First title", got)
	}

	second := receiveSnapshot(t, snapshots)
	if got := second.Plans[0].Plan.Plan.Title; got != "Second title" {
		t.Fatalf("second title = %q, want Second title", got)
	}

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func receiveSnapshot(t *testing.T, snapshots <-chan Snapshot) Snapshot {
	t.Helper()
	select {
	case snapshot := <-snapshots:
		return snapshot
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for snapshot")
		return Snapshot{}
	}
}

func planJSON(id, title string) string {
	return `{
  "schema_version": 1,
  "generated_on": "2026-05-12",
  "plan": {
    "id": "` + id + `",
    "title": "` + title + `",
    "plan_doc": {"path": "docs/plan.md", "line": 1},
    "spec_doc": {"path": "docs/spec.md", "line": 1}
  },
  "fp_index": {},
  "doc_index": {},
  "tasks": {},
  "units": {}
}`
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
