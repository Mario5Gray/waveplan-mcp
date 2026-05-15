package swim

import "fmt"

// NextOptions resolves the current cursor step from schedule/journal/state paths.
type NextOptions struct {
	SchedulePath       string
	ReviewSchedulePath string
	JournalPath        string
	StatePath          string
}

// JournalView is a read-only journal inspection payload.
type JournalView struct {
	Cursor             int            `json:"cursor"`
	LastEvent          *JournalEvent  `json:"last_event,omitempty"`
	Events             []JournalEvent `json:"events"`
	MergedExecution    []ScheduleRow  `json:"merged_execution,omitempty"`
}

// ResolveNextFromPaths loads schedule, journal, and state and returns the current Decision.
func ResolveNextFromPaths(opts NextOptions) (*Decision, error) {
	if opts.SchedulePath == "" {
		return nil, fmt.Errorf("missing schedule path")
	}
	if opts.JournalPath == "" {
		return nil, fmt.Errorf("missing journal path")
	}
	if opts.StatePath == "" {
		return nil, fmt.Errorf("missing state path")
	}

	schedule, err := loadSchedule(opts.SchedulePath, opts.ReviewSchedulePath)
	if err != nil {
		return nil, err
	}
	journal, err := loadOrInitJournal(opts.JournalPath, opts.SchedulePath)
	if err != nil {
		return nil, err
	}
	snap, err := ReadStateSnapshotOrEmpty(opts.StatePath)
	if err != nil {
		return nil, err
	}
	decision := ResolveNext(schedule, journal, snap)
	return &decision, nil
}

// ReadJournalView loads the journal and optionally tails the final N events.
// When schedulePath is provided, uses it for schedule context and sidecar merging (does not defer to journal.SchedulePath).
// When reviewSchedulePath is provided, validates and merges the sidecar into execution.
func ReadJournalView(journalPath, schedulePath, reviewSchedulePath string, tail int) (*JournalView, error) {
	if journalPath == "" {
		return nil, fmt.Errorf("missing journal path")
	}
	journal, err := loadOrInitJournal(journalPath, schedulePath)
	if err != nil {
		return nil, err
	}
	var mergedExecution []ScheduleRow
	if schedulePath != "" {
		schedule, err := loadSchedule(schedulePath, reviewSchedulePath)
		if err != nil {
			return nil, err
		}
		if journal.Cursor > len(schedule.Execution) {
			return nil, fmt.Errorf("journal cursor %d exceeds execution length %d (possible schedule mismatch)", journal.Cursor, len(schedule.Execution))
		}
		mergedExecution = schedule.Execution
	}
	events := journal.Events
	if events == nil {
		events = []JournalEvent{}
	} else if tail > 0 && tail < len(events) {
		events = append([]JournalEvent(nil), events[len(events)-tail:]...)
	} else if tail > 0 {
		events = append([]JournalEvent(nil), events...)
	}
	return &JournalView{
		Cursor:          journal.Cursor,
		LastEvent:       journal.LastEvent,
		Events:          events,
		MergedExecution: mergedExecution,
	}, nil
}
