package swim

import (
	"errors"
	"fmt"
	"strings"
)

// ApplyOptions exposes one operator-facing apply invocation.
type ApplyOptions struct {
	SchedulePath   string
	JournalPath    string
	StatePath      string
	ArtifactRoot   string
	LockPath       string
	WorkDir        string
	ExpectCursor   *int
	InvokeFn       func(argv []string, workDir string) error
	ReadSnapshotFn func(path string) (*StateSnapshot, error)
}

// ApplyReport is the stable JSON-friendly report for CLI and MCP callers.
type ApplyReport struct {
	Status          string `json:"status"`
	StepID          string `json:"step_id,omitempty"`
	TaskID          string `json:"task_id,omitempty"`
	Seq             int    `json:"seq,omitempty"`
	ExitCode        int    `json:"exit_code,omitempty"`
	StdoutPath      string `json:"stdout_path,omitempty"`
	StderrPath      string `json:"stderr_path,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Hint            string `json:"hint,omitempty"`
	Boundary        string `json:"boundary,omitempty"`
	InquiryRequired bool   `json:"inquiry_required,omitempty"`
	InquirySource   string `json:"inquiry_source,omitempty"`
	InquiryHint     string `json:"inquiry_hint,omitempty"`
}

// Apply executes exactly one safe apply attempt and normalizes the result
// into stable operator-facing statuses.
func Apply(opts ApplyOptions) (*ApplyReport, error) {
	if _, err := DetectAndMarkUnknown(opts.JournalPath); err != nil {
		return nil, err
	}

	res, err := ExecuteNextStepSafe(SafeExecOptions{
		SchedulePath:   opts.SchedulePath,
		JournalPath:    opts.JournalPath,
		StatePath:      opts.StatePath,
		ArtifactRoot:   opts.ArtifactRoot,
		LockPath:       opts.LockPath,
		WorkDir:        opts.WorkDir,
		ExpectCursor:   opts.ExpectCursor,
		InvokeFn:       opts.InvokeFn,
		ReadSnapshotFn: opts.ReadSnapshotFn,
	})
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			return lockBusyReport(opts.LockPath, opts.SchedulePath, opts.ArtifactRoot), nil
		}
		return nil, err
	}

	journal, err := loadOrInitJournal(opts.JournalPath, opts.SchedulePath)
	if err != nil {
		return nil, err
	}
	if ev, ok := firstUnknownPending(journal, journal.Cursor); ok {
		return &ApplyReport{
			Status:   "unknown_pending",
			StepID:   ev.StepID,
			Seq:      ev.Seq,
			Reason:   fmt.Sprintf("unknown_pending: event_id=%s step_id=%s seq=%d", ev.EventID, ev.StepID, ev.Seq),
			Hint:     "Resolve with swim step --ack-unknown <step_id> before retrying apply.",
			Boundary: "unknown_pending",
		}, nil
	}

	if res.Done {
		return &ApplyReport{Status: "done", Boundary: "done"}, nil
	}

	event := journal.LastEvent
	if event == nil {
		return &ApplyReport{Status: "blocked", Reason: "missing_last_event", Boundary: "blocked"}, nil
	}
	report := &ApplyReport{
		StepID:     event.StepID,
		TaskID:     event.TaskID,
		Seq:        event.Seq,
		ExitCode:   derefInt(event.ExitCode),
		StdoutPath: event.StdoutPath,
		StderrPath: event.StderrPath,
	}

	switch event.Outcome {
	case "applied":
		report.Status = "applied"
		return report, nil
	case "failed":
		report.Status = "blocked"
		report.Boundary = "blocked"
		report.Reason = fmt.Sprintf("invoke_exit: step_id=%s exit_code=%d", event.StepID, report.ExitCode)
		applyReceiptInquiry(report, opts.SchedulePath, opts.ArtifactRoot, event)
		return report, nil
	case "blocked":
		report.Reason = normalizeBlockedReason(event.Reason)
		if strings.Contains(report.Reason, "incomplete_dispatch:") {
			report.Status = "incomplete_dispatch"
			report.Boundary = "incomplete_dispatch"
			report.Hint = "Re-run the same swim step to continue dispatch without re-claiming the task."
			applyReceiptInquiry(report, opts.SchedulePath, opts.ArtifactRoot, event)
			return report, nil
		}
		report.Status = "blocked"
		report.Boundary = "blocked"
		applyReceiptInquiry(report, opts.SchedulePath, opts.ArtifactRoot, event)
		return report, nil
	default:
		report.Status = "blocked"
		report.Boundary = "blocked"
		report.Reason = event.Reason
		applyReceiptInquiry(report, opts.SchedulePath, opts.ArtifactRoot, event)
		return report, nil
	}
}

func lockBusyReport(lockPath, schedulePath, artifactRoot string) *ApplyReport {
	if lockPath == "" {
		lockPath = DeriveLockPath(schedulePath, artifactRoot)
	}
	report := &ApplyReport{Status: "lock_busy"}
	report.Boundary = "blocked"
	pid, startedAt, err := ReadLockHolder(lockPath)
	if err == nil && pid > 0 {
		report.Hint = fmt.Sprintf("Lock held by pid=%d started_at=%s", pid, startedAt)
	}
	return report
}

func normalizeBlockedReason(reason string) string {
	if strings.Contains(reason, "postcondition_unmet:") {
		return strings.Replace(reason, "postcondition_unmet:", "postcondition_mismatch:", 1)
	}
	return reason
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func applyReceiptInquiry(report *ApplyReport, schedulePath, artifactRoot string, event *JournalEvent) {
	if report == nil || event == nil || !isDispatchAction(event.Action) {
		return
	}
	receipt, err := loadDispatchReceipt(schedulePath, artifactRoot, event.StepID, event.Attempt)
	if err != nil || receipt == nil || !receipt.InquiryRequired {
		return
	}
	report.InquiryRequired = true
	report.InquirySource = receipt.InquirySource
	report.InquiryHint = receipt.InquiryHint
}
