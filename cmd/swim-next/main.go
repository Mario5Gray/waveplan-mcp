package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/swim"
)

func main() {
	schedulePath := flag.String("schedule", "", "path to schedule JSON (swim-schedule-schema-v2)")
	reviewSchedulePath := flag.String("review-schedule", "", "path to review schedule sidecar JSON")
	journalPath := flag.String("journal", "", "path to journal JSON sidecar")
	workDir := flag.String("workdir", "", "optional working directory for invoke.argv")
	expectCursor := flag.Int("expect-cursor", -1, "optional CAS guard: expected current cursor")
	flag.Parse()

	opts := swim.ExecNextOptions{
		SchedulePath:       *schedulePath,
		ReviewSchedulePath: *reviewSchedulePath,
		JournalPath:        *journalPath,
		WorkDir:            *workDir,
	}
	if *expectCursor >= 0 {
		opts.ExpectCursor = expectCursor
	}

	res, err := swim.ExecuteNextStep(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "swim-next error: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
}
