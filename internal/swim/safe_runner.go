package swim

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// SafeExecOptions wraps one-step execution with lock ownership and
// snapshot-based race closure.
type SafeExecOptions struct {
	SchedulePath       string
	ReviewSchedulePath string
	JournalPath        string
	StatePath          string
	ArtifactRoot       string
	LockPath           string
	WorkDir            string
	ExpectCursor       *int
	InvokeFn           func(argv []string, workDir string) error
	ReadSnapshotFn     func(path string) (*StateSnapshot, error)
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
		lockPath = DeriveLockPath(opts.SchedulePath, opts.ArtifactRoot)
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

	readSnapshot := opts.ReadSnapshotFn
	if readSnapshot == nil {
		readSnapshot = ReadStateSnapshotOrEmpty
	}

	snapA, err := readSnapshot(opts.StatePath)
	if err != nil {
		return nil, err
	}

	decision := ResolveNext(schedule, journal, snapA)
	recoverDispatch := false
	if decision.Action == ActionReady && isDispatchAction(decision.Row.Action) && anyDispatchReceiptExists(opts.SchedulePath, opts.ArtifactRoot, decision.Row.StepID) {
		if err := AdvanceStateSnapshot(opts.StatePath, opts.SchedulePath, decision.Row.TaskID, Predict(decision.Row), time.Now().UTC()); err != nil {
			return nil, err
		}
		return appendAdoptedAppliedResult(opts, journal, decision.Row, Predict(decision.Row), "receipt_adopt")
	}
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
		if decision.Action == ActionDrift {
			actual := snapA.StatusOf(decision.Row.TaskID)
			if actual == Predict(decision.Row) {
				if isDispatchAction(decision.Row.Action) && !anyDispatchReceiptExists(opts.SchedulePath, opts.ArtifactRoot, decision.Row.StepID) {
					recoverDispatch = true
					break
				}
				return appendAdoptedAppliedResult(opts, journal, decision.Row, actual, "idempotent_adopt")
			}
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
	if recoverDispatch && snapB.StatusOf(decision.Row.TaskID) != Predict(decision.Row) {
		actual := snapB.StatusOf(decision.Row.TaskID)
		reason := fmt.Sprintf(
			"dispatch_recovery_lost_state: action=%s produces=%s actual=%s",
			decision.Row.Action,
			Predict(decision.Row),
			actual,
		)
		return appendBlockedResult(opts, journal, decision.Row, reason, actual)
	}

	eventIndex := len(journal.Events)
	event := makeInFlightEvent(journal, decision.Row)
	event.StdoutPath, event.StderrPath = deriveLogPaths(opts.SchedulePath, opts.ArtifactRoot, decision.Row.StepID, event.Attempt)
	stdoutAbsPath, stderrAbsPath := deriveLogAbsPaths(opts.SchedulePath, opts.ArtifactRoot, decision.Row.StepID, event.Attempt)
	receiptPath := ""
	extraEnv := map[string]string{}
	if isDispatchAction(decision.Row.Action) {
		receiptPath = deriveDispatchReceiptPath(opts.SchedulePath, opts.ArtifactRoot, decision.Row.StepID, event.Attempt)
		extraEnv["SWIM_DISPATCH_RECEIPT_PATH"] = dispatchReceiptAbsPath(opts.SchedulePath, opts.ArtifactRoot, decision.Row.StepID, event.Attempt)
		extraEnv["SWIM_STEP_ID"] = decision.Row.StepID
		extraEnv["SWIM_TASK_ID"] = decision.Row.TaskID
		extraEnv["SWIM_ACTION"] = decision.Row.Action
	}
	if decision.Row.Action == "fix" {
		for i := len(journal.Events) - 1; i >= 0; i-- {
			ev := journal.Events[i]
			if ev.Action != "review" || ev.TaskID != decision.Row.TaskID || ev.StdoutPath == "" {
				continue
			}
			if filepath.IsAbs(ev.StdoutPath) {
				extraEnv["SWIM_PRIOR_STDOUT_PATH"] = ev.StdoutPath
			} else {
				extraEnv["SWIM_PRIOR_STDOUT_PATH"] = filepath.Join(filepath.Dir(opts.SchedulePath), ev.StdoutPath)
			}
			break
		}
	}
	journal.Events = append(journal.Events, event)
	last := journal.Events[eventIndex]
	journal.LastEvent = &last
	if err := saveJournal(opts.JournalPath, journal); err != nil {
		return nil, err
	}
	if err := WriteLockHolder(lock.f, os.Getpid(), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}

	argv, err := BuildInvokeArgv(decision.Row, opts.SchedulePath)
	if err != nil {
		return nil, err
	}
	runErr := invokeArgv(argv, opts.WorkDir, stdoutAbsPath, stderrAbsPath, extraEnv, opts.InvokeFn)
	exitCode := 0
	outcome := "applied"
	reason := ""
	stateAfter := Predict(decision.Row)
	if runErr != nil {
		exitCode = exitCodeFromErr(runErr)
		if isDispatchAction(decision.Row.Action) {
			snapC, err := readSnapshot(opts.StatePath)
			if err != nil {
				return nil, err
			}
			actual := snapC.StatusOf(decision.Row.TaskID)
			if actual == Predict(decision.Row) && !dispatchReceiptExists(opts.SchedulePath, opts.ArtifactRoot, decision.Row.StepID, event.Attempt) {
				outcome = "blocked"
				reason = fmt.Sprintf(
					"incomplete_dispatch: action=%s produces=%s actual=%s receipt_missing=%s invoke_error=%s",
					decision.Row.Action,
					Predict(decision.Row),
					actual,
					receiptPath,
					runErr.Error(),
				)
				stateAfter = actual
			} else {
				outcome = "failed"
				reason = runErr.Error()
				stateAfter = Status(decision.Row.Requires.TaskStatus)
			}
		} else {
			outcome = "failed"
			reason = runErr.Error()
			stateAfter = Status(decision.Row.Requires.TaskStatus)
		}
	} else {
		if err := AdvanceStateSnapshot(opts.StatePath, opts.SchedulePath, decision.Row.TaskID, Predict(decision.Row), time.Now().UTC()); err != nil {
			return nil, err
		}
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
		} else if isDispatchAction(decision.Row.Action) && !dispatchReceiptExists(opts.SchedulePath, opts.ArtifactRoot, decision.Row.StepID, event.Attempt) {
			outcome = "blocked"
			reason = fmt.Sprintf(
				"incomplete_dispatch: action=%s produces=%s actual=%s receipt_missing=%s",
				decision.Row.Action,
				Predict(decision.Row),
				actual,
				receiptPath,
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

func loadSchedule(schedulePath, reviewSchedulePath string) (*Schedule, error) {
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
	if reviewSchedulePath == "" {
		return &schedule, nil
	}

	reviewRaw, err := os.ReadFile(reviewSchedulePath)
	if err != nil {
		return nil, fmt.Errorf("read review schedule sidecar: %w", err)
	}
	if err := ValidateReviewScheduleSidecar(reviewRaw, &schedule); err != nil {
		return nil, fmt.Errorf("invalid review schedule sidecar: %w", err)
	}
	var reviewSidecar ReviewScheduleSidecar
	if err := json.Unmarshal(reviewRaw, &reviewSidecar); err != nil {
		return nil, fmt.Errorf("decode review schedule sidecar: %w", err)
	}
	merged, err := mergeExecutionWithReviewInsertions(schedule.Execution, reviewSidecar.Insertions)
	if err != nil {
		return nil, err
	}
	schedule.Execution = merged
	return &schedule, nil
}

func mergeExecutionWithReviewInsertions(base []ScheduleRow, insertions []ReviewScheduleInsertion) ([]ScheduleRow, error) {
	if len(insertions) == 0 {
		out := append([]ScheduleRow(nil), base...)
		for i := range out {
			out[i].Seq = i + 1
		}
		return out, nil
	}

	children := make(map[string][]ReviewScheduleInsertion)
	for _, ins := range insertions {
		children[ins.AfterStepID] = append(children[ins.AfterStepID], ins)
	}
	for anchor := range children {
		rows := children[anchor]
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].SeqHint == rows[j].SeqHint {
				return rows[i].ID < rows[j].ID
			}
			return rows[i].SeqHint < rows[j].SeqHint
		})
		children[anchor] = rows
	}

	merged := make([]ScheduleRow, 0, len(base)+len(insertions))
	emitted := make(map[string]struct{}, len(insertions))
	var appendChildren func(anchor string) error
	appendChildren = func(anchor string) error {
		for _, ins := range children[anchor] {
			if _, ok := emitted[ins.ID]; ok {
				return fmt.Errorf("duplicate insertion id during merge: %s", ins.ID)
			}
			emitted[ins.ID] = struct{}{}
			merged = append(merged, scheduleRowFromInsertion(ins))
			if err := appendChildren(ins.ID); err != nil {
				return err
			}
		}
		return nil
	}

	for _, row := range base {
		merged = append(merged, row)
		if err := appendChildren(row.StepID); err != nil {
			return nil, err
		}
	}
	if len(emitted) != len(insertions) {
		return nil, fmt.Errorf("failed to merge all review insertions: merged=%d insertions=%d", len(emitted), len(insertions))
	}
	for i := range merged {
		merged[i].Seq = i + 1
	}
	return merged, nil
}

func scheduleRowFromInsertion(ins ReviewScheduleInsertion) ScheduleRow {
	return ScheduleRow{
		StepID:   ins.StepID,
		TaskID:   ins.TaskID,
		Action:   ins.Action,
		Requires: ins.Requires,
		Produces: ins.Produces,
		Operation: ins.Operation,
		Source:   scheduleRowSourceReviewSidecar,
		Invoke: InvokeSpec{
			Argv: append([]string(nil), ins.Invoke.Argv...),
		},
	}
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

func appendAdoptedAppliedResult(opts SafeExecOptions, journal *Journal, row ScheduleRow, actual Status, adoptionKind string) (*ExecNextResult, error) {
	if adoptionKind == "" {
		adoptionKind = "idempotent_adopt"
	}
	event := JournalEvent{
		EventID:     nextEventID(journal),
		StepID:      row.StepID,
		Seq:         row.Seq,
		TaskID:      row.TaskID,
		Action:      row.Action,
		Attempt:     nextAttempt(journal, row.StepID),
		StartedOn:   time.Now().UTC().Format(time.RFC3339),
		CompletedOn: time.Now().UTC().Format(time.RFC3339),
		Outcome:     "applied",
		StateBefore: StatusWrapper{TaskStatus: row.Requires.TaskStatus},
		StateAfter:  StatusWrapper{TaskStatus: string(actual)},
		Reason: fmt.Sprintf(
			"%s: action=%s actual=%s matches_produces=%s",
			adoptionKind,
			row.Action,
			actual,
			Predict(row),
		),
	}
	journal.Events = append(journal.Events, event)
	journal.Cursor++
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
		Outcome:  "applied",
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

func exitError(code int) error {
	return exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
}
