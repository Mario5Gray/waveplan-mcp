package swim

import (
	"time"
)

// DetectAndMarkUnknown promotes orphan in-flight events to terminal unknown.
// Orphan signature: started_on set, outcome empty, completed_on empty.
func DetectAndMarkUnknown(journalPath string) (int, error) {
	journal, err := loadOrInitJournal(journalPath, "")
	if err != nil {
		return 0, err
	}
	count := 0
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range journal.Events {
		e := &journal.Events[i]
		if e.StartedOn != "" && e.Outcome == "" && e.CompletedOn == "" {
			e.Outcome = "unknown"
			e.CompletedOn = now
			count++
		}
	}
	if count > 0 {
		last := journal.Events[len(journal.Events)-1]
		journal.LastEvent = &last
		if err := saveJournal(journalPath, journal); err != nil {
			return 0, err
		}
	}
	return count, nil
}
