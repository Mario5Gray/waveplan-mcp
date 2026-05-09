package swim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

const (
	scheduleSchemaRelPath = "docs/specs/swim-schedule-schema-v2.json"
	journalSchemaRelPath  = "docs/specs/swim-journal-schema-v1.json"
)

var (
	scheduleSchemaOnce sync.Once
	scheduleSchema     *jsonschema.Schema
	scheduleSchemaErr  error

	journalSchemaOnce sync.Once
	journalSchema     *jsonschema.Schema
	journalSchemaErr  error
)

var statusByAction = map[string]struct {
	requires string
	produces string
}{
	"implement":  {requires: "available", produces: "taken"},
	"review":     {requires: "taken", produces: "review_taken"},
	"end_review": {requires: "review_taken", produces: "review_ended"},
	"finish":     {requires: "review_ended", produces: "completed"},
}

// Schedule is the emitted SWIM schedule contract.
type Schedule struct {
	SchemaVersion int           `json:"schema_version"`
	Execution     []ScheduleRow `json:"execution"`
}

// ScheduleRow is one executable step in schedule execution[].
type ScheduleRow struct {
	Seq      int           `json:"seq"`
	StepID   string        `json:"step_id"`
	TaskID   string        `json:"task_id"`
	Action   string        `json:"action"`
	Requires StatusWrapper `json:"requires"`
	Produces StatusWrapper `json:"produces"`
	Invoke   InvokeSpec    `json:"invoke"`
}

// StatusWrapper holds task_status state transitions.
type StatusWrapper struct {
	TaskStatus string `json:"task_status"`
}

// InvokeSpec is the canonical invocation payload.
type InvokeSpec struct {
	Argv []string `json:"argv"`
}

// Journal is the SWIM execution journal sidecar contract.
type Journal struct {
	SchemaVersion int            `json:"schema_version"`
	SchedulePath  string         `json:"schedule_path"`
	Cursor        int            `json:"cursor"`
	LastEvent     *JournalEvent  `json:"last_event,omitempty"`
	Events        []JournalEvent `json:"events"`
}

// JournalEvent is one append-only event row in a journal.
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

// LoadScheduleSchema compiles and caches docs/specs/swim-schedule-schema-v2.json.
func LoadScheduleSchema() (*jsonschema.Schema, error) {
	scheduleSchemaOnce.Do(func() {
		scheduleSchema, scheduleSchemaErr = compileSchemaFromRepoPath(scheduleSchemaRelPath, "mem://swim-schedule-schema-v2.json")
	})
	return scheduleSchema, scheduleSchemaErr
}

// LoadJournalSchema compiles and caches docs/specs/swim-journal-schema-v1.json.
func LoadJournalSchema() (*jsonschema.Schema, error) {
	journalSchemaOnce.Do(func() {
		journalSchema, journalSchemaErr = compileSchemaFromRepoPath(journalSchemaRelPath, "mem://swim-journal-schema-v1.json")
	})
	return journalSchema, journalSchemaErr
}

// ValidateSchedule validates schedule JSON using schema + strict structural invariants.
func ValidateSchedule(data []byte) error {
	sch, err := LoadScheduleSchema()
	if err != nil {
		return err
	}
	if err := validateJSONAgainstSchema("schedule", data, sch); err != nil {
		return err
	}

	var sched Schedule
	if err := json.Unmarshal(data, &sched); err != nil {
		return fmt.Errorf("schedule decode failed: %w", err)
	}

	seenStepIDs := map[string]int{}
	expectedSeq := 1
	for i, row := range sched.Execution {
		if row.Seq != expectedSeq {
			return fmt.Errorf("seq not strictly increasing from 1: row=%d got=%d want=%d", i, row.Seq, expectedSeq)
		}
		expectedSeq++

		if prev, ok := seenStepIDs[row.StepID]; ok {
			return fmt.Errorf("duplicate step_id: %s (rows %d and %d)", row.StepID, prev, i)
		}
		seenStepIDs[row.StepID] = i

		if len(row.Invoke.Argv) == 0 {
			return fmt.Errorf("malformed argv: row=%d has empty argv", i)
		}
		if strings.TrimSpace(row.Invoke.Argv[0]) == "" {
			return fmt.Errorf("malformed argv: row=%d has empty argv[0]", i)
		}

		expected, ok := statusByAction[row.Action]
		if !ok {
			return fmt.Errorf("invalid action: %q", row.Action)
		}
		if row.Requires.TaskStatus != expected.requires || row.Produces.TaskStatus != expected.produces {
			return fmt.Errorf(
				"requires/produces mismatch for action %s: requires=%s produces=%s want requires=%s produces=%s",
				row.Action,
				row.Requires.TaskStatus,
				row.Produces.TaskStatus,
				expected.requires,
				expected.produces,
			)
		}
	}

	return nil
}

// ValidateJournal validates journal JSON using schema + strict structural invariants.
func ValidateJournal(data []byte) error {
	sch, err := LoadJournalSchema()
	if err != nil {
		return err
	}
	if err := validateJSONAgainstSchema("journal", data, sch); err != nil {
		return err
	}

	var journal Journal
	if err := json.Unmarshal(data, &journal); err != nil {
		return fmt.Errorf("journal decode failed: %w", err)
	}

	seenEventIDs := map[string]struct{}{}
	for i, event := range journal.Events {
		if _, exists := seenEventIDs[event.EventID]; exists {
			return fmt.Errorf("duplicate event_id: %s", event.EventID)
		}
		seenEventIDs[event.EventID] = struct{}{}

		if event.Outcome == "waived" {
			if strings.TrimSpace(event.Operator) == "" || strings.TrimSpace(event.Reason) == "" || strings.TrimSpace(event.WaivedOn) == "" {
				return fmt.Errorf("waived outcome requires operator, reason, waived_on (row=%d)", i)
			}
		}
	}

	return nil
}

func validateJSONAgainstSchema(kind string, data []byte, sch *jsonschema.Schema) error {
	var decoded any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return fmt.Errorf("%s decode failed: %w", kind, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%s decode failed: extra trailing JSON payload", kind)
		}
		return fmt.Errorf("%s decode failed: %w", kind, err)
	}
	if err := sch.Validate(decoded); err != nil {
		return fmt.Errorf("%s schema validation failed: %w", kind, err)
	}
	return nil
}

func compileSchemaFromRepoPath(relPath, resourceURL string) (*jsonschema.Schema, error) {
	root, err := findRepoRoot()
	if err != nil {
		return nil, err
	}
	abs := filepath.Join(root, relPath)
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read schema %s: %w", abs, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat = true
	if err := compiler.AddResource(resourceURL, bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("add schema resource %s: %w", relPath, err)
	}
	sch, err := compiler.Compile(resourceURL)
	if err != nil {
		return nil, fmt.Errorf("compile schema %s: %w", relPath, err)
	}
	return sch, nil
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	for {
		candidate := filepath.Join(wd, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return "", fmt.Errorf("could not locate repo root (go.mod) from current working directory")
}
