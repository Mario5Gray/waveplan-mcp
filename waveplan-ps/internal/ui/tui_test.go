package ui

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestRenderTextShowsActiveLogWithAgentAndLines(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "S3_T5.1_implement.1.stdout.log")
	if err := os.WriteFile(logFile, []byte("line one\nline two\nline three\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	journalPath := filepath.Join(dir, "schedule.json.journal.json")
	snapshot := renderTestSnapshot()
	snapshot.Journals = []watch.LoadedJournal{{
		Path: journalPath,
		Journal: &model.Journal{
			Events: []model.JournalEvent{{
				StepID:      "S3_T5.1_implement",
				TaskID:      "T5.1",
				Action:      "implement",
				StartedOn:   "2026-05-12T14:49:00Z",
				StateBefore: model.StatusWrapper{TaskStatus: model.StatusTaken},
				StateAfter:  model.StatusWrapper{TaskStatus: model.StatusTaken},
				StdoutPath:  logFile,
			}},
		},
	}}

	rendered := RenderText(snapshot, Options{ExpandFirstWave: true, LogTailLines: 3})

	for _, want := range []string{
		"Log  S3_T5.1_implement  [running]  agent:phi",
		"line one",
		"line three",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("RenderText() missing %q in:\n%s", want, rendered)
		}
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

func TestRenderLogEventStepStart(t *testing.T) {
	line := `{"type":"step_start","timestamp":1778492597000,"sessionID":"ses_1e994c983ffeXJ7d2xAotRhyeF","part":{"id":"prt_1","messageID":"msg_1","sessionID":"ses_1e994c983ffeXJ7d2xAotRhyeF","snapshot":"3f801dc51d26cee3ce5d4ea648972eb1d27a7c53","type":"step-start"}}`
	got := renderLogEvent(line)
	if len(got) != 1 {
		t.Fatalf("renderLogEvent step_start: want 1 line, got %d: %v", len(got), got)
	}
	for _, want := range []string{"20260511", "AotRhyeF", "3f801dc5", "step-start"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("renderLogEvent step_start: missing %q in %q", want, got[0])
		}
	}
}

func TestRenderLogEventText(t *testing.T) {
	line := `{"type":"text","timestamp":1778492602000,"sessionID":"ses_1e994c983ffeXJ7d2xAotRhyeF","part":{"id":"prt_2","messageID":"msg_1","sessionID":"ses_1e994c983ffeXJ7d2xAotRhyeF","type":"text","text":"Hello\nworld","time":{"start":1778492597000,"end":1778492602000}}}`
	got := renderLogEvent(line)
	// header + two text lines
	if len(got) < 2 {
		t.Fatalf("renderLogEvent text: want >=2 lines, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "5s") {
		t.Errorf("renderLogEvent text: want duration 5s in header %q", got[0])
	}
	joined := strings.Join(got[1:], "\n")
	if !strings.Contains(joined, "Hello") || !strings.Contains(joined, "world") {
		t.Errorf("renderLogEvent text: want text content in lines %v", got[1:])
	}
}

func TestRenderLogEventTextTruncates(t *testing.T) {
	long := strings.Repeat("x", 300)
	line := fmt.Sprintf(`{"type":"text","timestamp":1778492602000,"sessionID":"ses_X","part":{"type":"text","text":%q,"time":{"start":1778492597000,"end":1778492602000}}}`, long)
	got := renderLogEvent(line)
	for _, l := range got[1:] {
		if len(l) > maxTextChars+5 {
			t.Errorf("renderLogEvent text: line exceeds limit: len=%d", len(l))
		}
	}
}

func TestRenderLogEventStepFinish(t *testing.T) {
	line := `{"type":"step_finish","timestamp":1778492603000,"sessionID":"ses_1e994c983ffeXJ7d2xAotRhyeF","part":{"reason":"tool-calls","snapshot":"abc","messageID":"msg_1","sessionID":"ses_1e994c983ffeXJ7d2xAotRhyeF","type":"step-finish","tokens":{"total":19158,"input":18841,"output":317,"reasoning":0,"cache":{"write":10,"read":200}},"cost":0}}`
	got := renderLogEvent(line)
	if len(got) != 1 {
		t.Fatalf("renderLogEvent step_finish: want 1 line, got %d", len(got))
	}
	for _, want := range []string{"tot:19158", "in:18841", "out:317", "rsn:0", "cw:10", "cr:200"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("renderLogEvent step_finish: missing %q in %q", want, got[0])
		}
	}
}

func TestRenderLogEventToolUse(t *testing.T) {
	line := `{"type":"tool_use","timestamp":1778625449562,"sessionID":"ses_1e1a9b928ffe5AxoIhMCZjRG80","part":{"type":"tool","tool":"read","callID":"call_abc","state":{"status":"completed","title":"plan.json","time":{"start":1778625449544,"end":1778625449574}}}}`
	got := renderLogEvent(line)
	if len(got) != 1 {
		t.Fatalf("renderLogEvent tool_use: want 1 line, got %d: %v", len(got), got)
	}
	for _, want := range []string{"tool_use", "read", "completed", "plan.json", "→"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("renderLogEvent tool_use: missing %q in %q", want, got[0])
		}
	}
}

func TestReadRenderedLogLinesPlainText(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "plain.log")
	if err := os.WriteFile(p, []byte("a\nb\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readRenderedLogLines(p, 2)
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("readRenderedLogLines plain: want [b c], got %v", got)
	}
}

func TestReadRenderedLogLinesJSONL(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "run.stdout.log")
	content := `{"type":"step_start","timestamp":1778492597000,"sessionID":"ses_ABC","part":{"snapshot":"aabbccdd","type":"step-start"}}` + "\n" +
		`{"type":"step_finish","timestamp":1778492603000,"sessionID":"ses_ABC","part":{"tokens":{"total":100,"input":90,"output":10,"reasoning":0,"cache":{"write":0,"read":0}}}}` + "\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readRenderedLogLines(p, 20)
	if len(got) < 2 {
		t.Fatalf("readRenderedLogLines JSONL: want >=2 lines, got %v", got)
	}
	if !strings.Contains(got[0], "step-start") {
		t.Errorf("readRenderedLogLines JSONL: first line missing step-start: %q", got[0])
	}
	if !strings.Contains(got[len(got)-1], "tot:100") {
		t.Errorf("readRenderedLogLines JSONL: last line missing tot:100: %q", got[len(got)-1])
	}
}

func TestRenderTextShowsActiveLogWithJSONL(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "S3_T5.1_implement.1.stdout.log")
	jsonlContent := `{"type":"step_start","timestamp":1778492597000,"sessionID":"ses_TESTID","part":{"snapshot":"deadbeef1234","type":"step-start"}}` + "\n" +
		`{"type":"text","timestamp":1778492602000,"sessionID":"ses_TESTID","part":{"type":"text","text":"doing work","time":{"start":1778492597000,"end":1778492602000}}}` + "\n"
	if err := os.WriteFile(logFile, []byte(jsonlContent), 0o600); err != nil {
		t.Fatal(err)
	}

	journalPath := filepath.Join(dir, "schedule.json.journal.json")
	snapshot := renderTestSnapshot()
	snapshot.Journals = []watch.LoadedJournal{{
		Path: journalPath,
		Journal: &model.Journal{
			Events: []model.JournalEvent{{
				StepID:      "S3_T5.1_implement",
				TaskID:      "T5.1",
				Action:      "implement",
				StartedOn:   "2026-05-12T14:49:00Z",
				StateBefore: model.StatusWrapper{TaskStatus: model.StatusTaken},
				StateAfter:  model.StatusWrapper{TaskStatus: model.StatusTaken},
				StdoutPath:  logFile,
			}},
		},
	}}

	rendered := RenderText(snapshot, Options{ExpandFirstWave: true, LogTailLines: 10})
	if !strings.Contains(rendered, "step-start") {
		t.Errorf("RenderText with JSONL log: missing step-start in:\n%s", rendered)
	}
	if !strings.Contains(rendered, "doing work") {
		t.Errorf("RenderText with JSONL log: missing text content in:\n%s", rendered)
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
