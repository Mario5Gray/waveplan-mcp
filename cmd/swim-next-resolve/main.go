package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/swim"
)

func main() {
	schedulePath := flag.String("schedule", "", "path to schedule JSON")
	journalPath := flag.String("journal", "", "path to journal JSON")
	statePath := flag.String("state", "", "path to state JSON")
	flag.Parse()

	if *schedulePath == "" || *journalPath == "" || *statePath == "" {
		writeError(2, "missing required --schedule/--journal/--state")
	}

	decision, err := swim.ResolveNextFromPaths(swim.NextOptions{
		SchedulePath: *schedulePath,
		JournalPath:  *journalPath,
		StatePath:    *statePath,
	})
	if err != nil {
		writeError(3, err.Error())
	}

	writeJSON(0, decision)
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
