package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/swim"
)

func main() {
	schedulePath := flag.String("schedule", "", "path to schedule JSON")
	reviewSchedulePath := flag.String("review-schedule", "", "path to review schedule sidecar JSON")
	journalPath := flag.String("journal", "", "path to journal JSON")
	tail := flag.Int("tail", 0, "tail N events")
	flag.Parse()

	if *journalPath == "" {
		writeError(2, "missing required --journal")
	}
	view, err := swim.ReadJournalView(*journalPath, *schedulePath, *reviewSchedulePath, *tail)
	if err != nil {
		writeError(3, err.Error())
	}
	writeJSON(0, view)
}

func writeJSON(code int, v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	os.Exit(code)
}

func writeError(code int, message string) {
	writeJSON(code, map[string]any{
		"ok":    false,
		"error": message,
	})
}
