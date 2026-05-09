package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/txtstore"
)

const usageText = `txtstore - Create and manage sectioned markdown files

Usage:
  txtstore [flags]

Flags:
  -f filepath    Path to the markdown file (required)
  -m mode        Mode: 'e' (edit) or 'a' (append) (required)
  -t title       Section title / anchor text (required)
  -txt text      Content text (alternative to stdin)
  --unit unit    Optional unit prefix for heading hierarchy
  --section sec  Optional section prefix for heading hierarchy
  -h, --help     Show this help text

Examples:
  # Append a section via stdin
  echo "content" | txtstore -f notes.md -m a -t "Section Title"

  # Append with inline text
  txtstore -f notes.md -m a -t "Section Title" -txt "content here"

  # Edit an existing section
  txtstore -f notes.md -m e -t "Section Title" -txt "updated content"

  # With unit/section hierarchy
  txtstore -f notes.md -m a -t "OAuth" --unit "Auth" --section "Security" -txt "notes"

  # Duplicate titles get auto-renamed (-2, -3, ...)
  txtstore -f notes.md -m a -t "Section Title" -txt "first"
  txtstore -f notes.md -m a -t "Section Title" -txt "second"  # becomes "Section Title-2"
`

func printUsage() {
	fmt.Fprint(os.Stderr, usageText)
}

func main() {
	// Check for explicit help flags before flag.Parse
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "-help" || arg == "--help" || arg == "help" || arg == "-usage" || arg == "--usage" || arg == "usage" {
			printUsage()
			os.Exit(0)
		}
	}

	mode := flag.String("m", "", "Mode: 'e' (edit) or 'a' (append)")
	filepath := flag.String("f", "", "Path to the markdown file")
	title := flag.String("t", "", "Section title (anchor text)")
	txt := flag.String("txt", "", "Content text (alternative to stdin)")
	unit := flag.String("unit", "", "Optional unit prefix for heading hierarchy")
	section := flag.String("section", "", "Optional section prefix for heading hierarchy")

	flag.Usage = printUsage
	flag.Parse()

	if *mode == "" || *filepath == "" || *title == "" {
		fmt.Fprintln(os.Stderr, "Error: -f, -m, and -t are required")
		fmt.Fprintln(os.Stderr)
		printUsage()
		os.Exit(1)
	}

	if *mode != "e" && *mode != "a" {
		fmt.Fprintln(os.Stderr, "Error: -m must be 'e' or 'a'")
		fmt.Fprintln(os.Stderr)
		printUsage()
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
		content = strings.TrimRight(string(data), "\n\r")
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