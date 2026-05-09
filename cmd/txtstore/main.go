package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/txtstore"
)

func main() {
	mode := flag.String("m", "", "Mode: 'e' (edit) or 'a' (append)")
	filepath := flag.String("f", "", "Path to the markdown file")
	title := flag.String("t", "", "Section title (anchor text)")
	txt := flag.String("txt", "", "Content text (alternative to stdin)")
	unit := flag.String("unit", "", "Optional unit prefix for heading hierarchy")
	section := flag.String("section", "", "Optional section prefix for heading hierarchy")
	flag.Parse()

	if *mode == "" || *filepath == "" || *title == "" {
		fmt.Fprintln(os.Stderr, "Error: -f, -m, and -t are required")
		os.Exit(1)
	}

	if *mode != "e" && *mode != "a" {
		fmt.Fprintln(os.Stderr, "Error: -m must be 'e' or 'a'")
		os.Exit(1)
	}

	var content string
	if *txt != "" {
		content = *txt
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		content = string(data)
	}

	store := txtstore.New(*filepath)

	var err error
	switch *mode {
	case "e":
		err = store.Edit(*title, content, *unit, *section)
	case "a":
		err = store.Append(*title, content, *unit, *section)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}