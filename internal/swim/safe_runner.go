package swim

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// SafeExecOptions wraps one-step execution with lock ownership and
// snapshot-based race closure.
type SafeExecOptions struct {
	SchedulePath   string
	JournalPath    string
	StatePath      string
	LockPath       string
	WorkDir        string
	ExpectCursor   *int
	InvokeFn       func(argv []string, workDir string) error
	ReadSnapshotFn func(path string) (*StateSnapshot, error)
}

// ExecuteNextStepSafe runs one schedule step under the T2.5 A/B/C protocol.
func ExecuteNextStepSafe(opts SafeExecOptions) (*ExecNextResult, error) {
	if opts.SchedulePath == "" {
		return nil, fmt.Errorf("missing schedule path")
	}
	if opts.JournalPath == "" {
		return nil, fmt.Errorf("missing journal path")
	}
	if opts.StatePath == "" {
		return nil, fmt.Errorf("missing state path")
	}

	lockPath := opts.LockPath
	if lockPath == "" {
		lockPath = DeriveLockPath(opts.SchedulePath)
	}
	lock, err := AcquireLock(lockPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = lock.Release()
	}()

	if _, err := DetectAndMarkUnknown(opts.JournalPath); err != nil {
		return nil, err
	}

	schedule, err := loadSchedule(opts.SchedulePath)
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

	readSnapshot := opts.ReadSnapshotFn
	if readSnapshot == nil {
		readSnapshot = ReadStateSnapshot
	}

	snapA, err := readSnapshot(opts.StatePath)
	if err != nil {
		return nil, err
	}

	decision := ResolveNext(schedule, journal, snapA)
	switch decision.Action {
	case ActionDone:
		if err := saveJournal(opts.JournalPath, journal); err != nil {
			return nil, err
		}
		return &ExecNextResult{
			Done:     true,
			Cursor:   journal.Cursor,
			Journal:  opts.JournalPath,
			Schedule: opts.SchedulePath,
		}, nil
	case ActionReady:
		// Continue.
	case ActionBlocked, ActionDrift:
		if decision.Row.StepID == "" {
			return &ExecNextResult{
				Done:     false,
				Cursor:   journal.Cursor,
				Outcome:  "blocked",
				Journal:  opts.JournalPath,
				Schedule: opts.SchedulePath,
			}, nil
		}
		return appendBlockedResult(opts, journal, decision.Row, decision.Reason, snapA.StatusOf(decision.Row.TaskID))
	default:
		return nil, fmt.Errorf("unsupported resolver action: %s", decision.Action)
	}

	snapB, err := readSnapshot(opts.StatePath)
	if err != nil {
		return nil, err
	}
	if snapA.Token() != snapB.Token() {
		actual := snapB.StatusOf(decision.Row.TaskID)
		reason := fmt.Sprintf(
			"state_token_mismatch: before=%s after=%s task=%s actual=%s",
			snapA.Token(),
			snapB.Token(),
			decision.Row.TaskID,
			actual,
		)
		return appendBlockedResult(opts, journal, decision.Row, reason, actual)
	}

	eventIndex := len(journal.Events)
	event := makeInFlightEvent(journal, decision.Row)
	journal.Events = append(journal.Events, event)
	last := journal.Events[eventIndex]
	journal.LastEvent = &last
	if err := saveJournal(opts.JournalPath, journal); err != nil {
		return nil, err
	}

	runErr := invokeArgv(decision.Row.Invoke.Argv, opts.WorkDir, opts.InvokeFn)
	exitCode := 0
	outcome := "applied"
	reason := ""
	stateAfter := Predict(decision.Row)
	if runErr != nil {
		outcome = "failed"
		exitCode = exitCodeFromErr(runErr)
		reason = runErr.Error()
		stateAfter = Status(decision.Row.Requires.TaskStatus)
	} else {
		snapC, err := readSnapshot(opts.StatePath)
		if err != nil {
			return nil, err
		}
		actual := snapC.StatusOf(decision.Row.TaskID)
		if actual != Predict(decision.Row) {
			outcome = "blocked"
			reason = fmt.Sprintf(
				"postcondition_unmet: action=%s produces=%s actual=%s",
				decision.Row.Action,
				Predict(decision.Row),
				actual,
			)
			stateAfter = actual
		}
	}

	journal, err = loadOrInitJournal(opts.JournalPath, opts.SchedulePath)
	if err != nil {
		return nil, err
	}
	if eventIndex >= len(journal.Events) {
		return nil, fmt.Errorf("in-flight event missing at index %d", eventIndex)
	}
	journal.Events[eventIndex].CompletedOn = time.Now().UTC().Format(time.RFC3339)
	journal.Events[eventIndex].Outcome = outcome
	journal.Events[eventIndex].StateAfter = StatusWrapper{TaskStatus: string(stateAfter)}
	if reason != "" {
		journal.Events[eventIndex].Reason = reason
	}
	if runErr != nil {
		journal.Events[eventIndex].ExitCode = &exitCode
	}
	if outcome == "applied" {
		journal.Cursor++
	}
	last = journal.Events[eventIndex]
	journal.LastEvent = &last
	if err := saveJournal(opts.JournalPath, journal); err != nil {
		return nil, err
	}

	return &ExecNextResult{
		Done:     false,
		Cursor:   journal.Cursor,
		StepID:   decision.Row.StepID,
		TaskID:   decision.Row.TaskID,
		Action:   decision.Row.Action,
		Outcome:  outcome,
		EventID:  journal.Events[eventIndex].EventID,
		ExitCode: exitCode,
		Journal:  opts.JournalPath,
		Schedule: opts.SchedulePath,
	}, nil
}

func loadSchedule(schedulePath string) (*Schedule, error) {
	scheduleRaw, err := os.ReadFile(schedulePath)
	if err != nil {
		return nil, fmt.Errorf("read schedule: %w", err)
	}
	if err := ValidateSchedule(scheduleRaw); err != nil {
		return nil, fmt.Errorf("invalid schedule: %w", err)
	}
	var schedule Schedule
	if err := json.Unmarshal(scheduleRaw, &schedule); err != nil {
		return nil, fmt.Errorf("decode schedule: %w", err)
	}
	return &schedule, nil
}

func appendBlockedResult(opts SafeExecOptions, journal *Journal, row ScheduleRow, reason string, actual Status) (*ExecNextResult, error) {
	event := JournalEvent{
		EventID:     nextEventID(journal),
		StepID:      row.StepID,
		Seq:         row.Seq,
		TaskID:      row.TaskID,
		Action:      row.Action,
		Attempt:     nextAttempt(journal, row.StepID),
		StartedOn:   time.Now().UTC().Format(time.RFC3339),
		CompletedOn: time.Now().UTC().Format(time.RFC3339),
		Outcome:     "blocked",
		StateBefore: StatusWrapper{TaskStatus: row.Requires.TaskStatus},
		StateAfter:  StatusWrapper{TaskStatus: string(actual)},
		Reason:      reason,
	}
	journal.Events = append(journal.Events, event)
	journal.LastEvent = &event
	if err := saveJournal(opts.JournalPath, journal); err != nil {
		return nil, err
	}
	return &ExecNextResult{
		Done:     false,
		Cursor:   journal.Cursor,
		StepID:   row.StepID,
		TaskID:   row.TaskID,
		Action:   row.Action,
		Outcome:  "blocked",
		EventID:  event.EventID,
		Journal:  opts.JournalPath,
		Schedule: opts.SchedulePath,
	}, nil
}

func makeInFlightEvent(journal *Journal, row ScheduleRow) JournalEvent {
	return JournalEvent{
		EventID:     nextEventID(journal),
		StepID:      row.StepID,
		Seq:         row.Seq,
		TaskID:      row.TaskID,
		Action:      row.Action,
		Attempt:     nextAttempt(journal, row.StepID),
		StartedOn:   time.Now().UTC().Format(time.RFC3339),
		StateBefore: StatusWrapper{TaskStatus: row.Requires.TaskStatus},
		StateAfter:  StatusWrapper{TaskStatus: row.Produces.TaskStatus},
	}
}

func nextEventID(journal *Journal) string {
	return fmt.Sprintf("E%04d", len(journal.Events)+1)
}

func nextAttempt(journal *Journal, stepID string) int {
	attempt := 1
	for _, ev := range journal.Events {
		if ev.StepID == stepID {
			attempt++
		}
	}
	return attempt
}
