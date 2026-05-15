package model

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// ReviewScheduleFile is the review-loop sidecar consumed alongside a base schedule.
type ReviewScheduleFile struct {
	SchemaVersion    int                       `json:"schema_version"`
	BaseSchedulePath string                    `json:"base_schedule_path"`
	Insertions       []ReviewScheduleInsertion `json:"insertions"`
}

// ReviewScheduleInsertion is one sidecar insertion row.
type ReviewScheduleInsertion struct {
	ID            string        `json:"id"`
	AfterStepID   string        `json:"after_step_id"`
	StepID        string        `json:"step_id"`
	SeqHint       int           `json:"seq_hint"`
	TaskID        string        `json:"task_id"`
	Action        string        `json:"action"`
	Requires      StatusWrapper `json:"requires"`
	Produces      StatusWrapper `json:"produces"`
	Reason        string        `json:"reason"`
	SourceEventID string        `json:"source_event_id"`
}

// LoadReviewSchedule reads a review sidecar file from path.
func LoadReviewSchedule(path string) (*ReviewScheduleFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open review schedule %q: %w", path, err)
	}
	defer file.Close()

	reviewSchedule, err := DecodeReviewSchedule(file)
	if err != nil {
		return nil, fmt.Errorf("decode review schedule %q: %w", path, err)
	}
	return reviewSchedule, nil
}

// DecodeReviewSchedule decodes a review sidecar from r.
func DecodeReviewSchedule(r io.Reader) (*ReviewScheduleFile, error) {
	var reviewSchedule ReviewScheduleFile
	if err := json.NewDecoder(r).Decode(&reviewSchedule); err != nil {
		return nil, err
	}
	if reviewSchedule.Insertions == nil {
		reviewSchedule.Insertions = []ReviewScheduleInsertion{}
	}
	return &reviewSchedule, nil
}
