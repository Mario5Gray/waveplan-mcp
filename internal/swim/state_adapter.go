package swim

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Status is the canonical runtime-state value for a task.
type Status string

const (
	StatusAvailable   Status = "available"
	StatusTaken       Status = "taken"
	StatusReviewTaken Status = "review_taken"
	StatusReviewEnded Status = "review_ended"
	StatusCompleted   Status = "completed"
)

// takenEntry mirrors a single value in state.taken.
type takenEntry struct {
	TakenBy         string `json:"taken_by"`
	StartedAt       string `json:"started_at"`
	ReviewEnteredAt string `json:"review_entered_at,omitempty"`
	ReviewEndedAt   string `json:"review_ended_at,omitempty"`
	Reviewer        string `json:"reviewer,omitempty"`
}

// completedEntry mirrors a single value in state.completed.
type completedEntry struct {
	TakenBy         string `json:"taken_by"`
	StartedAt       string `json:"started_at"`
	ReviewEnteredAt string `json:"review_entered_at,omitempty"`
	ReviewEndedAt   string `json:"review_ended_at,omitempty"`
	Reviewer        string `json:"reviewer,omitempty"`
	FinishedAt      string `json:"finished_at,omitempty"`
}

// rawState mirrors the on-disk waveplan state file.
type rawState struct {
	Plan      string                    `json:"plan"`
	Taken     map[string]takenEntry     `json:"taken"`
	Completed map[string]completedEntry `json:"completed"`
}

// StateSnapshot is an immutable, in-memory view of a state file.
// Token() returns a deterministic content hash usable as a CAS sentinel.
type StateSnapshot struct {
	raw       rawState
	canonBody []byte // canonical-ordered re-serialization for Token()
	warnings  []string
}

// ReadStateSnapshot loads and parses a state JSON file from disk.
func ReadStateSnapshot(path string) (*StateSnapshot, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state file %q: %w", path, err)
	}
	return ReadStateSnapshotBytes(body)
}

// ReadStateSnapshotOrEmpty is the operator-facing variant used by SWIM CLI/MCP
// flows. A missing file is treated as a clean empty state so first-run `next`
// / `step` / `run --dry-run` can operate without an explicit bootstrap step.
func ReadStateSnapshotOrEmpty(path string) (*StateSnapshot, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ReadStateSnapshotBytes([]byte(fmt.Sprintf(`{"plan":%q,"taken":{},"completed":{}}`, filepath.Base(path))))
		}
		return nil, fmt.Errorf("read state file %q: %w", path, err)
	}
	return ReadStateSnapshotBytes(body)
}

// ReadStateSnapshotBytes parses a state JSON document already in memory.
func ReadStateSnapshotBytes(body []byte) (*StateSnapshot, error) {
	var raw rawState
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse state JSON: %w", err)
	}
	if raw.Taken == nil {
		raw.Taken = map[string]takenEntry{}
	}
	if raw.Completed == nil {
		raw.Completed = map[string]completedEntry{}
	}

	s := &StateSnapshot{raw: raw}
	s.canonBody = canonicalize(raw)
	s.collectWarnings()
	return s, nil
}

// StatusOf returns the canonical runtime status for taskID.
//
// Resolution order (top wins; terminal beats in-flight):
//  1. Completed map with finished_at set → completed
//  2. Taken map with review_ended_at set → review_ended
//  3. Taken map with review_entered_at set, no review_ended_at → review_taken
//  4. Taken map with no review fields → taken
//  5. Otherwise → available
//
// review_taken and review_ended derive from timestamp presence per the locked
// decision: the state schema is not extended with explicit boolean flags.
func (s *StateSnapshot) StatusOf(taskID string) Status {
	if c, ok := s.raw.Completed[taskID]; ok && c.FinishedAt != "" {
		return StatusCompleted
	}
	if t, ok := s.raw.Taken[taskID]; ok {
		switch {
		case t.ReviewEndedAt != "":
			return StatusReviewEnded
		case t.ReviewEnteredAt != "":
			return StatusReviewTaken
		default:
			return StatusTaken
		}
	}
	return StatusAvailable
}

// Token returns a deterministic SHA-256 hex digest of the canonical
// re-serialization. Two snapshots produced from byte-identical state files
// produce identical tokens; any field change produces a different token.
//
// Used by the apply-time race-closure protocol (T2.5) as the CAS sentinel:
// snapshot A is taken before invoke, snapshot B is re-read just before
// running argv, and execution proceeds only if Token(A) == Token(B).
func (s *StateSnapshot) Token() string {
	sum := sha256.Sum256(s.canonBody)
	return hex.EncodeToString(sum[:])
}

// Warnings returns non-fatal observations about the state file (e.g. a task
// present in both taken and completed). Empty slice when clean.
func (s *StateSnapshot) Warnings() []string {
	out := make([]string, len(s.warnings))
	copy(out, s.warnings)
	return out
}

func (s *StateSnapshot) collectWarnings() {
	for tid := range s.raw.Taken {
		if _, both := s.raw.Completed[tid]; both {
			s.warnings = append(s.warnings,
				fmt.Sprintf("task %s present in both taken and completed; completed wins (terminal)", tid))
		}
	}
	sort.Strings(s.warnings)
}

// canonicalize re-serializes rawState with sorted map keys so identical
// content (regardless of input ordering) produces byte-identical output.
// Go's encoding/json already sorts map keys deterministically, but we wrap
// the rawState in a known struct shape so future field additions don't
// silently change the canonical form.
func canonicalize(r rawState) []byte {
	type canonical struct {
		Plan      string                    `json:"plan"`
		Taken     map[string]takenEntry     `json:"taken"`
		Completed map[string]completedEntry `json:"completed"`
	}
	c := canonical{
		Plan:      r.Plan,
		Taken:     r.Taken,
		Completed: r.Completed,
	}
	body, err := json.Marshal(c)
	if err != nil {
		// Should not happen on a parsed input. Fall back to empty token input.
		return nil
	}
	return body
}

// AdvanceStateSnapshot applies a canonical task-status transition to the SWIM
// state file, creating the file when absent. The state file is SWIM-owned
// execution state and is updated after successful step application so future
// resolver decisions do not depend on external tools mutating it.
func AdvanceStateSnapshot(path, planRef, taskID string, status Status, now time.Time) error {
	s, err := ReadStateSnapshotOrEmpty(path)
	if err != nil {
		return err
	}
	raw := s.raw
	if raw.Plan == "" {
		raw.Plan = planRef
	}
	if raw.Taken == nil {
		raw.Taken = map[string]takenEntry{}
	}
	if raw.Completed == nil {
		raw.Completed = map[string]completedEntry{}
	}

	applyStateTransition(&raw, taskID, status, now.UTC().Format(time.RFC3339))

	body, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state file %q: %w", path, err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fmt.Errorf("write state file %q: %w", path, err)
	}
	return nil
}

func applyStateTransition(raw *rawState, taskID string, status Status, ts string) {
	if raw == nil {
		return
	}
	if raw.Taken == nil {
		raw.Taken = map[string]takenEntry{}
	}
	if raw.Completed == nil {
		raw.Completed = map[string]completedEntry{}
	}

	taken := raw.Taken[taskID]
	if completed, ok := raw.Completed[taskID]; ok {
		if taken.TakenBy == "" {
			taken.TakenBy = completed.TakenBy
		}
		if taken.StartedAt == "" {
			taken.StartedAt = completed.StartedAt
		}
		if taken.Reviewer == "" {
			taken.Reviewer = completed.Reviewer
		}
		if taken.ReviewEnteredAt == "" {
			taken.ReviewEnteredAt = completed.ReviewEnteredAt
		}
		if taken.ReviewEndedAt == "" {
			taken.ReviewEndedAt = completed.ReviewEndedAt
		}
	}
	if taken.TakenBy == "" {
		taken.TakenBy = "swim"
	}
	if taken.StartedAt == "" {
		taken.StartedAt = ts
	}

	switch status {
	case StatusAvailable:
		delete(raw.Taken, taskID)
		delete(raw.Completed, taskID)
	case StatusTaken:
		taken.ReviewEnteredAt = ""
		taken.ReviewEndedAt = ""
		taken.Reviewer = ""
		raw.Taken[taskID] = taken
		delete(raw.Completed, taskID)
	case StatusReviewTaken:
		if taken.ReviewEnteredAt == "" {
			taken.ReviewEnteredAt = ts
		}
		taken.ReviewEndedAt = ""
		if taken.Reviewer == "" {
			taken.Reviewer = "swim"
		}
		raw.Taken[taskID] = taken
		delete(raw.Completed, taskID)
	case StatusReviewEnded:
		if taken.ReviewEnteredAt == "" {
			taken.ReviewEnteredAt = ts
		}
		if taken.Reviewer == "" {
			taken.Reviewer = "swim"
		}
		taken.ReviewEndedAt = ts
		raw.Taken[taskID] = taken
		delete(raw.Completed, taskID)
	case StatusCompleted:
		completed := completedEntry{
			TakenBy:         taken.TakenBy,
			StartedAt:       taken.StartedAt,
			ReviewEnteredAt: taken.ReviewEnteredAt,
			ReviewEndedAt:   taken.ReviewEndedAt,
			Reviewer:        taken.Reviewer,
			FinishedAt:      ts,
		}
		raw.Completed[taskID] = completed
		delete(raw.Taken, taskID)
	}
}
