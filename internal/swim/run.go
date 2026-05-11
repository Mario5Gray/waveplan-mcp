package swim

import (
	"fmt"
	"strings"
)

// RunOptions controls repeated safe apply or dry-run sequencing.
type RunOptions struct {
	SchedulePath   string
	JournalPath    string
	StatePath      string
	LockPath       string
	WorkDir        string
	Until          string
	DryRun         bool
	MaxSteps       int
	InvokeFn       func(argv []string, workDir string) error
	ReadSnapshotFn func(path string) (*StateSnapshot, error)
}

// RunReport captures a multi-step wet or dry run.
type RunReport struct {
	Steps        []ApplyReport `json:"steps"`
	Stopped      string        `json:"stopped"`
	ReachedUntil bool          `json:"reached_until"`
	DryRun       bool          `json:"dry_run"`
}

type untilKind string

const (
	untilNone   untilKind = ""
	untilAction untilKind = "action"
	untilSeq    untilKind = "seq"
	untilStep   untilKind = "step"
)

type untilCond struct {
	kind   untilKind
	action string
	seq    int
	stepID string
}

// Run executes repeated Apply() calls until a stop condition is met.
func Run(opts RunOptions) (*RunReport, error) {
	cond, err := parseUntil(opts.Until)
	if err != nil {
		return nil, err
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
			SchedulePath:   opts.SchedulePath,
			JournalPath:    opts.JournalPath,
			StatePath:      opts.StatePath,
			LockPath:       opts.LockPath,
			WorkDir:        opts.WorkDir,
			InvokeFn:       opts.InvokeFn,
			ReadSnapshotFn: opts.ReadSnapshotFn,
		})
		if err != nil {
			return nil, err
		}
		report.Steps = append(report.Steps, *step)

		if step.Status != "applied" {
			report.Stopped = step.Status
			return report, nil
		}
		if matchesUntil(cond, step) {
			report.Stopped = "until"
			report.ReachedUntil = true
			return report, nil
		}
	}
	if opts.MaxSteps > 0 && len(report.Steps) == opts.MaxSteps {
		report.Stopped = "max_steps"
	}
	return report, nil
}

func runDry(opts RunOptions, cond untilCond, report *RunReport) (*RunReport, error) {
	schedule, err := loadSchedule(opts.SchedulePath)
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
			report.Steps = append(report.Steps, step)
			report.Stopped = "done"
			return report, nil
		case ActionReady:
			step = ApplyReport{
				Status: "would_apply",
				StepID: decision.Row.StepID,
				Seq:    decision.Row.Seq,
				Reason: strings.Join(decision.Row.Invoke.Argv, " "),
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
				Status: "would_block",
				StepID: decision.Row.StepID,
				Seq:    decision.Row.Seq,
				Reason: decision.Reason,
			}
			report.Steps = append(report.Steps, step)
			report.Stopped = "blocked"
			return report, nil
		}
	}
	if opts.MaxSteps > 0 && len(report.Steps) == opts.MaxSteps {
		report.Stopped = "max_steps"
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
	default:
		return false
	}
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
