package model

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// LogStream identifies a SWIM stdout or stderr log stream.
type LogStream string

const (
	LogStreamStdout LogStream = "stdout"
	LogStreamStderr LogStream = "stderr"
)

var logFilePattern = regexp.MustCompile(`^(S[0-9]+_[A-Za-z0-9.\-]+_(?:implement|end_review|finish|review(?:_r[0-9]+)?|fix(?:_r[0-9]+)?))\.([1-9][0-9]*)\.(stdout|stderr)\.log$`)

// LogRef identifies a SWIM step log file by filename.
type LogRef struct {
	Path    string    `json:"path"`
	StepID  string    `json:"step_id"`
	Attempt int       `json:"attempt"`
	Stream  LogStream `json:"stream"`
}

// Journal is the SWIM execution journal sidecar.
type Journal struct {
	SchemaVersion int            `json:"schema_version"`
	SchedulePath  string         `json:"schedule_path"`
	Cursor        int            `json:"cursor"`
	LastEvent     *JournalEvent  `json:"last_event,omitempty"`
	Events        []JournalEvent `json:"events"`
}

// JournalEvent is one append-only SWIM event row.
type JournalEvent struct {
	EventID     string        `json:"event_id"`
	StepID      string        `json:"step_id"`
	Seq         int           `json:"seq"`
	TaskID      string        `json:"task_id"`
	Action      string        `json:"action"`
	Attempt     int           `json:"attempt"`
	StartedOn   string        `json:"started_on"`
	CompletedOn string        `json:"completed_on,omitempty"`
	Outcome     string        `json:"outcome,omitempty"`
	StateBefore StatusWrapper `json:"state_before"`
	StateAfter  StatusWrapper `json:"state_after"`
	ExitCode    *int          `json:"exit_code,omitempty"`
	StdoutPath  string        `json:"stdout_path,omitempty"`
	StderrPath  string        `json:"stderr_path,omitempty"`
	Operator    string        `json:"operator,omitempty"`
	Reason      string        `json:"reason,omitempty"`
	WaivedOn    string        `json:"waived_on,omitempty"`
}

// LoadJournal reads a SWIM journal from path.
func LoadJournal(path string) (*Journal, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open journal %q: %w", path, err)
	}
	defer file.Close()

	journal, err := DecodeJournal(file)
	if err != nil {
		return nil, fmt.Errorf("decode journal %q: %w", path, err)
	}
	return journal, nil
}

// DecodeJournal decodes a SWIM journal from r.
func DecodeJournal(r io.Reader) (*Journal, error) {
	var journal Journal
	if err := json.NewDecoder(r).Decode(&journal); err != nil {
		return nil, err
	}
	if journal.Events == nil {
		journal.Events = []JournalEvent{}
	}
	return &journal, nil
}

// ParseLogPath extracts the SWIM step ID, attempt, and stream from a log path.
func ParseLogPath(path string) (*LogRef, error) {
	name := filepath.Base(path)
	matches := logFilePattern.FindStringSubmatch(name)
	if matches == nil {
		return nil, fmt.Errorf("invalid log filename %q", name)
	}
	attempt, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, fmt.Errorf("invalid log attempt in %q: %w", name, err)
	}
	return &LogRef{
		Path:    path,
		StepID:  matches[1],
		Attempt: attempt,
		Stream:  LogStream(matches[3]),
	}, nil
}
