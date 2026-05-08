package swim

import "fmt"

// FailureCode is a deterministic machine-readable failure category.
type FailureCode string

const (
	FailureCodePreconditionUnmet   FailureCode = "precondition_unmet"
	FailureCodeUnknownAction       FailureCode = "unknown_action"
	FailureCodeBadStatusInSchedule FailureCode = "bad_status_in_schedule"
)

// EvalResult is the deterministic precondition evaluation result.
type EvalResult struct {
	Allowed bool        `json:"allowed"`
	Reason  string      `json:"reason"`
	Code    FailureCode `json:"code"`
}

// Evaluate checks whether row.requires matches current runtime state for row.task_id.
// It also defends against schedule tampering by enforcing canonical action->status
// mapping for both requires and produces.
func Evaluate(row ScheduleRow, snap *StateSnapshot) EvalResult {
	expected, ok := statusByAction[row.Action]
	if !ok {
		return EvalResult{
			Allowed: false,
			Code:    FailureCodeUnknownAction,
			Reason:  fmt.Sprintf("unknown_action: action=%s", row.Action),
		}
	}

	requires := Status(row.Requires.TaskStatus)
	produces := Status(row.Produces.TaskStatus)
	if !isCanonicalStatus(requires) || !isCanonicalStatus(produces) ||
		requires != Status(expected.requires) || produces != Status(expected.produces) {
		return EvalResult{
			Allowed: false,
			Code:    FailureCodeBadStatusInSchedule,
			Reason:  fmt.Sprintf("bad_status_in_schedule: action=%s requires=%s produces=%s", row.Action, requires, produces),
		}
	}

	actual := Status("<nil>")
	if snap != nil {
		actual = snap.StatusOf(row.TaskID)
	}
	if actual != requires {
		return EvalResult{
			Allowed: false,
			Code:    FailureCodePreconditionUnmet,
			Reason:  fmt.Sprintf("precondition_unmet: action=%s requires=%s actual=%s", row.Action, requires, actual),
		}
	}

	return EvalResult{Allowed: true}
}

// Predict returns the postcondition status encoded on the schedule row.
func Predict(row ScheduleRow) Status {
	return Status(row.Produces.TaskStatus)
}

func isCanonicalStatus(s Status) bool {
	switch s {
	case StatusAvailable, StatusTaken, StatusReviewTaken, StatusReviewEnded, StatusCompleted:
		return true
	default:
		return false
	}
}
