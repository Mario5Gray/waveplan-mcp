package model

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// TaskStatus is the lifecycle status used by waveplan state and SWIM rows.
type TaskStatus string

const (
	StatusAvailable   TaskStatus = "available"
	StatusTaken       TaskStatus = "taken"
	StatusReviewTaken TaskStatus = "review_taken"
	StatusReviewEnded TaskStatus = "review_ended"
	StatusCompleted   TaskStatus = "completed"
)

// StateFile is a waveplan .state.json sidecar.
type StateFile struct {
	Plan      string               `json:"plan"`
	Taken     map[string]TaskEntry `json:"taken"`
	Completed map[string]TaskEntry `json:"completed"`
	Tail      map[string]TaskEntry `json:"tail"`
}

// TaskEntry records lifecycle metadata for a unit.
type TaskEntry struct {
	TakenBy         string `json:"taken_by"`
	StartedAt       string `json:"started_at"`
	ReviewEnteredAt string `json:"review_entered_at"`
	ReviewEndedAt   string `json:"review_ended_at"`
	Reviewer        string `json:"reviewer"`
	ReviewNote      string `json:"review_note,omitempty"`
	GitSHA          string `json:"git_sha,omitempty"`
	FinishedAt      string `json:"finished_at"`
}

// StatusWrapper is used by SWIM schedules and journals.
type StatusWrapper struct {
	TaskStatus TaskStatus `json:"task_status"`
}

// LoadState reads a waveplan state file from path.
func LoadState(path string) (*StateFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open state %q: %w", path, err)
	}
	defer file.Close()

	state, err := DecodeState(file)
	if err != nil {
		return nil, fmt.Errorf("decode state %q: %w", path, err)
	}
	return state, nil
}

// DecodeState decodes a waveplan state file from r.
func DecodeState(r io.Reader) (*StateFile, error) {
	var state StateFile
	if err := json.NewDecoder(r).Decode(&state); err != nil {
		return nil, err
	}
	if state.Taken == nil {
		state.Taken = map[string]TaskEntry{}
	}
	if state.Completed == nil {
		state.Completed = map[string]TaskEntry{}
	}
	if state.Tail == nil {
		state.Tail = map[string]TaskEntry{}
	}
	return &state, nil
}

// StatusOf derives a unit's lifecycle state from completed and taken maps.
func (s *StateFile) StatusOf(taskID string) TaskStatus {
	if s == nil {
		return StatusAvailable
	}
	if _, ok := s.Completed[taskID]; ok {
		return StatusCompleted
	}
	entry, ok := s.Taken[taskID]
	if !ok {
		return StatusAvailable
	}
	if entry.ReviewEndedAt != "" {
		return StatusReviewEnded
	}
	if entry.ReviewEnteredAt != "" {
		return StatusReviewTaken
	}
	return StatusTaken
}
