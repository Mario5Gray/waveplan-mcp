package swim

import "fmt"

// NextOptions resolves the current cursor step from schedule/journal/state paths.
type NextOptions struct {
	SchedulePath string
	JournalPath  string
	StatePath    string
}

// JournalView is a read-only journal inspection payload.
type JournalView struct {
	Cursor    int            `json:"cursor"`
	LastEvent *JournalEvent  `json:"last_event,omitempty"`
	Events    []JournalEvent `json:"events"`
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

	schedule, err := loadSchedule(opts.SchedulePath)
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
func ReadJournalView(journalPath, schedulePath string, tail int) (*JournalView, error) {
	if journalPath == "" {
		return nil, fmt.Errorf("missing journal path")
	}
	_ = schedulePath
	journal, err := loadOrInitJournal(journalPath, "")
	if err != nil {
		return nil, err
	}
	events := journal.Events
	if tail > 0 && tail < len(events) {
		events = append([]JournalEvent(nil), events[len(events)-tail:]...)
	} else {
		events = append([]JournalEvent(nil), events...)
	}
	return &JournalView{
		Cursor:    journal.Cursor,
		LastEvent: journal.LastEvent,
		Events:    events,
	}, nil
}
