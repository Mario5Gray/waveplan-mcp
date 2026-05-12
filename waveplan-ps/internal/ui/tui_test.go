package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/model"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/watch"
)

func TestLogsForUnitMatchesTaskIDSegmentExactly(t *testing.T) {
	logs := []model.LogRef{
		{Path: "logs/S3_T5.1_implement.1.stdout.log", StepID: "S3_T5.1_implement", Attempt: 1, Stream: model.LogStreamStdout},
		{Path: "logs/S3_T5.10_implement.1.stdout.log", StepID: "S3_T5.10_implement", Attempt: 1, Stream: model.LogStreamStdout},
		{Path: "logs/S3_XT5.1_implement.1.stderr.log", StepID: "S3_XT5.1_implement", Attempt: 1, Stream: model.LogStreamStderr},
		{Path: "logs/S3_T5.1_review_r2.2.stderr.log", StepID: "S3_T5.1_review_r2", Attempt: 2, Stream: model.LogStreamStderr},
	}

	matched := LogsForUnit(logs, "T5.1")

	if len(matched) != 2 {
		t.Fatalf("len(LogsForUnit()) = %d, want 2: %#v", len(matched), matched)
	}
	if matched[0].StepID != "S3_T5.1_implement" || matched[1].StepID != "S3_T5.1_review_r2" {
		t.Fatalf("matched step IDs = %#v", matched)
	}
}

func TestRenderTextIncludesWaveStatusTailAndExactLogCounts(t *testing.T) {
	snapshot := renderTestSnapshot()

	rendered := RenderText(snapshot, Options{ExpandFirstWave: true})

	for _, want := range []string{
		"waveplan-ps - observer",
		"Loaded: 2026-05-12 15:04:05",
		"Wave 1",
		"T1.1 [completed] Bootstrap model (logs: 1)",
		"Wave 3",
		"T5.1 [taken] Render UI (logs: 1)",
		"Tail",
		"T0.1 [completed] sigma",
		"Journals",
		"S3_T5.1_implement T5.1 implement taken -> taken",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("RenderText() missing %q in:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "logs: 2") {
		t.Fatalf("RenderText() counted substring log match:\n%s", rendered)
	}
}

func TestBuildPrimitiveRendersPlanTable(t *testing.T) {
	primitive := BuildPrimitive(renderTestSnapshot(), Options{ExpandFirstWave: true})
	if primitive == nil {
		t.Fatal("BuildPrimitive() = nil")
	}

	rendered := primitive.(*Root).Text()
	if !strings.Contains(rendered, "T5.1 [taken] Render UI (logs: 1)") {
		t.Fatalf("Root text missing T5.1 row:\n%s", rendered)
	}
}

func renderTestSnapshot() watch.Snapshot {
	loadedAt := time.Date(2026, 5, 12, 15, 4, 5, 0, time.UTC)
	return watch.Snapshot{
		Plans: []watch.LoadedPlan{{
			Path: "2026-waveplan-ps-execution-waves.json",
			Plan: &model.PlanFile{
				Plan: model.PlanMetadata{ID: "waveplan-ps", Title: "observer"},
				Units: map[string]model.Unit{
					"T1.1": {Task: "T1", Title: "Bootstrap model", Wave: 1},
					"T5.1": {Task: "T5", Title: "Render UI", Wave: 3},
				},
				Waves: []model.Wave{
					{Wave: 1, Units: []string{"T1.1"}},
					{Wave: 3, Units: []string{"T5.1"}},
				},
			},
		}},
		States: []watch.LoadedState{{
			Path: "2026-waveplan-ps-execution-waves.json.state.json",
			State: &model.StateFile{
				Taken: map[string]model.TaskEntry{
					"T5.1": {TakenBy: "phi", StartedAt: "2026-05-12 14:49"},
				},
				Completed: map[string]model.TaskEntry{
					"T1.1": {TakenBy: "alpha", FinishedAt: "2026-05-12 13:00"},
				},
				Tail: map[string]model.TaskEntry{
					"T0.1": {TakenBy: "sigma", FinishedAt: "2026-05-12 12:00"},
				},
			},
		}},
		Journals: []watch.LoadedJournal{{
			Path: "2026-waveplan-ps-execution-schedule.json.journal.json",
			Journal: &model.Journal{
				Events: []model.JournalEvent{{
					StepID:      "S3_T5.1_implement",
					TaskID:      "T5.1",
					Action:      "implement",
					StateBefore: model.StatusWrapper{TaskStatus: model.StatusTaken},
					StateAfter:  model.StatusWrapper{TaskStatus: model.StatusTaken},
				}},
			},
		}},
		Logs: []model.LogRef{
			{Path: "logs/S1_T1.1_implement.1.stdout.log", StepID: "S1_T1.1_implement", Attempt: 1, Stream: model.LogStreamStdout},
			{Path: "logs/S3_T5.1_implement.1.stdout.log", StepID: "S3_T5.1_implement", Attempt: 1, Stream: model.LogStreamStdout},
			{Path: "logs/S3_T5.10_implement.1.stdout.log", StepID: "S3_T5.10_implement", Attempt: 1, Stream: model.LogStreamStdout},
		},
		LoadedAt: loadedAt,
	}
}
