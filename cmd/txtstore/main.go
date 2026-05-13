package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const usageText = `txtstore - Create and manage sectioned markdown files

Usage:
  txtstore <command> [arguments] [flags]

Commands:
  append   Append a new section to a markdown file
  edit     Replace an existing section with new content
  write-swim-plan  Write a deterministic SWIM markdown plan
  help     Show this help text

Append Usage:
  txtstore append <filepath> <title> <content> [--unit U] [--section S]

Edit Usage:
  txtstore edit <filepath> <title> <content> [--unit U] [--section S]

Write SWIM Plan Usage:
  txtstore write-swim-plan <filepath> <json-or-json-file>

Flags:
  --unit unit    Optional unit prefix for heading hierarchy
  --section sec  Optional section prefix for heading hierarchy
  -h, --help     Show this help text

Examples:
  # Append a section
  txtstore append notes.md "Section Title" "content here"

  # Append with hierarchy
  txtstore append notes.md "OAuth" "notes" --unit "Auth" --section "Security"

  # Edit an existing section
  txtstore edit notes.md "Section Title" "updated content"

  # Duplicate titles get auto-renamed (-2, -3, ...)
  txtstore append notes.md "Section Title" "first"
  txtstore append notes.md "Section Title" "second"  # becomes "Section Title-2"

  # Write a complete SWIM markdown plan from JSON
  txtstore write-swim-plan docs/plans/source.md plan.json
`

func printUsage() {
	fmt.Fprint(os.Stderr, usageText)
}

func toolNameForCommand(command string) string {
	switch command {
	case "append", "edit":
		return "txtstore_" + command
	case "write-swim-plan":
		return "txtstore_write_swim_plan"
	default:
		return ""
	}
}

func findMcpBin() string {
	// Check env var
	if bin := os.Getenv("TXTSTORE_MCP_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}

	// Check ~/.local/bin/
	usrHome, err := os.UserHomeDir()
	if err == nil {
		defaultPath := filepath.Join(usrHome, ".local", "bin", "txtstore-mcp")
		if _, err := os.Stat(defaultPath); err == nil {
			return defaultPath
		}
	}

	// Check PATH
	if path, err := exec.LookPath("txtstore-mcp"); err == nil {
		return path
	}

	return ""
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func callMcp(mcpBin, toolName string, args map[string]any) (map[string]any, error) {
	cmd := exec.Command(mcpBin)

	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start txtstore-mcp: %w", err)
	}

	// Build MCP protocol sequence
	initMsg := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "txtstore", "version": "0.1.0"},
		},
	}

	toolID := 2
	toolMsg := map[string]any{
		"jsonrpc": "2.0",
		"id":      toolID,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}

	// Write all messages
	encoder := json.NewEncoder(stdin)
	encoder.Encode(initMsg)
	encoder.Encode(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	encoder.Encode(toolMsg)
	encoder.Encode(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/exit",
	})
	stdin.Close()

	// Read responses
	scanner := bufio.NewScanner(stdout)
	var toolResult map[string]any
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var resp mcpResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		if resp.ID != nil && *resp.ID == toolID {
			if resp.Error != nil {
				cmd.Wait()
				return nil, fmt.Errorf("MCP error: %s", resp.Error.Message)
			}
			// Parse result content
			var resultStruct struct {
				Content []map[string]any `json:"content"`
			}
			if err := json.Unmarshal(resp.Result, &resultStruct); err != nil {
				cmd.Wait()
				return nil, fmt.Errorf("failed to parse result: %w", err)
			}
			if len(resultStruct.Content) > 0 {
				if text, ok := resultStruct.Content[0]["text"].(string); ok {
					if err := json.Unmarshal([]byte(text), &toolResult); err != nil {
						cmd.Wait()
						return nil, fmt.Errorf("failed to parse tool result: %w", err)
					}
				}
			}
		}
	}

	cmd.Wait()
	if toolResult == nil {
		return nil, fmt.Errorf("no response from txtstore-mcp")
	}
	return toolResult, nil
}

func loadJSONArgument(arg string) (map[string]any, error) {
	payload := arg
	if info, err := os.Stat(arg); err == nil && !info.IsDir() {
		data, err := os.ReadFile(arg)
		if err != nil {
			return nil, fmt.Errorf("failed to read JSON payload file: %w", err)
		}
		payload = string(data)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return nil, fmt.Errorf("failed to parse JSON payload: %w", err)
	}
	return out, nil
}

func main() {
	// Check for help flags
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "-help" || arg == "--help" || arg == "help" || arg == "-usage" || arg == "--usage" {
			printUsage()
			os.Exit(0)
		}
	}

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Error: command is required (append, edit, or write-swim-plan)")
		fmt.Fprintln(os.Stderr)
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	if command != "append" && command != "edit" && command != "write-swim-plan" {
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", command)
		fmt.Fprintln(os.Stderr)
		printUsage()
		os.Exit(1)
	}

	if command == "write-swim-plan" {
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Error: write-swim-plan requires <filepath> <json-or-json-file>\n")
			fmt.Fprintln(os.Stderr)
			printUsage()
			os.Exit(1)
		}
	} else if len(os.Args) < 5 {
		fmt.Fprintf(os.Stderr, "Error: append/edit requires <filepath> <title> <content>\n")
		fmt.Fprintln(os.Stderr)
		printUsage()
		os.Exit(1)
	}

	filepath := os.Args[2]

	mcpBin := findMcpBin()
	if mcpBin == "" {
		fmt.Fprintln(os.Stderr, "Error: txtstore-mcp binary not found")
		fmt.Fprintln(os.Stderr, "Set TXTSTORE_MCP_BIN or install to ~/.local/bin/")
		os.Exit(1)
	}

	toolName := toolNameForCommand(command)
	args := map[string]any{
		"filepath": filepath,
	}

	if command == "write-swim-plan" {
		payload, err := loadJSONArgument(os.Args[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		for key, value := range payload {
			args[key] = value
		}
	} else {
		title := os.Args[3]
		content := os.Args[4]
		args["title"] = title
		args["content"] = content

		var unit, section string
		for i := 5; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--unit":
				if i+1 < len(os.Args) {
					unit = os.Args[i+1]
					i++
				}
			case "--section":
				if i+1 < len(os.Args) {
					section = os.Args[i+1]
					i++
				}
			}
		}
		if unit != "" {
			args["unit"] = unit
		}
		if section != "" {
			args["section"] = section
		}
	}

	result, err := callMcp(mcpBin, toolName, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}
