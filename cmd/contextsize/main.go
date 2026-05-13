package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/contextsize"
)

func main() {
	candidatePath := flag.String("candidate", "", "Path to ContextCandidate JSON file")
	budgetFlag := flag.String("budget", "64000:192000", "Budget range in tokens (min:max)")
	baseDir := flag.String("base-dir", "", "Root for resolving referenced file paths")
	flag.Parse()

	if *candidatePath == "" {
		fmt.Fprintln(os.Stderr, "Error: --candidate is required")
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