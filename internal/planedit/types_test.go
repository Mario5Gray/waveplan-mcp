package planedit

import "testing"

func TestDecodePlan_RejectsUnknownTopLevelField(t *testing.T) {
	input := []byte(`{"units":{},"tasks":{},"doc_index":{},"fp_index":{},"oops":1}`)
	_, err := DecodePlanStrict(input)
	if err == nil {
		t.Fatalf("expected error for unknown top-level field 'oops'")
	}
}

func TestDecodePlan_RequiresSchemaSections(t *testing.T) {
	input := []byte(`{"units":{},"tasks":{}}`)
	_, err := DecodePlanStrict(input)
	if err == nil {
		t.Fatalf("expected error for missing required sections doc_index/fp_index")
	}
}
