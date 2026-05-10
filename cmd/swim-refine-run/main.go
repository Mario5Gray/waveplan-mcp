// swim-refine-run executes a refinement sidecar's fine steps under the locked
// A/B/C apply protocol (per fine event) and writes a synthetic
// PARENT_<unit>_rollup event into the coarse journal once every targeted
// child for a parent reaches a terminal outcome.
//
// Concurrency note: the refine and coarse locks are independent. Operators
// MUST not run coarse swim Apply against the same parent unit while
// swim-refine-run is in flight; this safety boundary is workflow-enforced.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/swim"
)

func main() {
	refinePath := flag.String("refine", "", "path to refinement sidecar (required)")
	refineJournal := flag.String("refine-journal", "", "fine journal path (default: <refine>.journal.json)")
	coarseJournal := flag.String("coarse-journal", "", "coarse journal path (required for rollup events)")
	statePath := flag.String("state", "", "live waveplan state file (required)")
	dryRun := flag.Bool("dry-run", false, "resolve path without lock, invoke, or journal mutation")
	workDir := flag.String("work-dir", "", "working directory passed to invoke")
	flag.Parse()

	if *refinePath == "" {
		errExit(2, "missing required --refine")
	}
	if *statePath == "" && !*dryRun {
		errExit(2, "missing required --state (use --dry-run for read-only preview)")
	}

	report, err := swim.RefineRun(swim.RefineRunOptions{
		RefinePath:        *refinePath,
		RefineJournalPath: *refineJournal,
		CoarseJournalPath: *coarseJournal,
		StatePath:         *statePath,
		WorkDir:           *workDir,
		DryRun:            *dryRun,
	})
	if err != nil {
		errExit(3, err.Error())
	}

	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		errExit(3, "marshal: "+err.Error())
	}
	fmt.Println(string(body))

	switch report.Stopped {
	case "lock_busy":
		os.Exit(3)
	case "non_applied", "cross_parent_gate":
		// blocked: actionable but not a hard error. Exit 0; report carries detail.
	}
}

func errExit(code int, msg string) {
	body, _ := json.MarshalIndent(map[string]any{
		"ok":    false,
		"error": msg,
	}, "", "  ")
	fmt.Fprintln(os.Stderr, string(body))
	os.Exit(code)
}
