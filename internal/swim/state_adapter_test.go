package swim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixturePath(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	return filepath.Join(root, "tests", "swim", "fixtures", "state-sample.json")
}

func TestStatusOf_AllFiveDerivations(t *testing.T) {
	s, err := ReadStateSnapshot(fixturePath(t))
	if err != nil {
		t.Fatalf("ReadStateSnapshot: %v", err)
	}
	cases := map[string]Status{
		"T1.1": StatusTaken,
		"T1.2": StatusReviewTaken,
		"T1.3": StatusReviewEnded,
		"T1.4": StatusCompleted,
		"T1.5": StatusAvailable, // not in either map
	}
	for tid, want := range cases {
		if got := s.StatusOf(tid); got != want {
			t.Errorf("StatusOf(%s) = %q, want %q", tid, got, want)
		}
	}
}

func TestToken_Deterministic(t *testing.T) {
	s1, err := ReadStateSnapshot(fixturePath(t))
	if err != nil {
		t.Fatalf("read s1: %v", err)
	}
	s2, err := ReadStateSnapshot(fixturePath(t))
	if err != nil {
		t.Fatalf("read s2: %v", err)
	}
	if s1.Token() != s2.Token() {
		t.Fatalf("token not deterministic: %s vs %s", s1.Token(), s2.Token())
	}
	if len(s1.Token()) != 64 {
		t.Fatalf("token wrong length %d, want 64 hex chars", len(s1.Token()))
	}
}

func TestToken_ChangesOnMutation(t *testing.T) {
	raw, err := os.ReadFile(fixturePath(t))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	s1, err := ReadStateSnapshotBytes(raw)
	if err != nil {
		t.Fatalf("snap s1: %v", err)
	}
	mutated := strings.Replace(string(raw), "phi", "sigma", 1)
	s2, err := ReadStateSnapshotBytes([]byte(mutated))
	if err != nil {
		t.Fatalf("snap s2: %v", err)
	}
	if s1.Token() == s2.Token() {
		t.Fatalf("token did not change after mutation: %s", s1.Token())
	}
}

func TestStatusOf_BothMapsConflict_CompletedWins(t *testing.T) {
	// Synthetic input: T9.9 is in BOTH taken and completed. Terminal wins.
	body := []byte(`{
		"plan": "x.json",
		"taken": {"T9.9": {"taken_by": "phi", "started_at": "2026-05-08 09:00"}},
		"completed": {"T9.9": {"taken_by": "phi", "started_at": "2026-05-08 09:00", "finished_at": "2026-05-08 09:30"}}
	}`)
	s, err := ReadStateSnapshotBytes(body)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := s.StatusOf("T9.9"); got != StatusCompleted {
		t.Fatalf("conflict resolution: got %q, want %q", got, StatusCompleted)
	}
	warns := s.Warnings()
	found := false
	for _, w := range warns {
		if strings.Contains(w, "T9.9") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning mentioning T9.9, got %v", warns)
	}
}

func TestStatusOf_CompletedWithoutFinishedAt(t *testing.T) {
	// Defensive: a completed entry missing finished_at is treated as available
	// (malformed completion; do not silently mark complete).
	body := []byte(`{
		"plan": "x.json",
		"taken": {},
		"completed": {"T1.1": {"taken_by": "phi", "started_at": "2026-05-08 09:00"}}
	}`)
	s, err := ReadStateSnapshotBytes(body)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := s.StatusOf("T1.1"); got != StatusAvailable {
		t.Errorf("malformed completion: got %q, want %q", got, StatusAvailable)
	}
}

func TestReadStateSnapshot_BadPath(t *testing.T) {
	if _, err := ReadStateSnapshot("/nonexistent/state.json"); err == nil {
		t.Fatal("expected error on missing file")
	}
}

func TestReadStateSnapshot_InvalidJSON(t *testing.T) {
	if _, err := ReadStateSnapshotBytes([]byte("not json")); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

// Sanity: the fixture itself is well-formed JSON.
func TestFixture_IsValidJSON(t *testing.T) {
	raw, err := os.ReadFile(fixturePath(t))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("fixture invalid JSON: %v", err)
	}
}
