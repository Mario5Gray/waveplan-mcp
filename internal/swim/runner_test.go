package swim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"reflect"
	"testing"
)

func TestExecuteNextStep_SuccessAdvancesCursor(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_implement",
			TaskID:   "T1.1",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
		},
	})

	res, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath})
	if err != nil {
		t.Fatalf("ExecuteNextStep: %v", err)
	}
	if res.Outcome != "applied" {
		t.Fatalf("outcome = %q, want applied", res.Outcome)
	}
	if res.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", res.Cursor)
	}

	j := readJournal(t, journalPath)
	if j.Cursor != 1 {
		t.Fatalf("journal cursor = %d, want 1", j.Cursor)
	}
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	if j.Events[0].Outcome != "applied" {
		t.Fatalf("event outcome = %q, want applied", j.Events[0].Outcome)
	}
}

func TestExecuteNextStep_FailureKeepsCursorAndAppendsEvent(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_implement",
			TaskID:   "T1.1",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "exit 7"}},
		},
	})

	res, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath})
	if err != nil {
		t.Fatalf("ExecuteNextStep: %v", err)
	}
	if res.Outcome != "failed" {
		t.Fatalf("outcome = %q, want failed", res.Outcome)
	}
	if res.ExitCode != 7 {
		t.Fatalf("exit_code = %d, want 7", res.ExitCode)
	}
	if res.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", res.Cursor)
	}

	j := readJournal(t, journalPath)
	if j.Cursor != 0 {
		t.Fatalf("journal cursor = %d, want 0", j.Cursor)
	}
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	if j.Events[0].Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", j.Events[0].Attempt)
	}

	res2, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath})
	if err != nil {
		t.Fatalf("second ExecuteNextStep: %v", err)
	}
	if res2.Outcome != "failed" {
		t.Fatalf("second outcome = %q, want failed", res2.Outcome)
	}
	j = readJournal(t, journalPath)
	if len(j.Events) != 2 {
		t.Fatalf("events len after retry = %d, want 2", len(j.Events))
	}
	if j.Events[1].Attempt != 2 {
		t.Fatalf("attempt after retry = %d, want 2", j.Events[1].Attempt)
	}
}

func TestExecuteNextStep_UsesDerivedInvokeArgvForV3Rows(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")

	writeScheduleVersion(t, schedulePath, 3, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_review",
			TaskID:   "T1.1",
			Action:   "review",
			Requires: StatusWrapper{TaskStatus: string(StatusTaken)},
			Produces: StatusWrapper{TaskStatus: string(StatusReviewTaken)},
			Operation: OperationSpec{
				Kind:     "agent_dispatch",
				Target:   "claude",
				Agent:    "phi",
				Reviewer: "theta",
			},
			Invoke: InvokeSpec{Argv: []string{"wp-plan-step.sh", "--action", "review", "--plan", "/tmp/stale-plan.json", "--task-id", "T1.1", "--target", "claude", "--agent", "phi", "--reviewer", "theta"}},
		},
	})

	var got []string
	res, err := ExecuteNextStep(ExecNextOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		InvokeFn: func(argv []string, workDir string) error {
			got = append([]string(nil), argv...)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("ExecuteNextStep: %v", err)
	}
	if res.Outcome != "applied" {
		t.Fatalf("outcome = %q, want applied", res.Outcome)
	}
	want := []string{"wp-plan-step.sh", "--action", "review", "--plan", schedulePath, "--task-id", "T1.1", "--target", "claude", "--agent", "phi", "--reviewer", "theta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("derived argv mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestExecuteNextStep_DoneWhenCursorAtEnd(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_implement",
			TaskID:   "T1.1",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
		},
	})

	writeJournal(t, journalPath, Journal{SchemaVersion: 1, SchedulePath: schedulePath, Cursor: 1, Events: []JournalEvent{}})

	res, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath})
	if err != nil {
		t.Fatalf("ExecuteNextStep: %v", err)
	}
	if !res.Done {
		t.Fatalf("Done = false, want true")
	}
	if res.Cursor != 1 {
		t.Fatalf("cursor = %d, want 1", res.Cursor)
	}
}

func TestExecuteNextStep_ExpectCursorMismatch(t *testing.T) {
	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "schedule.json")
	journalPath := filepath.Join(dir, "journal.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_implement",
			TaskID:   "T1.1",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"bash", "-lc", "true"}},
		},
	})
	writeJournal(t, journalPath, Journal{SchemaVersion: 1, SchedulePath: schedulePath, Cursor: 0, Events: []JournalEvent{}})

	expected := 1
	if _, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath, ExpectCursor: &expected}); err == nil {
		t.Fatalf("expected cursor mismatch error")
	}
}

func TestExecuteNextStep_WritesStdoutAndStderrLogs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX argv fixture")
	}

	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "sample-plan.json")
	journalPath := filepath.Join(dir, "journal.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_implement",
			TaskID:   "T1.1",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"/bin/sh", "-c", "printf 'hello\\n'; printf 'warn\\n' >&2"}},
		},
	})

	if _, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath}); err != nil {
		t.Fatalf("ExecuteNextStep: %v", err)
	}

	j := readJournal(t, journalPath)
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	event := j.Events[0]
	if event.StdoutPath == "" {
		t.Fatal("stdout_path = empty, want populated")
	}
	if event.StderrPath == "" {
		t.Fatal("stderr_path = empty, want populated")
	}
	wantStdoutPath := filepath.Join(".waveplan", "swim", "sample-plan", "logs", "S1_T1.1_implement.1.stdout.log")
	wantStderrPath := filepath.Join(".waveplan", "swim", "sample-plan", "logs", "S1_T1.1_implement.1.stderr.log")
	if event.StdoutPath != wantStdoutPath {
		t.Fatalf("stdout_path = %q, want %q", event.StdoutPath, wantStdoutPath)
	}
	if event.StderrPath != wantStderrPath {
		t.Fatalf("stderr_path = %q, want %q", event.StderrPath, wantStderrPath)
	}

	stdoutBody, err := os.ReadFile(filepath.Join(dir, event.StdoutPath))
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	if string(stdoutBody) != "hello\n" {
		t.Fatalf("stdout log = %q, want %q", string(stdoutBody), "hello\n")
	}

	stderrBody, err := os.ReadFile(filepath.Join(dir, event.StderrPath))
	if err != nil {
		t.Fatalf("read stderr log: %v", err)
	}
	if string(stderrBody) != "warn\n" {
		t.Fatalf("stderr log = %q, want %q", string(stderrBody), "warn\n")
	}
}

func TestExecuteNextStep_WritesLogsUnderExplicitArtifactRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX argv fixture")
	}

	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "sample-plan.json")
	journalPath := filepath.Join(dir, "journal.json")
	artifactRoot := filepath.Join(dir, "artifacts")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_implement",
			TaskID:   "T1.1",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"/bin/sh", "-c", "printf 'hello\\n'; printf 'warn\\n' >&2"}},
		},
	})

	if _, err := ExecuteNextStep(ExecNextOptions{
		SchedulePath: schedulePath,
		JournalPath:  journalPath,
		ArtifactRoot: artifactRoot,
	}); err != nil {
		t.Fatalf("ExecuteNextStep: %v", err)
	}

	j := readJournal(t, journalPath)
	if len(j.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(j.Events))
	}
	event := j.Events[0]
	wantStdoutPath := filepath.Join(artifactRoot, "logs", "S1_T1.1_implement.1.stdout.log")
	wantStderrPath := filepath.Join(artifactRoot, "logs", "S1_T1.1_implement.1.stderr.log")
	if event.StdoutPath != wantStdoutPath {
		t.Fatalf("stdout_path = %q, want %q", event.StdoutPath, wantStdoutPath)
	}
	if event.StderrPath != wantStderrPath {
		t.Fatalf("stderr_path = %q, want %q", event.StderrPath, wantStderrPath)
	}

	stdoutBody, err := os.ReadFile(event.StdoutPath)
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	if string(stdoutBody) != "hello\n" {
		t.Fatalf("stdout log = %q, want %q", string(stdoutBody), "hello\n")
	}

	stderrBody, err := os.ReadFile(event.StderrPath)
	if err != nil {
		t.Fatalf("read stderr log: %v", err)
	}
	if string(stderrBody) != "warn\n" {
		t.Fatalf("stderr log = %q, want %q", string(stderrBody), "warn\n")
	}
}

func TestExecuteNextStep_FailureWritesLogPathsAndExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX argv fixture")
	}

	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "sample-plan.json")
	journalPath := filepath.Join(dir, "journal.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_implement",
			TaskID:   "T1.1",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"/bin/sh", "-c", "printf 'boom\\n' >&2; exit 7"}},
		},
	})

	res, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath})
	if err != nil {
		t.Fatalf("ExecuteNextStep: %v", err)
	}
	if res.ExitCode != 7 {
		t.Fatalf("exit_code = %d, want 7", res.ExitCode)
	}

	j := readJournal(t, journalPath)
	event := j.Events[0]
	if event.StdoutPath == "" || event.StderrPath == "" {
		t.Fatalf("expected log paths, got stdout=%q stderr=%q", event.StdoutPath, event.StderrPath)
	}
	if event.ExitCode == nil || *event.ExitCode != 7 {
		t.Fatalf("event exit_code = %v, want 7", event.ExitCode)
	}

	stderrBody, err := os.ReadFile(filepath.Join(dir, event.StderrPath))
	if err != nil {
		t.Fatalf("read stderr log: %v", err)
	}
	if string(stderrBody) != "boom\n" {
		t.Fatalf("stderr log = %q, want %q", string(stderrBody), "boom\n")
	}
}

func TestExecuteNextStep_RetryUsesAttemptInLogFileNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX argv fixture")
	}

	dir := t.TempDir()
	schedulePath := filepath.Join(dir, "sample-plan.json")
	journalPath := filepath.Join(dir, "journal.json")

	writeSchedule(t, schedulePath, []ScheduleRow{
		{
			Seq:      1,
			StepID:   "S1_T1.1_implement",
			TaskID:   "T1.1",
			Action:   "implement",
			Requires: StatusWrapper{TaskStatus: string(StatusAvailable)},
			Produces: StatusWrapper{TaskStatus: string(StatusTaken)},
			Invoke:   InvokeSpec{Argv: []string{"/bin/false"}},
		},
	})

	if _, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath}); err != nil {
		t.Fatalf("first ExecuteNextStep: %v", err)
	}
	if _, err := ExecuteNextStep(ExecNextOptions{SchedulePath: schedulePath, JournalPath: journalPath}); err != nil {
		t.Fatalf("second ExecuteNextStep: %v", err)
	}

	j := readJournal(t, journalPath)
	if len(j.Events) != 2 {
		t.Fatalf("events len = %d, want 2", len(j.Events))
	}
	if got := j.Events[0].StdoutPath; got != filepath.Join(".waveplan", "swim", "sample-plan", "logs", "S1_T1.1_implement.1.stdout.log") {
		t.Fatalf("first stdout_path = %q", got)
	}
	if got := j.Events[1].StdoutPath; got != filepath.Join(".waveplan", "swim", "sample-plan", "logs", "S1_T1.1_implement.2.stdout.log") {
		t.Fatalf("second stdout_path = %q", got)
	}
}

func writeSchedule(t *testing.T, path string, rows []ScheduleRow) {
	t.Helper()
	writeScheduleVersion(t, path, 2, rows)
}

func writeScheduleVersion(t *testing.T, path string, version int, rows []ScheduleRow) {
	t.Helper()
	body, err := json.MarshalIndent(map[string]any{
		"schema_version": version,
		"execution":      rows,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal schedule: %v", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write schedule: %v", err)
	}
}

func writeJournal(t *testing.T, path string, j Journal) {
	t.Helper()
	body, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		t.Fatalf("marshal journal: %v", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write journal: %v", err)
	}
}

func readJournal(t *testing.T, path string) Journal {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	var j Journal
	if err := json.Unmarshal(body, &j); err != nil {
		t.Fatalf("decode journal: %v", err)
	}
	return j
}
