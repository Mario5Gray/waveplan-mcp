package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/config"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/model"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/ui"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/watch"
	"github.com/rivo/tview"
)

func TestBuildWatchOptionsPlanFilterNarrowsPlansAndStates(t *testing.T) {
	root := t.TempDir()
	alphaPlan := filepath.Join(root, "2026-alpha-execution-waves.json")
	betaPlan := filepath.Join(root, "2026-beta-execution-waves.json")
	alphaState := alphaPlan + ".state.json"
	betaState := betaPlan + ".state.json"
	writeFile(t, alphaPlan, planJSON("alpha", "Alpha"))
	writeFile(t, betaPlan, planJSON("beta", "Beta"))
	writeFile(t, alphaState, stateJSON("T1.1", "phi"))
	writeFile(t, betaState, stateJSON("T2.1", "sigma"))

	options, err := buildWatchOptions(config.Config{
		PlanDirs:  []string{root},
		StateDirs: []string{root},
	}, cliOptions{planFilters: []string{alphaPlan}})
	if err != nil {
		t.Fatalf("buildWatchOptions() error = %v", err)
	}

	if !reflect.DeepEqual(options.PlanPaths, []string{alphaPlan}) {
		t.Fatalf("PlanPaths = %#v, want only %#v", options.PlanPaths, []string{alphaPlan})
	}
	if !reflect.DeepEqual(options.StatePaths, []string{alphaState}) {
		t.Fatalf("StatePaths = %#v, want only %#v", options.StatePaths, []string{alphaState})
	}
}

func TestOnceCommandPrintsSnapshotForSelectedPlan(t *testing.T) {
	root := t.TempDir()
	selectedPlan := filepath.Join(root, "2026-selected-execution-waves.json")
	ignoredPlan := filepath.Join(root, "2026-ignored-execution-waves.json")
	writeFile(t, selectedPlan, planJSON("selected", "Selected"))
	writeFile(t, ignoredPlan, planJSON("ignored", "Ignored"))

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--once", "--plan-dir", root, "--plan", selectedPlan})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v\n%s", err, out.String())
	}

	rendered := out.String()
	if !strings.Contains(rendered, "selected - Selected") {
		t.Fatalf("snapshot output missing selected plan:\n%s", rendered)
	}
	if strings.Contains(rendered, "ignored - Ignored") {
		t.Fatalf("snapshot output included unselected plan:\n%s", rendered)
	}
}

func TestExecuteContextErrorsWhenNoDiscoveryRootsConfigured(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("ExecuteContext() error = nil, want configuration error")
	}
	if !strings.Contains(err.Error(), "no discovery roots configured") {
		t.Fatalf("error = %v, want no discovery roots configured", err)
	}
}

func TestQueueLiveSnapshotReplacesActivePageInsideQueuedDraw(t *testing.T) {
	app := &fakeApp{}
	pages := &fakePages{}

	queueLiveSnapshot(app, pages, watch.Snapshot{
		Plans: []watch.LoadedPlan{{
			Path: "2026-demo-execution-waves.json",
			Plan: &model.PlanFile{
				Plan: model.PlanMetadata{ID: "demo", Title: "Demo"},
				Units: map[string]model.Unit{
					"T1.1": {Task: "T1", Title: "Render live page", Wave: 1},
				},
			},
		}},
	}, ui.Options{ExpandFirstWave: true})

	if app.queued != 1 {
		t.Fatalf("queued updates = %d, want 1", app.queued)
	}
	if !pages.removed {
		t.Fatal("active page was not removed before replacement")
	}
	if pages.active != livePageName {
		t.Fatalf("active page = %q, want %q", pages.active, livePageName)
	}
	root, ok := pages.item.(*ui.Root)
	if !ok {
		t.Fatalf("replacement page type = %T, want *ui.Root", pages.item)
	}
	if !strings.Contains(root.Text(), "T1.1 [available] Render live page") {
		t.Fatalf("replacement page did not render snapshot:\n%s", root.Text())
	}
}

type fakeApp struct {
	queued int
}

func (a *fakeApp) SetRoot(root tview.Primitive, fullscreen bool) appRunner {
	return a
}

func (a *fakeApp) Run() error {
	return nil
}

func (a *fakeApp) Stop() {
}

func (a *fakeApp) QueueUpdateDraw(fn func()) appRunner {
	a.queued++
	if fn != nil {
		fn()
	}
	return a
}

type fakePages struct {
	active  string
	item    tview.Primitive
	removed bool
}

func (p *fakePages) Replace(name string, item tview.Primitive) {
	p.removed = true
	p.active = name
	p.item = item
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
