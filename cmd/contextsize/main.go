package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/contextsize"
)

const usage = `contextsize — estimate context footprint of an issue candidate

Estimates the token budget an LLM would need to read and implement a task.
Outputs a JSON ContextEstimate with fit classification, confidence scoring,
and split/merge recommendations.

USAGE
  contextsize [estimate] --candidate <file.json> [options]

ARGUMENTS
  --candidate       Path to ContextCandidate JSON file (required)
  --budget          Budget range in tokens, min:max (default: 64000:192000)
  --base-dir        Root directory for resolving referenced file paths

EXAMPLES
	# Estimate a hand-authored candidate
	contextsize --candidate issue.json

	# With custom budget
	contextsize --candidate issue.json --budget 96000:192000

	# With base directory for relative file paths
	contextsize --candidate issue.json --base-dir /path/to/repo

  # Via waveplan-cli
  python waveplan-cli context estimate --candidate issue.json --base-dir /path/to/repo

CANDIDATE FORMAT
  The candidate file must be valid JSON matching the ContextCandidate schema:
  {
    "id": "T1.1",
    "title": "Add feature X",
    "description": "Brief description of the change",
    "referenced_files": ["main.go", "internal/foo.go"],
    "referenced_sections": [{"path": "docs/spec.md", "heading": "Architecture"}],
    "depends_on": ["T1.0"],
    "source": "waveplan"
  }

OUTPUT
  A JSON ContextEstimate with fields: estimated_tokens, fit, confidence,
  drivers, recommendation, missing_files, missing_sections, unknown_files,
  split_candidates, merge_candidates.
`

func main() {
	candidatePath := flag.String("candidate", "", "Path to ContextCandidate JSON file")
	budgetFlag := flag.String("budget", "64000:192000", "Budget range in tokens (min:max)")
	baseDir := flag.String("base-dir", "", "Root for resolving referenced file paths")
	helpFlag := flag.Bool("help", false, "Show this help message")
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "estimate" {
		args = args[1:]
	}
	if err := flag.CommandLine.Parse(args); err != nil {
		os.Exit(2)
	}

	if *helpFlag || flag.NFlag() == 0 {
		fmt.Print(usage)
		os.Exit(0)
	}

	if *candidatePath == "" {
		fmt.Fprintln(os.Stderr, "Error: --candidate is required")
		fmt.Fprintln(os.Stderr, "Run 'contextsize' with no arguments for usage info.")
		os.Exit(2)
	}

	// Parse budget.
	parts := strings.Split(*budgetFlag, ":")
	if len(parts) != 2 {
		fmt.Fprintln(os.Stderr, "Error: --budget must be in format min:max")
		os.Exit(2)
	}

	var minTokens, maxTokens int
	if _, err := fmt.Sscanf(parts[0], "%d", &minTokens); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid budget min: %s\n", parts[0])
		os.Exit(2)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &maxTokens); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid budget max: %s\n", parts[1])
		os.Exit(2)
	}

	budget := contextsize.Budget{
		MinTokens: minTokens,
		MaxTokens: maxTokens,
	}

	// Read candidate JSON.
	candidateData, err := os.ReadFile(*candidatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading candidate file: %v\n", err)
		os.Exit(3)
	}

	candidate, err := contextsize.DecodeCandidateJSON(candidateData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding candidate: %v\n", err)
		os.Exit(2)
	}

	// Run estimation.
	estimator := &contextsize.Estimator{
		BaseDir: *baseDir,
	}

	est, err := estimator.Estimate(candidate, budget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error estimating context: %v\n", err)
		os.Exit(2)
	}

	// Output result.
	output, err := contextsize.EncodeEstimateJSON(est)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding result: %v\n", err)
		os.Exit(3)
	}

	fmt.Print(string(output))
}
