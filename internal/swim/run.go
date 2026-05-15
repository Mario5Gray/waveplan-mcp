package swim

import (
	"fmt"
	"strings"
)

// RunOptions controls repeated safe apply or dry-run sequencing.
type RunOptions struct {
	SchedulePath       string
	ReviewSchedulePath string
	JournalPath        string
	StatePath          string
	ArtifactRoot       string
	LockPath           string
	WorkDir            string
	Until              string
	DryRun             bool
	MaxSteps           int
	InvokeFn           func(argv []string, workDir string) error
	ReadSnapshotFn     func(path string) (*StateSnapshot, error)
}

// RunReport captures a multi-step wet or dry run.
type RunReport struct {
	Steps           []ApplyReport `json:"steps"`
	Stopped         string        `json:"stopped"`
	ReachedUntil    bool          `json:"reached_until"`
	DryRun          bool          `json:"dry_run"`
	Boundary        string        `json:"boundary,omitempty"`
	InquiryRequired bool          `json:"inquiry_required,omitempty"`
	InquirySource   string        `json:"inquiry_source,omitempty"`
	InquiryHint     string        `json:"inquiry_hint,omitempty"`
}

type untilKind string

const (
	untilNone   untilKind = ""
	untilAction untilKind = "action"
	untilSeq    untilKind = "seq"
	untilStep   untilKind = "step"
	untilTask   untilKind = "task"
)

type untilCond struct {
	kind   untilKind
	action string
	seq    int
	stepID string
	taskID string
}

// Run executes repeated Apply() calls until a stop condition is met.
func Run(opts RunOptions) (*RunReport, error) {
	cond, err := parseUntil(opts.Until)
	if err != nil {
		return nil, err
	}
	if cond.kind == untilTask {
		schedule, err := loadSchedule(opts.SchedulePath, opts.ReviewSchedulePath)
		if err != nil {
			return nil, err
		}
		lastSeq, ok := lastSeqForTask(schedule, cond.taskID)
		if !ok {
			return nil, fmt.Errorf("invalid until %q: task not present in schedule", opts.Until)
		}
		cond.kind = untilSeq
		cond.seq = lastSeq
	}
	report := &RunReport{DryRun: opts.DryRun}
	if opts.DryRun {
		return runDry(opts, cond, report)
	}
	return runWet(opts, cond, report)
}

func runWet(opts RunOptions, cond untilCond, report *RunReport) (*RunReport, error) {
	for opts.MaxSteps == 0 || len(report.Steps) < opts.MaxSteps {
		step, err := Apply(ApplyOptions{
			SchedulePath:       opts.SchedulePath,
			ReviewSchedulePath: opts.ReviewSchedulePath,
			JournalPath:        opts.JournalPath,
			StatePath:          opts.StatePath,
			ArtifactRoot:       opts.ArtifactRoot,
			LockPath:           opts.LockPath,
			WorkDir:            opts.WorkDir,
			InvokeFn:           opts.InvokeFn,
			ReadSnapshotFn:     opts.ReadSnapshotFn,
		})
		if err != nil {
			return nil, err
		}
		report.Steps = append(report.Steps, *step)

		if step.Status != "applied" {
			report.Stopped = step.Status
			report.Boundary = step.Boundary
			report.InquiryRequired = step.InquiryRequired
			report.InquirySource = step.InquirySource
			report.InquiryHint = step.InquiryHint
			return report, nil
		}
		if matchesUntil(cond, step) {
			report.Stopped = "until"
			report.Boundary = "until_reached"
			report.ReachedUntil = true
			return report, nil
		}
	}
	if opts.MaxSteps > 0 && len(report.Steps) == opts.MaxSteps {
		report.Stopped = "max_steps"
		report.Boundary = "max_steps"
	}
	return report, nil
}

func runDry(opts RunOptions, cond untilCond, report *RunReport) (*RunReport, error) {
	schedule, err := loadSchedule(opts.SchedulePath, opts.ReviewSchedulePath)
	if err != nil {
		return nil, err
	}
	journal, err := loadOrInitJournal(opts.JournalPath, opts.SchedulePath)
	if err != nil {
		return nil, err
	}
	readSnapshot := opts.ReadSnapshotFn
	if readSnapshot == nil {
		readSnapshot = ReadStateSnapshotOrEmpty
	}
	snap, err := readSnapshot(opts.StatePath)
	if err != nil {
		return nil, err
	}
	shadowSnap := cloneSnapshot(snap)
	shadowJournal := cloneJournal(journal)

	for opts.MaxSteps == 0 || len(report.Steps) < opts.MaxSteps {
		decision := ResolveNext(schedule, shadowJournal, shadowSnap)
		step := ApplyReport{}
		switch decision.Action {
		case ActionDone:
			step.Status = "done"
			step.Boundary = "done"
			report.Steps = append(report.Steps, step)
			report.Stopped = "done"
			report.Boundary = "done"
			return report, nil
		case ActionReady:
			argv, err := BuildInvokeArgv(decision.Row, opts.SchedulePath)
			if err != nil {
				return nil, err
			}
			step = ApplyReport{
				Status: "would_apply",
				StepID: decision.Row.StepID,
				TaskID: decision.Row.TaskID,
				Seq:    decision.Row.Seq,
				Reason: strings.Join(argv, " "),
			}
			report.Steps = append(report.Steps, step)
			applyShadow(shadowJournal, shadowSnap, decision.Row)
			if matchesUntil(cond, &step) {
				report.Stopped = "until"
				report.ReachedUntil = true
				return report, nil
			}
		default:
			step = ApplyReport{
				Status:   "would_block",
				StepID:   decision.Row.StepID,
				TaskID:   decision.Row.TaskID,
				Seq:      decision.Row.Seq,
				Reason:   decision.Reason,
				Boundary: "blocked",
			}
			report.Steps = append(report.Steps, step)
			report.Stopped = "blocked"
			report.Boundary = "blocked"
			return report, nil
		}
	}
	if opts.MaxSteps > 0 && len(report.Steps) == opts.MaxSteps {
		report.Stopped = "max_steps"
		report.Boundary = "max_steps"
	}
	return report, nil
}

func parseUntil(raw string) (untilCond, error) {
	if raw == "" {
		return untilCond{kind: untilNone}, nil
	}
	switch raw {
	case "implement", "review", "end_review", "finish", "fix":
		return untilCond{kind: untilAction, action: raw}, nil
	}
	if strings.HasPrefix(raw, "seq:") {
		var seq int
		if _, err := fmt.Sscanf(raw, "seq:%d", &seq); err != nil || seq <= 0 {
			return untilCond{}, fmt.Errorf("invalid until %q: expected seq:N", raw)
		}
		return untilCond{kind: untilSeq, seq: seq}, nil
	}
	if strings.HasPrefix(raw, "step:") {
		stepID := strings.TrimPrefix(raw, "step:")
		if stepID == "" {
			return untilCond{}, fmt.Errorf("invalid until %q: expected step:<step_id>", raw)
		}
		return untilCond{kind: untilStep, stepID: stepID}, nil
	}
	if strings.HasPrefix(raw, "task:") {
		taskID := strings.TrimPrefix(raw, "task:")
		if !isTaskUntilID(taskID) {
			return untilCond{}, fmt.Errorf("invalid until %q: expected task:Tn", raw)
		}
		return untilCond{kind: untilTask, taskID: taskID}, nil
	}
	if isTaskUntilID(raw) {
		return untilCond{kind: untilTask, taskID: raw}, nil
	}
	return untilCond{}, fmt.Errorf("invalid until %q", raw)
}

func matchesUntil(cond untilCond, step *ApplyReport) bool {
	switch cond.kind {
	case untilNone:
		return false
	case untilAction:
		suffix := "_" + cond.action
		return strings.HasSuffix(step.StepID, suffix) ||
			strings.Contains(step.StepID, suffix+"_r")
	case untilSeq:
		return step.Seq == cond.seq
	case untilStep:
		return step.StepID == cond.stepID
	case untilTask:
		return step.TaskID == cond.taskID || strings.HasPrefix(step.TaskID, cond.taskID+".")
	default:
		return false
	}
}

func isTaskUntilID(raw string) bool {
	if raw == "" || raw[0] != 'T' {
		return false
	}
	for i := 1; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return false
		}
	}
	return len(raw) > 1
}

func lastSeqForTask(schedule *Schedule, taskID string) (int, bool) {
	if schedule == nil {
		return 0, false
	}
	lastSeq := 0
	found := false
	for _, row := range schedule.Execution {
		if row.TaskID == taskID || strings.HasPrefix(row.TaskID, taskID+".") {
			if row.Seq > lastSeq {
				lastSeq = row.Seq
			}
			found = true
		}
	}
	return lastSeq, found
}

func cloneJournal(j *Journal) *Journal {
	if j == nil {
		return &Journal{SchemaVersion: 1, Events: []JournalEvent{}}
	}
	out := *j
	out.Events = append([]JournalEvent(nil), j.Events...)
	if j.LastEvent != nil {
		last := *j.LastEvent
		out.LastEvent = &last
	}
	return &out
}

func cloneSnapshot(s *StateSnapshot) *StateSnapshot {
	out := &StateSnapshot{
		raw: rawState{
			Plan:      s.raw.Plan,
			Taken:     map[string]takenEntry{},
			Completed: map[string]completedEntry{},
		},
	}
	for k, v := range s.raw.Taken {
		out.raw.Taken[k] = v
	}
	for k, v := range s.raw.Completed {
		out.raw.Completed[k] = v
	}
	out.canonBody = canonicalize(out.raw)
	return out
}

func applyShadow(j *Journal, s *StateSnapshot, row ScheduleRow) {
	j.Cursor++
	taskID := row.TaskID
	switch Status(row.Produces.TaskStatus) {
	case StatusTaken:
		s.raw.Taken[taskID] = takenEntry{TakenBy: "dry-run", StartedAt: "2026-05-09T00:00:00Z"}
	case StatusReviewTaken:
		entry := s.raw.Taken[taskID]
		entry.ReviewEnteredAt = "2026-05-09T00:00:00Z"
		s.raw.Taken[taskID] = entry
	case StatusReviewEnded:
		entry := s.raw.Taken[taskID]
		entry.ReviewEndedAt = "2026-05-09T00:00:00Z"
		s.raw.Taken[taskID] = entry
	case StatusCompleted:
		delete(s.raw.Taken, taskID)
		s.raw.Completed[taskID] = completedEntry{TakenBy: "dry-run", StartedAt: "2026-05-09T00:00:00Z", FinishedAt: "2026-05-09T00:00:00Z"}
	}
	s.canonBody = canonicalize(s.raw)
}
