package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/config"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/model"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/ui"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/watch"
)

func TestBuildWatchOptionsUsesExplicitPaths(t *testing.T) {
	root := t.TempDir()
	alphaPlan := filepath.Join(root, "2026-alpha-execution-waves.json")
	alphaState := alphaPlan + ".state.json"
	alphaJournal := filepath.Join(root, "2026-alpha-execution-journal.json")

	options, err := buildWatchOptions(config.Config{
		PlanPaths:    []string{alphaPlan},
		StatePaths:   []string{alphaState},
		JournalPaths: []string{alphaJournal},
	}, cliOptions{})
	if err != nil {
		t.Fatalf("buildWatchOptions() error = %v", err)
	}

	if !reflect.DeepEqual(options.PlanPaths, []string{alphaPlan}) {
		t.Fatalf("PlanPaths = %#v, want %#v", options.PlanPaths, []string{alphaPlan})
	}
	if !reflect.DeepEqual(options.StatePaths, []string{alphaState}) {
		t.Fatalf("StatePaths = %#v, want %#v", options.StatePaths, []string{alphaState})
	}
	if !reflect.DeepEqual(options.JournalPaths, []string{alphaJournal}) {
		t.Fatalf("JournalPaths = %#v, want %#v", options.JournalPaths, []string{alphaJournal})
	}
}

func TestBuildWatchOptionsFallsBackToWaveplanEnvVars(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "2026-selected-execution-waves.json")
	statePath := filepath.Join(root, "2026-selected-execution-state.json")
	journalPath := filepath.Join(root, "2026-selected-execution-journal.json")

	t.Setenv("WAVEPLAN_PLAN", planPath)
	t.Setenv("WAVEPLAN_STATE", statePath)
	t.Setenv("WAVEPLAN_JOURNAL", journalPath)

	options, err := buildWatchOptions(config.Config{}, cliOptions{})
	if err != nil {
		t.Fatalf("buildWatchOptions() error = %v", err)
	}
	if !reflect.DeepEqual(options.PlanPaths, []string{planPath}) {
		t.Fatalf("PlanPaths = %#v, want %#v", options.PlanPaths, []string{planPath})
	}
	if !reflect.DeepEqual(options.StatePaths, []string{statePath}) {
		t.Fatalf("StatePaths = %#v, want %#v", options.StatePaths, []string{statePath})
	}
	if !reflect.DeepEqual(options.JournalPaths, []string{journalPath}) {
		t.Fatalf("JournalPaths = %#v, want %#v", options.JournalPaths, []string{journalPath})
	}
}

func TestOnceCommandPrintsSnapshotForExplicitPlanPath(t *testing.T) {
	root := t.TempDir()
	selectedPlan := filepath.Join(root, "2026-selected-execution-waves.json")
	writeFile(t, selectedPlan, planJSON("selected", "Selected"))

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--once", "--plan", selectedPlan})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v\n%s", err, out.String())
	}

	rendered := out.String()
	if !strings.Contains(rendered, "selected - Selected") {
		t.Fatalf("snapshot output missing selected plan:\n%s", rendered)
	}
}

func TestExecuteContextErrorsWhenNoObserverInputsConfigured(t *testing.T) {
	t.Setenv("WAVEPLAN_PLAN", "")
	t.Setenv("WAVEPLAN_STATE", "")
	t.Setenv("WAVEPLAN_JOURNAL", "")

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("ExecuteContext() error = nil, want configuration error")
	}
	if !strings.Contains(err.Error(), "no observer inputs configured") {
		t.Fatalf("error = %v, want no observer inputs configured", err)
	}
}

func TestNewFlagSetParsesUnitLimit(t *testing.T) {
	opts := cliOptions{
		interval:        time.Second,
		tailLimit:       10,
		journalLimit:    10,
		logTailLines:    8,
		unitLimit:       10,
		expandFirstWave: true,
	}

	flags := newFlagSet(&opts, io.Discard)
	if err := flags.Parse([]string{"--unit-limit", "14"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.unitLimit != 14 {
		t.Fatalf("unitLimit = %d, want 14", opts.unitLimit)
	}
}

func TestBuildPrimitiveUpdatePreservesTableSelection(t *testing.T) {
	snap := watch.Snapshot{
		Plans: []watch.LoadedPlan{{
			Path: "2026-demo-execution-waves.json",
			Plan: &model.PlanFile{
				Plan: model.PlanMetadata{ID: "demo", Title: "Demo"},
				Units: map[string]model.Unit{
					"T1.1": {Task: "T1", Title: "First unit", Wave: 1},
					"T1.2": {Task: "T1", Title: "Second unit", Wave: 1},
					"T1.3": {Task: "T1", Title: "Third unit", Wave: 1},
				},
				Waves: []model.Wave{
					{Wave: 1, Units: []string{"T1.1", "T1.2", "T1.3"}},
				},
			},
		}},
	}

	prim := ui.BuildPrimitive(snap, ui.Options{})
	root, ok := prim.(*ui.Root)
	if !ok {
		t.Fatalf("BuildPrimitive returned %T, want *ui.Root", prim)
	}

	root.Table().Select(2, 0)
	selRow, _ := root.Table().GetSelection()
	if selRow != 2 {
		t.Fatalf("pre-update selection = %d, want 2", selRow)
	}

	root.Update(snap, ui.Options{})

	selRow, _ = root.Table().GetSelection()
	if selRow != 2 {
		t.Fatalf("post-update selection = %d, want 2 (selection was reset)", selRow)
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
  "units": {
    "T1.1": {"task":"T1","title":"Bootstrap selected plan","kind":"impl","wave":1,"plan_line":1}
  }
}`
}

func stateJSON(taskID, takenBy string) string {
	return `{"plan":"plan.json","taken":{"` + taskID + `":{"taken_by":"` + takenBy + `","started_at":"2026-05-12 15:00"}},"completed":{}}`
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
