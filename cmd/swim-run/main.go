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
	artifactRoot := flag.String("artifact-root", "", "root directory for SWIM runtime artifacts (logs, receipts, lock). Default: <dirname(schedule)>/.waveplan/swim/<schedule-name>. Env: WAVEPLAN_SWIM_ARTIFACT_ROOT")
	until := flag.String("until", "", "stop condition")
	dryRun := flag.Bool("dry-run", false, "dry run")
	maxSteps := flag.Int("max-steps", 0, "max steps")
	flag.Parse()

	if *schedulePath == "" || *journalPath == "" || *statePath == "" {
		writeError(2, "missing required --schedule/--journal/--state")
	}

	if *artifactRoot == "" {
		*artifactRoot = os.Getenv("WAVEPLAN_SWIM_ARTIFACT_ROOT")
	}

	report, err := swim.Run(swim.RunOptions{
		SchedulePath: *schedulePath,
		JournalPath:  *journalPath,
		StatePath:    *statePath,
		ArtifactRoot: *artifactRoot,
		Until:        *until,
		DryRun:       *dryRun,
		MaxSteps:     *maxSteps,
	})
	if err != nil {
		writeError(3, err.Error())
	}
	switch report.Stopped {
	case "unknown_pending":
		writeJSON(4, report)
	case "blocked", "lock_busy":
		writeJSON(3, report)
	default:
		writeJSON(0, report)
	}
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
