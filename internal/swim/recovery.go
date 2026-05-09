package swim

import (
	"fmt"
	"os"
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

// AckUnknown converts the latest unknown event for stepID into either failed or waived.
func AckUnknown(journalPath, stepID, outcome string) error {
	if stepID == "" {
		return fmt.Errorf("missing step_id")
	}
	if outcome != "failed" && outcome != "waived" {
		return fmt.Errorf("invalid outcome %q", outcome)
	}

	journal, err := loadOrInitJournal(journalPath, "")
	if err != nil {
		return err
	}

	var target *JournalEvent
	for i := len(journal.Events) - 1; i >= 0; i-- {
		if journal.Events[i].StepID == stepID && journal.Events[i].Outcome == "unknown" {
			target = &journal.Events[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("unknown step not found: %s", stepID)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	target.Outcome = outcome
	target.CompletedOn = now
	if outcome == "waived" {
		target.Operator = ackOperator()
		target.Reason = "operator_ack_unknown"
		target.WaivedOn = now
	}

	last := journal.Events[len(journal.Events)-1]
	journal.LastEvent = &last
	return saveJournal(journalPath, journal)
}

func ackOperator() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	return "waveplan-cli"
}
