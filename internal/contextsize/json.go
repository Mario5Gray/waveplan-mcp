package contextsize

import (
	"bytes"
	"encoding/json"
)

// DecodeCandidateJSON decodes a ContextCandidate from JSON with strict decoding.
func DecodeCandidateJSON(data []byte) (ContextCandidate, error) {
	var c ContextCandidate
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return ContextCandidate{}, err
	}
	return c, nil
}

// EncodeEstimateJSON encodes a ContextEstimate to pretty-printed JSON.
func EncodeEstimateJSON(e ContextEstimate) ([]byte, error) {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(e); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}