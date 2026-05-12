package discovery

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/model"
)

func TestDiscoverRecursivelyReturnsDeterministicFileSets(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "b", "notes", "review.md"), "## Review\nok\n")
	writeFile(t, filepath.Join(root, "a", "2026-demo-execution-waves.json"), "{}")
	writeFile(t, filepath.Join(root, "b", "2026-demo-execution-waves.json.state.json"), "{}")
	writeFile(t, filepath.Join(root, "a", "2026-demo-execution-schedule.json.journal.json"), "{}")
	writeFile(t, filepath.Join(root, "a", "ignore-execution-schedule.json"), "{}")
	writeFile(t, filepath.Join(root, "b", "ignore.json"), "{}")

	inventory, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	wantPlans := []string{filepath.Join(root, "a", "2026-demo-execution-waves.json")}
	if !reflect.DeepEqual(inventory.PlanPaths, wantPlans) {
		t.Fatalf("PlanPaths = %#v, want %#v", inventory.PlanPaths, wantPlans)
	}

	wantStates := []string{filepath.Join(root, "b", "2026-demo-execution-waves.json.state.json")}
	if !reflect.DeepEqual(inventory.StatePaths, wantStates) {
		t.Fatalf("StatePaths = %#v, want %#v", inventory.StatePaths, wantStates)
	}

	wantJournals := []string{filepath.Join(root, "a", "2026-demo-execution-schedule.json.journal.json")}
	if !reflect.DeepEqual(inventory.JournalPaths, wantJournals) {
		t.Fatalf("JournalPaths = %#v, want %#v", inventory.JournalPaths, wantJournals)
	}

	wantNotes := []string{filepath.Join(root, "b", "notes", "review.md")}
	if !reflect.DeepEqual(inventory.NotePaths, wantNotes) {
		t.Fatalf("NotePaths = %#v, want %#v", inventory.NotePaths, wantNotes)
	}
}

func TestDiscoverLogsParsesAndSortsValidLogFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "z", "S2_T2.1_implement.1.stderr.log"), "err")
	writeFile(t, filepath.Join(root, "a", "S1_T1.1_review.2.stdout.log"), "out")
	writeFile(t, filepath.Join(root, "a", "receipt.dispatch.json"), "{}")

	logs, err := DiscoverLogs(root)
	if err != nil {
		t.Fatalf("DiscoverLogs() error = %v", err)
	}

	want := []model.LogRef{
		{
			Path:    filepath.Join(root, "a", "S1_T1.1_review.2.stdout.log"),
			StepID:  "S1_T1.1_review",
			Attempt: 2,
			Stream:  model.LogStreamStdout,
		},
		{
			Path:    filepath.Join(root, "z", "S2_T2.1_implement.1.stderr.log"),
			StepID:  "S2_T2.1_implement",
			Attempt: 1,
			Stream:  model.LogStreamStderr,
		},
	}
	if !reflect.DeepEqual(logs, want) {
		t.Fatalf("DiscoverLogs() = %#v, want %#v", logs, want)
	}
}

func TestDiscoverLogsRejectsMalformedLogNames(t *testing.T) {
	root := t.TempDir()
	badPath := filepath.Join(root, "S1_T1.1_implement.stdout.log")
	writeFile(t, badPath, "missing attempt")

	_, err := DiscoverLogs(root)
	if err == nil {
		t.Fatal("DiscoverLogs() error = nil, want malformed log error")
	}
	if !strings.Contains(err.Error(), badPath) {
		t.Fatalf("DiscoverLogs() error = %q, want path %q", err, badPath)
	}
}

func TestDiscoverAllIgnoresMalformedLogLikeFile(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "2026-demo-execution-waves.json")
	badLogPath := filepath.Join(root, "S1_T1.1_implement.stdout.log")
	writeFile(t, planPath, "{}")
	writeFile(t, badLogPath, "missing attempt")

	inventory, err := DiscoverAll([]string{root})
	if err != nil {
		t.Fatalf("DiscoverAll() error = %v", err)
	}

	wantPlans := []string{planPath}
	if !reflect.DeepEqual(inventory.PlanPaths, wantPlans) {
		t.Fatalf("PlanPaths = %#v, want %#v", inventory.PlanPaths, wantPlans)
	}
	if len(inventory.Logs) != 0 {
		t.Fatalf("Logs = %#v, want empty", inventory.Logs)
	}
}

func TestDiscoverAllCombinesMultipleRootsDeterministically(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "zzz-root")
	second := filepath.Join(parent, "aaa-root")
	firstPath := filepath.Join(first, "zzz-execution-waves.json")
	secondPath := filepath.Join(second, "aaa-execution-waves.json")
	writeFile(t, secondPath, "{}")
	writeFile(t, firstPath, "{}")

	inventory, err := DiscoverAll([]string{second, first})
	if err != nil {
		t.Fatalf("DiscoverAll() error = %v", err)
	}

	want := []string{
		secondPath,
		firstPath,
	}
	if !reflect.DeepEqual(inventory.PlanPaths, want) {
		t.Fatalf("PlanPaths = %#v, want %#v", inventory.PlanPaths, want)
	}
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
