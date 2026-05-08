package swim

import "testing"

func TestEvaluate_Table(t *testing.T) {
	taskID := "T9.9"

	cases := []struct {
		name        string
		action      string
		requires    Status
		produces    Status
		actual      Status
		wantAllowed bool
		wantCode    FailureCode
		wantReason  string
	}{
		{
			name:        "implement on available",
			action:      "implement",
			requires:    StatusAvailable,
			produces:    StatusTaken,
			actual:      StatusAvailable,
			wantAllowed: true,
			wantCode:    "",
			wantReason:  "",
		},
		{
			name:        "implement on taken",
			action:      "implement",
			requires:    StatusAvailable,
			produces:    StatusTaken,
			actual:      StatusTaken,
			wantAllowed: false,
			wantCode:    FailureCodePreconditionUnmet,
			wantReason:  "precondition_unmet: action=implement requires=available actual=taken",
		},
		{
			name:        "review on taken",
			action:      "review",
			requires:    StatusTaken,
			produces:    StatusReviewTaken,
			actual:      StatusTaken,
			wantAllowed: true,
			wantCode:    "",
			wantReason:  "",
		},
		{
			name:        "review on available",
			action:      "review",
			requires:    StatusTaken,
			produces:    StatusReviewTaken,
			actual:      StatusAvailable,
			wantAllowed: false,
			wantCode:    FailureCodePreconditionUnmet,
			wantReason:  "precondition_unmet: action=review requires=taken actual=available",
		},
		{
			name:        "end_review on review_taken",
			action:      "end_review",
			requires:    StatusReviewTaken,
			produces:    StatusReviewEnded,
			actual:      StatusReviewTaken,
			wantAllowed: true,
			wantCode:    "",
			wantReason:  "",
		},
		{
			name:        "end_review on review_ended",
			action:      "end_review",
			requires:    StatusReviewTaken,
			produces:    StatusReviewEnded,
			actual:      StatusReviewEnded,
			wantAllowed: false,
			wantCode:    FailureCodePreconditionUnmet,
			wantReason:  "precondition_unmet: action=end_review requires=review_taken actual=review_ended",
		},
		{
			name:        "finish on review_ended",
			action:      "finish",
			requires:    StatusReviewEnded,
			produces:    StatusCompleted,
			actual:      StatusReviewEnded,
			wantAllowed: true,
			wantCode:    "",
			wantReason:  "",
		},
		{
			name:        "finish on running review",
			action:      "finish",
			requires:    StatusReviewEnded,
			produces:    StatusCompleted,
			actual:      StatusReviewTaken,
			wantAllowed: false,
			wantCode:    FailureCodePreconditionUnmet,
			wantReason:  "precondition_unmet: action=finish requires=review_ended actual=review_taken",
		},
		{
			name:        "finish on completed",
			action:      "finish",
			requires:    StatusReviewEnded,
			produces:    StatusCompleted,
			actual:      StatusCompleted,
			wantAllowed: false,
			wantCode:    FailureCodePreconditionUnmet,
			wantReason:  "precondition_unmet: action=finish requires=review_ended actual=completed",
		},
		{
			name:        "unknown action",
			action:      "xyz",
			requires:    StatusAvailable,
			produces:    StatusTaken,
			actual:      StatusAvailable,
			wantAllowed: false,
			wantCode:    FailureCodeUnknownAction,
			wantReason:  "unknown_action: action=xyz",
		},
		{
			name:        "bad require enum",
			action:      "implement",
			requires:    Status("weird"),
			produces:    StatusTaken,
			actual:      StatusAvailable,
			wantAllowed: false,
			wantCode:    FailureCodeBadStatusInSchedule,
			wantReason:  "bad_status_in_schedule: action=implement requires=weird produces=taken",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := ScheduleRow{
				TaskID:   taskID,
				Action:   tc.action,
				Requires: StatusWrapper{TaskStatus: string(tc.requires)},
				Produces: StatusWrapper{TaskStatus: string(tc.produces)},
			}
			snap := snapshotWithTaskStatus(taskID, tc.actual)
			got := Evaluate(row, snap)
			if got.Allowed != tc.wantAllowed {
				t.Fatalf("Allowed mismatch: got=%v want=%v", got.Allowed, tc.wantAllowed)
			}
			if got.Code != tc.wantCode {
				t.Fatalf("Code mismatch: got=%q want=%q", got.Code, tc.wantCode)
			}
			if got.Reason != tc.wantReason {
				t.Fatalf("Reason mismatch:\n got=%q\nwant=%q", got.Reason, tc.wantReason)
			}
		})
	}
}

func TestPredict(t *testing.T) {
	row := ScheduleRow{Produces: StatusWrapper{TaskStatus: string(StatusReviewEnded)}}
	if got := Predict(row); got != StatusReviewEnded {
		t.Fatalf("Predict() = %q, want %q", got, StatusReviewEnded)
	}
}

func snapshotWithTaskStatus(taskID string, status Status) *StateSnapshot {
	s := &StateSnapshot{raw: rawState{Taken: map[string]takenEntry{}, Completed: map[string]completedEntry{}}}
	switch status {
	case StatusAvailable:
		// absent in both maps
	case StatusTaken:
		s.raw.Taken[taskID] = takenEntry{TakenBy: "phi", StartedAt: "2026-05-08 10:00"}
	case StatusReviewTaken:
		s.raw.Taken[taskID] = takenEntry{TakenBy: "phi", StartedAt: "2026-05-08 10:00", ReviewEnteredAt: "2026-05-08 10:01"}
	case StatusReviewEnded:
		s.raw.Taken[taskID] = takenEntry{TakenBy: "phi", StartedAt: "2026-05-08 10:00", ReviewEnteredAt: "2026-05-08 10:01", ReviewEndedAt: "2026-05-08 10:02"}
	case StatusCompleted:
		s.raw.Completed[taskID] = completedEntry{TakenBy: "phi", StartedAt: "2026-05-08 10:00", FinishedAt: "2026-05-08 10:03"}
	}
	return s
}
