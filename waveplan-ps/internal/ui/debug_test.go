package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/model"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/watch"
)

func TestDebugCycleLogMode(t *testing.T) {
	dir := t.TempDir()
	stdoutFile := filepath.Join(dir, "S2_T1.1_review.1.stdout.log")
	stderrFile := filepath.Join(dir, "S2_T1.1_review.1.stderr.log")
	os.WriteFile(stdoutFile, []byte("stdout line one\nstdout line two\n"), 0o600)
	os.WriteFile(stderrFile, []byte("stderr line one\nstderr line two\n"), 0o600)

	snapshot := watch.Snapshot{
		Plans: []watch.LoadedPlan{{
			Path: "2026-demo-execution-waves.json",
			Plan: &model.PlanFile{
				Plan: model.PlanMetadata{ID: "demo", Title: "Demo"},
				Units: map[string]model.Unit{
					"T1.1": {Task: "T1", Title: "Adapter mapping", Wave: 1},
				},
				Waves: []model.Wave{{Wave: 1, Units: []string{"T1.1"}}},
			},
		}},
		States: []watch.LoadedState{{
			Path: "2026-demo-execution-state.json",
			State: &model.StateFile{
				Plan: "2026-demo-execution-waves.json",
				Taken: map[string]model.TaskEntry{
					"T1.1": {TakenBy: "phi", Reviewer: "sigma"},
				},
			},
		}},
		Journals: []watch.LoadedJournal{{
			Path: "2026-demo-execution-journal.json",
			Journal: &model.Journal{
				Events: []model.JournalEvent{{
					StepID: "S2_T1.1_review", TaskID: "T1.1", Action: "review",
					Attempt: 1, StartedOn: "2026-05-12T14:54:00Z", Outcome: "applied",
					StateBefore: model.StatusWrapper{TaskStatus: model.StatusTaken},
					StateAfter:  model.StatusWrapper{TaskStatus: model.StatusReviewTaken},
					StdoutPath: stdoutFile, StderrPath: stderrFile,
				}},
			},
		}},
		Logs: []model.LogRef{
			{Path: stdoutFile, StepID: "S2_T1.1_review", Attempt: 1, Stream: model.LogStreamStdout},
			{Path: stderrFile, StepID: "S2_T1.1_review", Attempt: 1, Stream: model.LogStreamStderr},
		},
	}

	root := BuildPrimitive(snapshot, Options{LogTailLines: 2}).(*Root)
	
	fmt.Printf("BEFORE: logMode=%d, rowUnits=%v, tableRowCount=%d\n", root.logMode, root.rowUnits, root.table.GetRowCount())
	row, _ := root.table.GetSelection()
	fmt.Printf("Selection: row=%d\n", row)
	if row > 0 && row < len(root.rowUnits) {
		fmt.Printf("unitID at row=%s\n", root.rowUnits[row])
	}
	
	root.CycleLogMode()
	
	fmt.Printf("AFTER: logMode=%d\n", root.logMode)
	fmt.Printf("Status text: %q\n", root.Status().GetText(false))
}
