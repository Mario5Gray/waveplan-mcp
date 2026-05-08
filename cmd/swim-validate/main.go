package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/swim"
)

func main() {
	kind := flag.String("kind", "schedule", "validation kind: schedule|journal")
	inPath := flag.String("in", "", "path to JSON file")
	flag.Parse()

	if *inPath == "" {
		fmt.Fprintln(os.Stderr, "missing required --in")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		os.Exit(2)
	}

	switch *kind {
	case "schedule":
		err = swim.ValidateSchedule(raw)
	case "journal":
		err = swim.ValidateJournal(raw)
	default:
		fmt.Fprintf(os.Stderr, "invalid --kind: %q (expected schedule|journal)\n", *kind)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "validation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("ok")
}
