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
	scheduleSchemaV2RelPath      = "docs/specs/swim-schedule-schema-v2.json"
	scheduleSchemaV3RelPath      = "docs/specs/swim-schedule-schema-v3.json"
	journalSchemaRelPath         = "docs/specs/swim-journal-schema-v1.json"
	reviewSidecarSchemaV1RelPath = "docs/specs/swim-review-schedule-schema-v1.json"
	reviewSidecarSchemaV2RelPath = "docs/specs/swim-review-schedule-schema-v2.json"
)

var (
	scheduleSchemaV2Once sync.Once
	scheduleSchemaV2     *jsonschema.Schema
	scheduleSchemaV2Err  error

	scheduleSchemaV3Once sync.Once
	scheduleSchemaV3     *jsonschema.Schema
	scheduleSchemaV3Err  error

	journalSchemaOnce sync.Once
	journalSchema     *jsonschema.Schema
	journalSchemaErr  error

	reviewSidecarSchemaV1Once sync.Once
	reviewSidecarSchemaV1     *jsonschema.Schema
	reviewSidecarSchemaV1Err  error

	reviewSidecarSchemaV2Once sync.Once
	reviewSidecarSchemaV2     *jsonschema.Schema
	reviewSidecarSchemaV2Err  error
)

var statusByAction = map[string]struct {
	requires string
	produces string
}{
	"implement":  {requires: "available", produces: "taken"},
	"review":     {requires: "taken", produces: "review_taken"},
	"fix":        {requires: "review_taken", produces: "taken"},
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
	Operation OperationSpec `json:"operation,omitempty"`
	Invoke   InvokeSpec    `json:"invoke"`
	Source   string        `json:"-"`
}

const (
	scheduleRowSourceReviewSidecar = "review_sidecar"
)

// StatusWrapper holds task_status state transitions.
type StatusWrapper struct {
	TaskStatus string `json:"task_status"`
}

// InvokeSpec is the canonical invocation payload.
type InvokeSpec struct {
	Argv []string `json:"argv"`
}

// OperationSpec is the typed execution contract for schema v3 schedule rows.
type OperationSpec struct {
	Kind     string `json:"kind"`
	Target   string `json:"target,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
}

func (o OperationSpec) IsZero() bool {
	return o.Kind == "" && o.Target == "" && o.Agent == "" && o.Reviewer == ""
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

// ReviewScheduleSidecar is the supplemental review-loop insertion contract.
type ReviewScheduleSidecar struct {
	SchemaVersion    int                       `json:"schema_version"`
	BaseSchedulePath string                    `json:"base_schedule_path"`
	Insertions       []ReviewScheduleInsertion `json:"insertions"`
}

// ReviewScheduleInsertion is one anchored supplemental row.
type ReviewScheduleInsertion struct {
	ID            string        `json:"id"`
	AfterStepID   string        `json:"after_step_id"`
	StepID        string        `json:"step_id"`
	SeqHint       int           `json:"seq_hint"`
	TaskID        string        `json:"task_id"`
	Action        string        `json:"action"`
	Requires      StatusWrapper `json:"requires"`
	Produces      StatusWrapper `json:"produces"`
	Operation     OperationSpec `json:"operation,omitempty"`
	Invoke        InvokeSpec    `json:"invoke"`
	Reason        string        `json:"reason"`
	SourceEventID string        `json:"source_event_id"`
}

func (r ScheduleRow) MarshalJSON() ([]byte, error) {
	type alias ScheduleRow
	if r.Operation.IsZero() {
		return json.Marshal(struct {
			Seq      int           `json:"seq"`
			StepID   string        `json:"step_id"`
			TaskID   string        `json:"task_id"`
			Action   string        `json:"action"`
			Requires StatusWrapper `json:"requires"`
			Produces StatusWrapper `json:"produces"`
			Invoke   InvokeSpec    `json:"invoke"`
		}{
			Seq:      r.Seq,
			StepID:   r.StepID,
			TaskID:   r.TaskID,
			Action:   r.Action,
			Requires: r.Requires,
			Produces: r.Produces,
			Invoke:   r.Invoke,
		})
	}
	return json.Marshal(alias(r))
}

func (r ReviewScheduleInsertion) MarshalJSON() ([]byte, error) {
	type alias ReviewScheduleInsertion
	if r.Operation.IsZero() {
		return json.Marshal(struct {
			ID            string        `json:"id"`
			AfterStepID   string        `json:"after_step_id"`
			StepID        string        `json:"step_id"`
			SeqHint       int           `json:"seq_hint"`
			TaskID        string        `json:"task_id"`
			Action        string        `json:"action"`
			Requires      StatusWrapper `json:"requires"`
			Produces      StatusWrapper `json:"produces"`
			Invoke        InvokeSpec    `json:"invoke"`
			Reason        string        `json:"reason"`
			SourceEventID string        `json:"source_event_id"`
		}{
			ID:            r.ID,
			AfterStepID:   r.AfterStepID,
			StepID:        r.StepID,
			SeqHint:       r.SeqHint,
			TaskID:        r.TaskID,
			Action:        r.Action,
			Requires:      r.Requires,
			Produces:      r.Produces,
			Invoke:        r.Invoke,
			Reason:        r.Reason,
			SourceEventID: r.SourceEventID,
		})
	}
	return json.Marshal(alias(r))
}

// LoadScheduleSchema compiles and caches the requested schedule schema version.
func LoadScheduleSchema(version int) (*jsonschema.Schema, error) {
	switch version {
	case 2:
		scheduleSchemaV2Once.Do(func() {
			scheduleSchemaV2, scheduleSchemaV2Err = compileSchemaFromResolvedPath(scheduleSchemaV2RelPath, "mem://swim-schedule-schema-v2.json")
		})
		return scheduleSchemaV2, scheduleSchemaV2Err
	case 3:
		scheduleSchemaV3Once.Do(func() {
			scheduleSchemaV3, scheduleSchemaV3Err = compileSchemaFromResolvedPath(scheduleSchemaV3RelPath, "mem://swim-schedule-schema-v3.json")
		})
		return scheduleSchemaV3, scheduleSchemaV3Err
	default:
		return nil, fmt.Errorf("unsupported schedule schema_version: %d", version)
	}
}

// LoadScheduleSchemaV2 is retained for call sites that need the legacy schema.
func LoadScheduleSchemaV2() (*jsonschema.Schema, error) {
	return LoadScheduleSchema(2)
}

// LoadScheduleSchemaV3 is retained for call sites that need the new schema.
func LoadScheduleSchemaV3() (*jsonschema.Schema, error) {
	return LoadScheduleSchema(3)
}

// LoadReviewSidecarSchema compiles and caches the requested review sidecar schema version.
func LoadReviewSidecarSchema(version int) (*jsonschema.Schema, error) {
	switch version {
	case 1:
		reviewSidecarSchemaV1Once.Do(func() {
			reviewSidecarSchemaV1, reviewSidecarSchemaV1Err = compileSchemaFromResolvedPath(reviewSidecarSchemaV1RelPath, "mem://swim-review-schedule-schema-v1.json")
		})
		return reviewSidecarSchemaV1, reviewSidecarSchemaV1Err
	case 2:
		reviewSidecarSchemaV2Once.Do(func() {
			reviewSidecarSchemaV2, reviewSidecarSchemaV2Err = compileSchemaFromResolvedPath(reviewSidecarSchemaV2RelPath, "mem://swim-review-schedule-schema-v2.json")
		})
		return reviewSidecarSchemaV2, reviewSidecarSchemaV2Err
	default:
		return nil, fmt.Errorf("unsupported review schedule schema_version: %d", version)
	}
}

// LoadJournalSchema compiles and caches docs/specs/swim-journal-schema-v1.json.
func LoadJournalSchema() (*jsonschema.Schema, error) {
	journalSchemaOnce.Do(func() {
		journalSchema, journalSchemaErr = compileSchemaFromResolvedPath(journalSchemaRelPath, "mem://swim-journal-schema-v1.json")
	})
	return journalSchema, journalSchemaErr
}

func LoadReviewSidecarSchemaV1() (*jsonschema.Schema, error) {
	return LoadReviewSidecarSchema(1)
}

func LoadReviewSidecarSchemaV2() (*jsonschema.Schema, error) {
	return LoadReviewSidecarSchema(2)
}

// ValidateSchedule validates schedule JSON using schema + strict structural invariants.
func ValidateSchedule(data []byte) error {
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return fmt.Errorf("schedule decode failed: %w", err)
	}

	sch, err := LoadScheduleSchema(header.SchemaVersion)
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
		if sched.SchemaVersion >= 3 {
			if err := validateOperationForAction(row); err != nil {
				return err
			}
			if err := validateInvokeAgainstOperation(row); err != nil {
				return err
			}
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

// ValidateReviewScheduleSidecar validates review sidecar JSON and anchor invariants against base.
func ValidateReviewScheduleSidecar(data []byte, base *Schedule) error {
	if base == nil {
		return fmt.Errorf("base schedule required for review sidecar validation")
	}

	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return fmt.Errorf("review schedule sidecar decode failed: %w", err)
	}

	sch, err := LoadReviewSidecarSchema(header.SchemaVersion)
	if err != nil {
		return err
	}
	if err := validateJSONAgainstSchema("review schedule sidecar", data, sch); err != nil {
		return err
	}

	var sidecar ReviewScheduleSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		return fmt.Errorf("review schedule sidecar decode failed: %w", err)
	}

	baseStepIDs := make(map[string]struct{}, len(base.Execution))
	seenStepIDs := make(map[string]int, len(base.Execution)+len(sidecar.Insertions))
	for i, row := range base.Execution {
		baseStepIDs[row.StepID] = struct{}{}
		seenStepIDs[row.StepID] = i
	}

	insByID := make(map[string]ReviewScheduleInsertion, len(sidecar.Insertions))
	insOrder := make([]string, 0, len(sidecar.Insertions))
	insIndex := make(map[string]int, len(sidecar.Insertions))
	for i, ins := range sidecar.Insertions {
		if prev, ok := insIndex[ins.ID]; ok {
			return fmt.Errorf("duplicate insertion id: %s (rows %d and %d)", ins.ID, prev, i)
		}
		insIndex[ins.ID] = i
		insByID[ins.ID] = ins
		insOrder = append(insOrder, ins.ID)

		if prev, ok := seenStepIDs[ins.StepID]; ok {
			return fmt.Errorf("duplicate step_id with base schedule: %s (base row=%d sidecar row=%d)", ins.StepID, prev, i)
		}
		if prev, ok := findSidecarStepIDDuplicate(sidecar.Insertions[:i], ins.StepID); ok {
			return fmt.Errorf("duplicate sidecar step_id: %s (rows %d and %d)", ins.StepID, prev, i)
		}

		if len(ins.Invoke.Argv) == 0 {
			return fmt.Errorf("malformed argv: row=%d has empty argv", i)
		}
		if strings.TrimSpace(ins.Invoke.Argv[0]) == "" {
			return fmt.Errorf("malformed argv: row=%d has empty argv[0]", i)
		}

		expected, ok := statusByAction[ins.Action]
		if !ok {
			return fmt.Errorf("invalid action: %q", ins.Action)
		}
		if ins.Requires.TaskStatus != expected.requires || ins.Produces.TaskStatus != expected.produces {
			return fmt.Errorf(
				"requires/produces mismatch for action %s: requires=%s produces=%s want requires=%s produces=%s",
				ins.Action,
				ins.Requires.TaskStatus,
				ins.Produces.TaskStatus,
				expected.requires,
				expected.produces,
			)
		}
		if sidecar.SchemaVersion >= 2 {
			row := ScheduleRow{
				Seq:       ins.SeqHint,
				StepID:    ins.StepID,
				TaskID:    ins.TaskID,
				Action:    ins.Action,
				Requires:  ins.Requires,
				Produces:  ins.Produces,
				Operation: ins.Operation,
				Invoke:    ins.Invoke,
			}
			if err := validateOperationForAction(row); err != nil {
				return err
			}
			if err := validateInvokeAgainstOperation(row); err != nil {
				return err
			}
		}
	}

	for i, ins := range sidecar.Insertions {
		if _, ok := baseStepIDs[ins.AfterStepID]; ok {
			continue
		}
		if _, ok := insByID[ins.AfterStepID]; ok {
			continue
		}
		return fmt.Errorf("unknown after_step_id %q at sidecar row=%d", ins.AfterStepID, i)
	}

	state := make(map[string]int, len(insByID)) // 0=unseen,1=visiting,2=done
	var dfs func(id string) error
	dfs = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("anchor cycle detected at insertion id %q", id)
		case 2:
			return nil
		}
		state[id] = 1
		anchor := insByID[id].AfterStepID
		if _, ok := insByID[anchor]; ok {
			if err := dfs(anchor); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for _, id := range insOrder {
		if err := dfs(id); err != nil {
			return err
		}
	}

	return nil
}

func findSidecarStepIDDuplicate(existing []ReviewScheduleInsertion, stepID string) (int, bool) {
	for i, row := range existing {
		if row.StepID == stepID {
			return i, true
		}
	}
	return 0, false
}

func validateOperationForAction(row ScheduleRow) error {
	switch row.Action {
	case "implement", "fix":
		if row.Operation.Kind != "agent_dispatch" {
			return fmt.Errorf("%s requires agent_dispatch operation", row.Action)
		}
		if row.Operation.Agent == "" || row.Operation.Target == "" {
			return fmt.Errorf("%s requires agent and target", row.Action)
		}
	case "review":
		if row.Operation.Kind != "agent_dispatch" {
			return fmt.Errorf("review requires agent_dispatch operation")
		}
		if row.Operation.Agent == "" || row.Operation.Target == "" || row.Operation.Reviewer == "" {
			return fmt.Errorf("review requires agent, target, and reviewer")
		}
	case "end_review", "finish":
		if row.Operation.Kind != "state_transition" {
			return fmt.Errorf("%s requires state_transition operation", row.Action)
		}
	default:
		return fmt.Errorf("invalid action: %q", row.Action)
	}
	return nil
}

func validateInvokeAgainstOperation(row ScheduleRow) error {
	argv := row.Invoke.Argv
	if len(argv) == 0 {
		return fmt.Errorf("malformed argv: row has empty argv")
	}
	if strings.TrimSpace(argv[0]) == "" {
		return fmt.Errorf("malformed argv: row has empty argv[0]")
	}
	if filepath.Base(argv[0]) != "wp-plan-step.sh" {
		return fmt.Errorf("invoke.argv contradicts operation: argv[0]=%q want wp-plan-step.sh", argv[0])
	}
	if value, ok := lookupFlagValue(argv, "--action"); !ok || value != row.Action {
		return fmt.Errorf("invoke.argv contradicts operation: --action mismatch for %s", row.Action)
	}
	if value, ok := lookupFlagValue(argv, "--task-id"); !ok || value != row.TaskID {
		return fmt.Errorf("invoke.argv contradicts operation: --task-id mismatch for %s", row.TaskID)
	}

	switch row.Operation.Kind {
	case "agent_dispatch":
		if value, ok := lookupFlagValue(argv, "--target"); !ok || value != row.Operation.Target {
			return fmt.Errorf("invoke.argv contradicts operation: --target mismatch for %s", row.TaskID)
		}
		if value, ok := lookupFlagValue(argv, "--agent"); !ok || value != row.Operation.Agent {
			return fmt.Errorf("invoke.argv contradicts operation: --agent mismatch for %s", row.TaskID)
		}
		if row.Action == "review" {
			if value, ok := lookupFlagValue(argv, "--reviewer"); !ok || value != row.Operation.Reviewer {
				return fmt.Errorf("invoke.argv contradicts operation: --reviewer mismatch for %s", row.TaskID)
			}
		} else if hasFlag(argv, "--reviewer") {
			return fmt.Errorf("invoke.argv contradicts operation: unexpected --reviewer for %s", row.Action)
		}
	case "state_transition":
		if hasFlag(argv, "--target") || hasFlag(argv, "--agent") || hasFlag(argv, "--reviewer") {
			return fmt.Errorf("invoke.argv contradicts operation: state_transition cannot carry dispatch flags")
		}
	default:
		return fmt.Errorf("invoke.argv contradicts operation: unsupported operation kind %q", row.Operation.Kind)
	}

	return nil
}

// BuildInvokeArgv derives the runtime argv for schema v3 rows and falls back to
// the stored invoke payload for legacy rows.
func BuildInvokeArgv(row ScheduleRow, planPath string) ([]string, error) {
	if row.Operation.Kind == "" {
		return append([]string(nil), row.Invoke.Argv...), nil
	}

	argv := []string{"wp-plan-step.sh", "--action", row.Action, "--plan", planPath, "--task-id", row.TaskID}
	switch row.Operation.Kind {
	case "agent_dispatch":
		argv = append(argv, "--target", row.Operation.Target, "--agent", row.Operation.Agent)
		if row.Action == "review" {
			argv = append(argv, "--reviewer", row.Operation.Reviewer)
		}
	case "state_transition":
		// No extra flags.
	default:
		return nil, fmt.Errorf("unsupported operation kind: %s", row.Operation.Kind)
	}
	return argv, nil
}

func hasFlag(argv []string, flag string) bool {
	_, ok := lookupFlagValue(argv, flag)
	return ok
}

func lookupFlagValue(argv []string, flag string) (string, bool) {
	for i := 0; i < len(argv); i++ {
		if argv[i] != flag {
			continue
		}
		if i+1 >= len(argv) {
			return "", false
		}
		return argv[i+1], true
	}
	return "", false
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

func compileSchemaFromResolvedPath(relPath, resourceURL string) (*jsonschema.Schema, error) {
	abs, err := resolveSchemaPath(relPath)
	if err != nil {
		return nil, err
	}
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

func resolveSchemaPath(relPath string) (string, error) {
	name := filepath.Base(relPath)
	if exePath, err := os.Executable(); err == nil && exePath != "" {
		shareCandidate := filepath.Join(filepath.Dir(filepath.Dir(exePath)), "share", "waveplan", "specs", name)
		if _, err := os.Stat(shareCandidate); err == nil {
			return shareCandidate, nil
		}
		siblingCandidate := filepath.Join(filepath.Dir(exePath), name)
		if _, err := os.Stat(siblingCandidate); err == nil {
			return siblingCandidate, nil
		}
	}
	// Repo root takes priority over user share so that `go test` always uses
	// the checked-in schema rather than a potentially stale installed copy.
	if root, err := findRepoRoot(); err == nil {
		abs := filepath.Join(root, relPath)
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		userShare := filepath.Join(home, ".local", "share", "waveplan", "specs", name)
		if _, err := os.Stat(userShare); err == nil {
			return userShare, nil
		}
	}
	return "", fmt.Errorf("schema %s not found in install share dir or repo root", relPath)
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
