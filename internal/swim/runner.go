package swim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ExecNextOptions controls one-step schedule execution.
type ExecNextOptions struct {
	SchedulePath       string
	ReviewSchedulePath string
	JournalPath        string
	ArtifactRoot       string
	WorkDir            string
	ExpectCursor       *int
	InvokeFn           func(argv []string, workDir string) error
}

// ExecNextResult is the outcome of executing one cursor step.
type ExecNextResult struct {
	Done     bool   `json:"done"`
	Cursor   int    `json:"cursor"`
	StepID   string `json:"step_id,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
	Action   string `json:"action,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	EventID  string `json:"event_id,omitempty"`
	ExitCode int    `json:"exit_code"`
	Journal  string `json:"journal"`
	Schedule string `json:"schedule"`
}

// ExecuteNextStep executes exactly one schedule row at journal.cursor.
// Journal acts as the state machine cursor; cursor advances only on applied.
func ExecuteNextStep(opts ExecNextOptions) (*ExecNextResult, error) {
	if opts.SchedulePath == "" {
		return nil, errors.New("missing schedule path")
	}
	if opts.JournalPath == "" {
		return nil, errors.New("missing journal path")
	}

	schedule, err := loadSchedule(opts.SchedulePath, opts.ReviewSchedulePath)
	if err != nil {
		return nil, err
	}

	journal, err := loadOrInitJournal(opts.JournalPath, opts.SchedulePath)
	if err != nil {
		return nil, err
	}

	if opts.ExpectCursor != nil && journal.Cursor != *opts.ExpectCursor {
		return nil, fmt.Errorf("cursor mismatch: expected=%d actual=%d", *opts.ExpectCursor, journal.Cursor)
	}

	if journal.Cursor < 0 || journal.Cursor > len(schedule.Execution) {
		return nil, fmt.Errorf("invalid cursor %d for execution length %d", journal.Cursor, len(schedule.Execution))
	}
	if journal.Cursor == len(schedule.Execution) {
		if err := saveJournal(opts.JournalPath, journal); err != nil {
			return nil, err
		}
		return &ExecNextResult{
			Done:     true,
			Cursor:   journal.Cursor,
			Journal:  opts.JournalPath,
			Schedule: opts.SchedulePath,
		}, nil
	}

	row := schedule.Execution[journal.Cursor]
	attempt := 1
	for _, ev := range journal.Events {
		if ev.StepID == row.StepID {
			attempt++
		}
	}
	stdoutPath, stderrPath := deriveLogPaths(opts.SchedulePath, opts.ArtifactRoot, row.StepID, attempt)
	stdoutAbsPath, stderrAbsPath := deriveLogAbsPaths(opts.SchedulePath, opts.ArtifactRoot, row.StepID, attempt)

	started := time.Now().UTC().Format(time.RFC3339)
	runErr := invokeArgv(row.Invoke.Argv, opts.WorkDir, stdoutAbsPath, stderrAbsPath, nil, opts.InvokeFn)
	completed := time.Now().UTC().Format(time.RFC3339)

	exitCode := 0
	outcome := "applied"
	if runErr != nil {
		outcome = "failed"
		exitCode = exitCodeFromErr(runErr)
	}

	stateAfter := row.Produces.TaskStatus
	if outcome != "applied" {
		stateAfter = row.Requires.TaskStatus
	}

	eventID := fmt.Sprintf("E%04d", len(journal.Events)+1)
	event := JournalEvent{
		EventID:     eventID,
		StepID:      row.StepID,
		Seq:         row.Seq,
		TaskID:      row.TaskID,
		Action:      row.Action,
		Attempt:     attempt,
		StartedOn:   started,
		CompletedOn: completed,
		Outcome:     outcome,
		StateBefore: StatusWrapper{TaskStatus: row.Requires.TaskStatus},
		StateAfter:  StatusWrapper{TaskStatus: stateAfter},
		StdoutPath:  stdoutPath,
		StderrPath:  stderrPath,
	}
	if runErr != nil {
		event.ExitCode = &exitCode
		event.Reason = runErr.Error()
	}
	journal.Events = append(journal.Events, event)
	journal.LastEvent = &event
	if outcome == "applied" {
		journal.Cursor++
	}

	if err := saveJournal(opts.JournalPath, journal); err != nil {
		return nil, err
	}

	return &ExecNextResult{
		Done:     false,
		Cursor:   journal.Cursor,
		StepID:   row.StepID,
		TaskID:   row.TaskID,
		Action:   row.Action,
		Outcome:  outcome,
		EventID:  eventID,
		ExitCode: exitCode,
		Journal:  opts.JournalPath,
		Schedule: opts.SchedulePath,
	}, nil
}

func loadOrInitJournal(journalPath, schedulePath string) (*Journal, error) {
	raw, err := os.ReadFile(journalPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read journal: %w", err)
		}
		return &Journal{
			SchemaVersion: 1,
			SchedulePath:  schedulePath,
			Cursor:        0,
			Events:        []JournalEvent{},
		}, nil
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return &Journal{
			SchemaVersion: 1,
			SchedulePath:  schedulePath,
			Cursor:        0,
			Events:        []JournalEvent{},
		}, nil
	}
	if err := ValidateJournal(raw); err != nil {
		return nil, fmt.Errorf("invalid journal: %w", err)
	}
	var journal Journal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return nil, fmt.Errorf("decode journal: %w", err)
	}
	if schedulePath != "" && journal.SchedulePath != "" && journal.SchedulePath != schedulePath {
		return nil, fmt.Errorf("journal schedule_path mismatch: journal=%q schedule=%q", journal.SchedulePath, schedulePath)
	}
	if schedulePath != "" && journal.SchedulePath == "" {
		journal.SchedulePath = schedulePath
	}
	return &journal, nil
}

func saveJournal(path string, journal *Journal) error {
	body, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal journal: %w", err)
	}
	body = append(body, '\n')
	if err := ValidateJournal(body); err != nil {
		return fmt.Errorf("journal write would violate schema: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create journal dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".swim-journal-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp journal file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp journal: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp journal: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomic rename journal: %w", err)
	}
	return nil
}

func exitCodeFromErr(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 1
}

func invokeArgv(argv []string, workDir, stdoutPath, stderrPath string, extraEnv map[string]string, fn func(argv []string, workDir string) error) error {
	stdoutFile, stderrFile, err := openLogFiles(stdoutPath, stderrPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
	}()

	if fn != nil {
		restore := applyEnv(extraEnv)
		defer restore()
		return fn(argv, workDir)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	if len(extraEnv) > 0 {
		cmd.Env = os.Environ()
		for k, v := range extraEnv {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	return cmd.Run()
}

func applyEnv(extraEnv map[string]string) func() {
	if len(extraEnv) == 0 {
		return func() {}
	}
	type envRestore struct {
		key     string
		value   string
		present bool
	}
	restore := make([]envRestore, 0, len(extraEnv))
	for k, v := range extraEnv {
		prev, ok := os.LookupEnv(k)
		restore = append(restore, envRestore{key: k, value: prev, present: ok})
		_ = os.Setenv(k, v)
	}
	return func() {
		for i := len(restore) - 1; i >= 0; i-- {
			r := restore[i]
			if r.present {
				_ = os.Setenv(r.key, r.value)
			} else {
				_ = os.Unsetenv(r.key)
			}
		}
	}
}

func openLogFiles(stdoutAbs, stderrAbs string) (*os.File, *os.File, error) {
	if err := os.MkdirAll(filepath.Dir(stdoutAbs), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create stdout log dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(stderrAbs), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create stderr log dir: %w", err)
	}
	stdoutFile, err := os.OpenFile(stdoutAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open stdout log: %w", err)
	}
	stderrFile, err := os.OpenFile(stderrAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = stdoutFile.Close()
		return nil, nil, fmt.Errorf("open stderr log: %w", err)
	}
	return stdoutFile, stderrFile, nil
}
