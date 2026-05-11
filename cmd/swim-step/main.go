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
	seq := flag.Int("seq", 0, "expected current sequence")
	stepID := flag.String("step-id", "", "expected current step_id")
	apply := flag.Bool("apply", false, "apply current step")
	ackUnknown := flag.String("ack-unknown", "", "acknowledge unknown step_id")
	ackAs := flag.String("as", "", "ack outcome: failed|waived")
	flag.Parse()

	if *ackUnknown != "" {
		if *journalPath == "" {
			writeError(2, "missing required --journal for --ack-unknown")
		}
		if *ackAs == "" {
			writeError(2, "missing required --as for --ack-unknown")
		}
		if err := swim.AckUnknown(*journalPath, *ackUnknown, *ackAs); err != nil {
			writeError(3, err.Error())
		}
		writeJSON(0, map[string]any{
			"ok":      true,
			"step_id": *ackUnknown,
			"outcome": *ackAs,
		})
	}

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
	if *seq > 0 && decision.Row.Seq != *seq {
		writeError(3, "current cursor step does not match --seq")
	}
	if *stepID != "" && decision.Row.StepID != *stepID {
		writeError(3, "current cursor step does not match --step-id")
	}

	if !*apply {
		writeJSON(0, decision)
	}

	report, err := swim.Apply(swim.ApplyOptions{
		SchedulePath: *schedulePath,
		JournalPath:  *journalPath,
		StatePath:    *statePath,
	})
	if err != nil {
		writeError(3, err.Error())
	}
	switch report.Status {
	case "unknown_pending":
		writeJSON(4, report)
	case "blocked", "lock_busy", "incomplete_dispatch":
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
