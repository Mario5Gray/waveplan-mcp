package contextsize

import (
	"encoding/json"
	"testing"
)

func TestDecodeCandidateJSON_RoundTrip(t *testing.T) {
	original := ContextCandidate{
		ID:          "T1.1",
		Title:       "Test task",
		Description: "A test description",
		ReferencedFiles: []string{"main.go", "internal/foo.go"},
		ReferencedSections: []SectionRef{
			{Path: "docs/spec.md", Heading: "Architecture"},
		},
		DependsOn: []string{"T1.0"},
		Source:    "waveplan",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	decoded, err := DecodeCandidateJSON(data)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Title != original.Title {
		t.Errorf("Title mismatch: got %q, want %q", decoded.Title, original.Title)
	}
	if len(decoded.ReferencedFiles) != len(original.ReferencedFiles) {
		t.Errorf("ReferencedFiles length mismatch: got %d, want %d", len(decoded.ReferencedFiles), len(original.ReferencedFiles))
	}
	if decoded.Source != original.Source {
		t.Errorf("Source mismatch: got %q, want %q", decoded.Source, original.Source)
	}
}

func TestDecodeCandidateJSON_RejectsUnknownFields(t *testing.T) {
	input := []byte(`{"id":"T1.1","title":"Test","oops":"field"}`)
	_, err := DecodeCandidateJSON(input)
	if err == nil {
		t.Fatal("expected error for unknown field 'oops'")
	}
}

func TestEncodeEstimateJSON_ProducesValidJSON(t *testing.T) {
	est := ContextEstimate{
		EstimatedTokens: 91000,
		BudgetMin:       64000,
		BudgetMax:       192000,
		Fit:             "within",
		Confidence:      "high",
		Drivers:         []string{"7 referenced files"},
		Recommendation:  "keep",
	}

	data, err := EncodeEstimateJSON(est)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	var decoded ContextEstimate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.EstimatedTokens != est.EstimatedTokens {
		t.Errorf("EstimatedTokens mismatch: got %d, want %d", decoded.EstimatedTokens, est.EstimatedTokens)
	}
	if decoded.Fit != est.Fit {
		t.Errorf("Fit mismatch: got %q, want %q", decoded.Fit, est.Fit)
	}
}