package swim

import (
	"path/filepath"
	"testing"
)

func TestDetectAndMarkUnknown_MarksOrphanEvents(t *testing.T) {
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
				StartedOn:   "2026-05-08T00:00:00Z",
				StateBefore: StatusWrapper{TaskStatus: string(StatusAvailable)},
				StateAfter:  StatusWrapper{TaskStatus: string(StatusTaken)},
			},
		},
	})

	count, err := DetectAndMarkUnknown(journalPath)
	if err != nil {
		t.Fatalf("DetectAndMarkUnknown: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	j := readJournal(t, journalPath)
	if got := j.Events[0].Outcome; got != "unknown" {
		t.Fatalf("outcome = %q, want unknown", got)
	}
	if j.Events[0].CompletedOn == "" {
		t.Fatal("expected completed_on to be populated")
	}
}
