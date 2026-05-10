package swim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RefineRunOptions is the input contract for RefineRun.
//
// The fine journal is independent of any coarse cursor; it is keyed by step_id
// and accumulates terminal outcomes per fine step. The coarse journal is read
// for parent-rollup detection (idempotence) and gets a synthetic
// PARENT_<unit>_rollup event written when all targeted children of a parent
// are terminal AND the coarse postcondition is satisfied.
type RefineRunOptions struct {
	RefinePath        string
	RefineJournalPath string // default: <refine>.journal.json
	CoarseJournalPath string // optional; required when rollup is expected
	StatePath         string

	LockPath string // default derived from refine basename

	WorkDir  string
	InvokeFn func(argv []string, workDir string) error

	// ReadSnapshotFn allows tests to override how state is read. Default reads
	// StatePath via ReadStateSnapshot.
	ReadSnapshotFn func(path string) (*StateSnapshot, error)

	// DryRun resolves the path that would be taken without lock acquisition,
	// invocation, or journal mutation. Useful for previewing.
	DryRun bool
}

// RefineApplyReport is one fine-step attempt result, mirroring ApplyReport
// shape so CLI / MCP callers can reuse the same JSON parser.
type RefineApplyReport struct {
	Status     string `json:"status"`
	StepID     string `json:"step_id"`
	ParentUnit string `json:"parent_unit"`
	Seq        int    `json:"seq,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	StdoutPath string `json:"stdout_path,omitempty"`
	StderrPath string `json:"stderr_path,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Hint       string `json:"hint,omitempty"`
	WouldApply bool   `json:"would_apply,omitempty"`
	WouldBlock bool   `json:"would_block,omitempty"`
}

// RefineRunReport summarizes the full RefineRun call for callers.
type RefineRunReport struct {
	Steps             []RefineApplyReport `json:"steps"`
	ParentsCompleted  []string            `json:"parents_completed"`
	Stopped           string              `json:"stopped"`
	ProtocolNote      string              `json:"protocol_note,omitempty"`
	DryRun            bool                `json:"dry_run,omitempty"`
}

// RefineRun walks the refinement sidecar units in array order, executing each
// fine step that is not already terminal in the fine journal. After every
// successful fine apply, attempts parent rollup if all targeted children of
// the parent are terminal (applied|waived).
//
// Concurrency note: the refine and coarse locks are independent; T6.3 is safe
// only when an operator does not run coarse Apply against the same parent
// unit while RefineRun is in flight. This is a workflow contract, not enforced.
func RefineRun(opts RefineRunOptions) (*RefineRunReport, error) {
	side, err := loadRefinementSidecar(opts.RefinePath)
	if err != nil {
		return nil, err
	}

	refineJournalPath := opts.RefineJournalPath
	if refineJournalPath == "" {
		refineJournalPath = strings.TrimSuffix(opts.RefinePath, ".json") + ".journal.json"
	}

	report := &RefineRunReport{Stopped: "done", DryRun: opts.DryRun}

	if opts.DryRun {
		return refineRunDry(side, opts, refineJournalPath, report)
	}

	lockPath := opts.LockPath
	if lockPath == "" {
		lockPath = deriveRefineLockPath(opts.RefinePath)
	}

	lock, lockErr := AcquireLock(lockPath)
	if lockErr != nil {
		report.Stopped = "lock_busy"
		report.ProtocolNote = "lock_busy"
		pid, holder, _ := ReadLockHolder(lockPath)
		hint := holder
		if hint == "" {
			hint = fmt.Sprintf("pid=%d", pid)
		}
		report.Steps = append(report.Steps, RefineApplyReport{
			Status: "lock_busy",
			Hint:   hint,
		})
		return report, nil
	}
	defer lock.Release()

	fineJournal, err := loadOrInitFineJournal(refineJournalPath, opts.RefinePath)
	if err != nil {
		return nil, err
	}

	readSnap := opts.ReadSnapshotFn
	if readSnap == nil {
		readSnap = ReadStateSnapshot
	}

	var coarseJournal *Journal
	if opts.CoarseJournalPath != "" {
		coarseJournal, err = loadOrInitJournal(opts.CoarseJournalPath, opts.CoarseJournalPath)
		if err != nil {
			return nil, err
		}
	}

	// Group fine units by parent in array order.
	parentOrder := make([]string, 0)
	parentSeen := map[string]struct{}{}
	parentChildren := map[string][]RefineUnit{}
	for _, u := range side.Units {
		if _, ok := parentSeen[u.ParentUnit]; !ok {
			parentSeen[u.ParentUnit] = struct{}{}
			parentOrder = append(parentOrder, u.ParentUnit)
		}
		parentChildren[u.ParentUnit] = append(parentChildren[u.ParentUnit], u)
	}

	for _, parentID := range parentOrder {
		// Cross-parent gate: parent's coarse predecessors all complete?
		gateOK, gateReason, snapErr := crossParentGate(opts.StatePath, readSnap, side.CoarsePlan, parentID)
		if snapErr != nil {
			return nil, snapErr
		}
		if !gateOK {
			report.Stopped = "cross_parent_gate"
			report.Steps = append(report.Steps, RefineApplyReport{
				Status:     "blocked",
				StepID:     parentID,
				ParentUnit: parentID,
				Reason:     gateReason,
			})
			break
		}

		for _, child := range parentChildren[parentID] {
			if fineEventTerminal(fineJournal, child.StepID) {
				continue // idempotent skip
			}

			// Intra-parent dep gate.
			if reason, ok := intraParentDepsSatisfied(fineJournal, child); !ok {
				report.Stopped = "non_applied"
				report.Steps = append(report.Steps, RefineApplyReport{
					Status:     "blocked",
					StepID:     child.StepID,
					ParentUnit: child.ParentUnit,
					Seq:        child.Seq,
					Reason:     reason,
				})
				return report, nil
			}

			// Run the fine step. Fine steps do NOT enforce coarse postcondition;
			// they only execute argv, capture logs, and append a fine journal event.
			result := executeFineStep(child, fineJournal, refineJournalPath, opts)
			report.Steps = append(report.Steps, result)
			if result.Status != "applied" {
				report.Stopped = "non_applied"
				return report, nil
			}
		}

		// All children of this parent done in fine journal?
		if !allChildrenTerminal(fineJournal, parentChildren[parentID]) {
			continue
		}

		// Roll-up gate 1: idempotence — already rolled up?
		if coarseJournal != nil && parentAlreadyRolledUp(coarseJournal, parentID) {
			continue
		}

		// Roll-up gate 2: validate coarse postcondition (when fine units carry one).
		expectedStatus := lastChildExpectedStatus(parentChildren[parentID])
		if expectedStatus != "" {
			snap, err := readSnap(opts.StatePath)
			if err != nil {
				return nil, err
			}
			actual := snap.StatusOf(parentID)
			if string(actual) != expectedStatus {
				report.Steps = append(report.Steps, RefineApplyReport{
					Status:     "blocked",
					StepID:     parentRollupID(parentID),
					ParentUnit: parentID,
					Reason: fmt.Sprintf("rollup_postcondition_mismatch: expected %s, actual %s",
						expectedStatus, actual),
				})
				report.Stopped = "non_applied"
				return report, nil
			}
		}

		// Write synthetic rollup event into the coarse journal.
		if coarseJournal != nil {
			beforeStatus := string(StatusTaken)
			afterStatus := expectedStatus
			if afterStatus == "" {
				afterStatus = string(StatusReviewTaken)
			}
			if snap, err := readSnap(opts.StatePath); err == nil {
				// before-status reflects current parent status; the rollup event
				// records the transition observed externally.
				prior := string(snap.StatusOf(parentID))
				if prior != "" {
					beforeStatus = prior
				}
			}
			if err := writeRollupEvent(coarseJournal, opts.CoarseJournalPath, parentID, beforeStatus, afterStatus); err != nil {
				return nil, err
			}
			report.ParentsCompleted = append(report.ParentsCompleted, parentID)
		}
	}

	if report.Stopped == "done" && len(report.Steps) == 0 && len(report.ParentsCompleted) == 0 {
		report.ProtocolNote = "idempotent_noop"
	}
	return report, nil
}

func refineRunDry(side *RefinementSidecar, opts RefineRunOptions, fineJournalPath string, report *RefineRunReport) (*RefineRunReport, error) {
	fineJournal, _ := loadOrInitFineJournal(fineJournalPath, opts.RefinePath)
	readSnap := opts.ReadSnapshotFn
	if readSnap == nil {
		readSnap = ReadStateSnapshot
	}
	parentOrder := make([]string, 0)
	parentSeen := map[string]struct{}{}
	parentChildren := map[string][]RefineUnit{}
	for _, u := range side.Units {
		if _, ok := parentSeen[u.ParentUnit]; !ok {
			parentSeen[u.ParentUnit] = struct{}{}
			parentOrder = append(parentOrder, u.ParentUnit)
		}
		parentChildren[u.ParentUnit] = append(parentChildren[u.ParentUnit], u)
	}
	// Dry-run tracks would-apply siblings in memory so dependency chains
	// validate against the simulated future, not just the persisted past.
	wouldApplied := map[string]bool{}
	for _, parentID := range parentOrder {
		gateOK, gateReason, _ := crossParentGate(opts.StatePath, readSnap, side.CoarsePlan, parentID)
		if !gateOK {
			report.Steps = append(report.Steps, RefineApplyReport{
				Status:     "blocked",
				StepID:     parentID,
				ParentUnit: parentID,
				Reason:     gateReason,
				WouldBlock: true,
			})
			report.Stopped = "cross_parent_gate"
			break
		}
		for _, child := range parentChildren[parentID] {
			if fineEventTerminal(fineJournal, child.StepID) {
				wouldApplied[child.StepID] = true
				continue
			}
			depsOK := true
			var blockReason string
			for _, dep := range child.DependsOn {
				if !fineEventTerminal(fineJournal, dep) && !wouldApplied[dep] {
					depsOK = false
					blockReason = fmt.Sprintf("intra_parent_dep_unmet: %s depends on %s (not applied|waived)", child.StepID, dep)
					break
				}
			}
			if !depsOK {
				report.Steps = append(report.Steps, RefineApplyReport{
					Status:     "blocked",
					StepID:     child.StepID,
					ParentUnit: child.ParentUnit,
					Reason:     blockReason,
					WouldBlock: true,
				})
				report.Stopped = "non_applied"
				return report, nil
			}
			report.Steps = append(report.Steps, RefineApplyReport{
				Status:     "would_apply",
				StepID:     child.StepID,
				ParentUnit: child.ParentUnit,
				Seq:        child.Seq,
				WouldApply: true,
			})
			wouldApplied[child.StepID] = true
		}
	}
	return report, nil
}

func executeFineStep(unit RefineUnit, fineJournal *Journal, fineJournalPath string, opts RefineRunOptions) RefineApplyReport {
	report := RefineApplyReport{
		StepID:     unit.StepID,
		ParentUnit: unit.ParentUnit,
		Seq:        unit.Seq,
	}

	// Determine log paths. Per locked decision, layout is
	// .waveplan/swim/<refine-basename>/logs/<step_id>.<attempt>.{stdout,stderr}.log
	attempt := countAttempts(fineJournal, unit.StepID) + 1
	logsDir := deriveRefineLogsDir(opts.RefinePath)
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		report.Status = "blocked"
		report.Reason = "log_dir_setup_failed: " + err.Error()
		return report
	}
	stdoutPath := filepath.Join(logsDir, fmt.Sprintf("%s.%d.stdout.log", unit.StepID, attempt))
	stderrPath := filepath.Join(logsDir, fmt.Sprintf("%s.%d.stderr.log", unit.StepID, attempt))

	// Read parent's current coarse status to populate journal state fields.
	// Fine events DO NOT claim to mutate coarse state; state_before == state_after.
	readSnap := opts.ReadSnapshotFn
	if readSnap == nil {
		readSnap = ReadStateSnapshot
	}
	parentStatus := string(StatusAvailable)
	if opts.StatePath != "" {
		if snap, err := readSnap(opts.StatePath); err == nil {
			parentStatus = string(snap.StatusOf(unit.ParentUnit))
		}
	}

	startedOn := time.Now().UTC().Format(time.RFC3339)
	in := JournalEvent{
		EventID:     fmt.Sprintf("E%04d", len(fineJournal.Events)+1),
		StepID:      unit.StepID,
		Seq:         unit.Seq,
		TaskID:      unit.ParentUnit,
		Action:      "implement",
		Attempt:     attempt,
		StartedOn:   startedOn,
		StateBefore: StatusWrapper{TaskStatus: parentStatus},
		StateAfter:  StatusWrapper{TaskStatus: parentStatus},
	}
	fineJournal.Events = append(fineJournal.Events, in)
	if err := saveJournal(fineJournalPath, fineJournal); err != nil {
		report.Status = "blocked"
		report.Reason = "journal_write_in_flight_failed: " + err.Error()
		return report
	}

	// Invoke argv with stdout/stderr captured directly to disk.
	exitCode, invokeErr := invokeWithCapture(unit.Invoke.Argv, opts.WorkDir, stdoutPath, stderrPath, opts.InvokeFn)
	completedOn := time.Now().UTC().Format(time.RFC3339)

	idx := len(fineJournal.Events) - 1
	fineJournal.Events[idx].CompletedOn = completedOn
	fineJournal.Events[idx].StdoutPath = stdoutPath
	fineJournal.Events[idx].StderrPath = stderrPath
	fineJournal.Events[idx].ExitCode = &exitCode

	report.StdoutPath = stdoutPath
	report.StderrPath = stderrPath
	report.ExitCode = exitCode

	if invokeErr != nil || exitCode != 0 {
		fineJournal.Events[idx].Outcome = "failed"
		_ = saveJournal(fineJournalPath, fineJournal)
		report.Status = "blocked"
		if invokeErr != nil {
			report.Reason = fmt.Sprintf("invoke_failed: exit=%d err=%s", exitCode, invokeErr.Error())
		} else {
			report.Reason = fmt.Sprintf("invoke_failed: exit=%d", exitCode)
		}
		return report
	}

	fineJournal.Events[idx].Outcome = "applied"
	if err := saveJournal(fineJournalPath, fineJournal); err != nil {
		report.Status = "blocked"
		report.Reason = "journal_write_terminal_failed: " + err.Error()
		return report
	}
	report.Status = "applied"
	return report
}

// invokeWithCapture mirrors runner.go's invocation surface but is local to
// refine to keep concerns isolated. Real fork-exec is delegated to runner's
// invokeArgv (which returns only error; exit code is recovered via
// exitCodeFromErr). Tests inject InvokeFn; the log files are still touched so
// downstream readers can rely on their existence.
func invokeWithCapture(argv []string, workDir, stdoutPath, stderrPath string, fn func([]string, string) error) (int, error) {
	if fn != nil {
		_ = os.WriteFile(stdoutPath, nil, 0o644)
		_ = os.WriteFile(stderrPath, nil, 0o644)
		if err := fn(argv, workDir); err != nil {
			return exitCodeFromErr(err), err
		}
		return 0, nil
	}
	if err := invokeArgv(argv, workDir, "", stdoutPath, stderrPath, nil); err != nil {
		return exitCodeFromErr(err), err
	}
	return 0, nil
}

func loadOrInitFineJournal(path, refinePath string) (*Journal, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		return &Journal{
			SchemaVersion: 1,
			SchedulePath:  refinePath,
			Cursor:        0,
			Events:        []JournalEvent{},
		}, nil
	}
	var j Journal
	if err := json.Unmarshal(body, &j); err != nil {
		return nil, fmt.Errorf("parse fine journal %q: %w", path, err)
	}
	if j.Events == nil {
		j.Events = []JournalEvent{}
	}
	return &j, nil
}

func fineEventTerminal(j *Journal, stepID string) bool {
	for _, e := range j.Events {
		if e.StepID != stepID {
			continue
		}
		if e.Outcome == "applied" || e.Outcome == "waived" {
			return true
		}
	}
	return false
}

func intraParentDepsSatisfied(j *Journal, unit RefineUnit) (string, bool) {
	for _, dep := range unit.DependsOn {
		if !fineEventTerminal(j, dep) {
			return fmt.Sprintf("intra_parent_dep_unmet: %s depends on %s (not applied|waived)", unit.StepID, dep), false
		}
	}
	return "", true
}

func allChildrenTerminal(j *Journal, children []RefineUnit) bool {
	for _, c := range children {
		if !fineEventTerminalAny(j, c.StepID) {
			return false
		}
	}
	return true
}

func fineEventTerminalAny(j *Journal, stepID string) bool {
	for _, e := range j.Events {
		if e.StepID == stepID && (e.Outcome == "applied" || e.Outcome == "waived") {
			return true
		}
	}
	return false
}

func parentAlreadyRolledUp(j *Journal, parentID string) bool {
	target := parentRollupID(parentID)
	for _, e := range j.Events {
		if e.StepID == target && e.Outcome == "applied" {
			return true
		}
	}
	return false
}

func parentRollupID(parentID string) string {
	return "PARENT_" + parentID + "_rollup"
}

func writeRollupEvent(j *Journal, journalPath, parentID, beforeStatus, afterStatus string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	exitZero := 0
	if beforeStatus == "" {
		beforeStatus = string(StatusTaken)
	}
	if afterStatus == "" {
		afterStatus = string(StatusReviewTaken)
	}
	ev := JournalEvent{
		EventID:     fmt.Sprintf("E%04d", len(j.Events)+1),
		StepID:      parentRollupID(parentID),
		Seq:         len(j.Events) + 1,
		TaskID:      parentID,
		Action:      "implement",
		Attempt:     1,
		StartedOn:   now,
		CompletedOn: now,
		Outcome:     "applied",
		ExitCode:    &exitZero,
		StateBefore: StatusWrapper{TaskStatus: beforeStatus},
		StateAfter:  StatusWrapper{TaskStatus: afterStatus},
		Reason:      "synthetic refinement rollup: all targeted fine children terminal",
	}
	j.Events = append(j.Events, ev)
	return saveJournal(journalPath, j)
}

// crossParentGate checks that every coarse predecessor of parentID is `complete`
// in the live state. Predecessors are derived from the parent's `task` siblings
// in the coarse plan: any unit that shares the parent's parent_task and has a
// lower lexicographic ID. v1 keeps this conservative and walks the parent
// task's wave for predecessors with lower waves; cross-task gating reads coarse
// plan deps directly.
func crossParentGate(statePath string, readSnap func(string) (*StateSnapshot, error), coarsePlanPath, parentID string) (bool, string, error) {
	plan, err := loadCoarsePlan(coarsePlanPath)
	if err != nil {
		return false, "", err
	}
	parent, ok := plan.Units[parentID]
	if !ok {
		return false, fmt.Sprintf("parent %s not in coarse plan", parentID), nil
	}
	// Predecessors = units in earlier waves OR same wave with lower id.
	// For T6.3 v1: we only gate on earlier-wave units of the same parent's parent task.
	// Stronger inter-task gating belongs to T2.x cross-parent dependency machinery.
	snap, err := readSnap(statePath)
	if err != nil {
		return false, "", err
	}
	for id, u := range plan.Units {
		if id == parentID {
			continue
		}
		// Only consider strict predecessors: earlier wave.
		if u.Wave >= parent.Wave {
			continue
		}
		if snap.StatusOf(id) != StatusCompleted {
			return false, fmt.Sprintf("cross_parent_gate: predecessor %s not complete (status=%s)", id, snap.StatusOf(id)), nil
		}
	}
	return true, "", nil
}

func lastChildExpectedStatus(children []RefineUnit) string {
	if len(children) == 0 {
		return ""
	}
	last := children[len(children)-1]
	if last.Produces == nil {
		return ""
	}
	if v, ok := last.Produces["task_status"].(string); ok {
		return v
	}
	return ""
}

func loadRefinementSidecar(path string) (*RefinementSidecar, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read refine sidecar %q: %w", path, err)
	}
	var s RefinementSidecar
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("parse refine sidecar %q: %w", path, err)
	}
	return &s, nil
}

func deriveRefineLockPath(refinePath string) string {
	base := strings.TrimSuffix(filepath.Base(refinePath), ".json")
	return filepath.Join(filepath.Dir(refinePath), ".waveplan", "swim", base, "swim.lock")
}

func deriveRefineLogsDir(refinePath string) string {
	base := strings.TrimSuffix(filepath.Base(refinePath), ".json")
	return filepath.Join(filepath.Dir(refinePath), ".waveplan", "swim", base, "logs")
}

func countAttempts(j *Journal, stepID string) int {
	n := 0
	for _, e := range j.Events {
		if e.StepID == stepID {
			if e.Attempt > n {
				n = e.Attempt
			}
		}
	}
	return n
}
