// swim-refine-compile is the v1 deterministic coarse->fine compiler binary.
//
// Reads a SWIM coarse execution-waves plan, refines the named --targets under
// the locked 8k profile, and emits a refinement sidecar JSON conforming to
// docs/specs/swim-refine-schema-v1.json.
//
// Output is byte-identical for byte-identical input (coarse plan content +
// profile name + sorted targets).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/swim"
)

func main() {
	planPath := flag.String("plan", "", "path to coarse *-execution-waves.json plan (required)")
	profile := flag.String("profile", string(swim.ProfileEightK), "refinement profile (v1: 8k only)")
	targetsCSV := flag.String("targets", "", "comma-separated unit IDs to refine (required, non-empty)")
	outPath := flag.String("out", "", "output path; stdout if empty")
	invokerBin := flag.String("invoker", "wp-plan-to-agent.sh", "invoker script path used in emitted invoke.argv")
	flag.Parse()

	if *planPath == "" {
		errExit(2, "missing required --plan")
	}
	if *targetsCSV == "" {
		errExit(2, "missing required --targets (non-empty)")
	}

	targets := splitTargets(*targetsCSV)
	if len(targets) == 0 {
		errExit(2, "--targets is empty after parsing")
	}

	side, err := swim.Refine(swim.RefineOptions{
		CoarsePlanPath: *planPath,
		Profile:        swim.RefineProfile(*profile),
		Targets:        targets,
		InvokerBin:     *invokerBin,
	})
	if err != nil {
		errExit(3, err.Error())
	}

	body, err := swim.MarshalSidecar(side)
	if err != nil {
		errExit(3, "marshal: "+err.Error())
	}

	if *outPath == "" {
		os.Stdout.Write(body)
		return
	}
	if err := os.WriteFile(*outPath, body, 0o644); err != nil {
		errExit(3, "write: "+err.Error())
	}
	report := map[string]any{
		"ok":       true,
		"input":    *planPath,
		"output":   *outPath,
		"profile":  *profile,
		"targets":  side.Targets,
		"unit_count": len(side.Units),
	}
	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(out))
}

func splitTargets(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func errExit(code int, msg string) {
	body, _ := json.MarshalIndent(map[string]any{
		"ok":    false,
		"error": msg,
	}, "", "  ")
	fmt.Fprintln(os.Stderr, string(body))
	os.Exit(code)
}
