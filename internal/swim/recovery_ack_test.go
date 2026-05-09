package swim

import (
	"path/filepath"
	"testing"
)

func TestAckUnknown_PromotesToFailed(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "journal.json")

	writeJournal(t, journalPath, Journal{
		SchemaVersion: 1,
		SchedulePath:  "docs/plans/test.json",
		Cursor:        0,
		Events: []JournalEvent{
			{
				EventID:     "E0001",
				StepID:      "S1_T1.1_implement",
				Seq:         1,
				TaskID:      "T1.1",
				Action:      "implement",
				Attempt:     1,
				StartedOn:   "2026-05-09T00:00:00Z",
				CompletedOn: "2026-05-09T00:00:01Z",
				Outcome:     "unknown",
				StateBefore: StatusWrapper{TaskStatus: string(StatusAvailable)},
				StateAfter:  StatusWrapper{TaskStatus: string(StatusTaken)},
			},
		},
	})

	if err := AckUnknown(journalPath, "S1_T1.1_implement", "failed"); err != nil {
		t.Fatalf("AckUnknown: %v", err)
	}
	j := readJournal(t, journalPath)
	if got := j.Events[0].Outcome; got != "failed" {
		t.Fatalf("outcome = %q, want failed", got)
	}
}

func TestAckUnknown_PromotesToWaived(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "journal.json")

	writeJournal(t, journalPath, Journal{
		SchemaVersion: 1,
		SchedulePath:  "docs/plans/test.json",
		Cursor:        0,
		Events: []JournalEvent{
			{
				EventID:     "E0001",
				StepID:      "S1_T1.1_implement",
				Seq:         1,
				TaskID:      "T1.1",
				Action:      "implement",
				Attempt:     1,
				StartedOn:   "2026-05-09T00:00:00Z",
				CompletedOn: "2026-05-09T00:00:01Z",
				Outcome:     "unknown",
				StateBefore: StatusWrapper{TaskStatus: string(StatusAvailable)},
				StateAfter:  StatusWrapper{TaskStatus: string(StatusTaken)},
			},
		},
	})

	if err := AckUnknown(journalPath, "S1_T1.1_implement", "waived"); err != nil {
		t.Fatalf("AckUnknown: %v", err)
	}
	j := readJournal(t, journalPath)
	if got := j.Events[0].Outcome; got != "waived" {
		t.Fatalf("outcome = %q, want waived", got)
	}
	if j.Events[0].Operator == "" || j.Events[0].Reason == "" || j.Events[0].WaivedOn == "" {
		t.Fatal("waived event missing required operator metadata")
	}
}
