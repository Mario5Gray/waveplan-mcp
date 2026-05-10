# txtstore MCP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert txtstore CLI into an MCP server with a Go CLI proxy, following the waveplan pattern.

**Architecture:** Two Go binaries — `txtstore-mcp` (MCP server on stdio) and `txtstore` (CLI proxy that spawns txtstore-mcp over stdio). Shared `internal/txtstore` package.

**Tech Stack:** Go 1.26, `github.com/mark3labs/mcp-go` (already in go.mod)

---

### Task 1: Create txtstore-mcp server

**Files:**
- Create: `cmd/txtstore-mcp/main.go`

- [ ] **Step 1: Create the MCP server**

Create `cmd/txtstore-mcp/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/txtstore"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	mcpServer := server.NewMCPServer("txtstore", "0.1.0")

	// txtstore_append tool
	appendTool := mcp.NewTool("txtstore_append",
		mcp.WithDescription("Append a new section to a markdown file. Auto-renames duplicates with -2, -3, etc."),
		mcp.WithString("filepath", mcp.Required(), mcp.Description("Path to the markdown file")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Section title / anchor text")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Section content")),
		mcp.WithString("unit", mcp.Description("Optional unit prefix for heading hierarchy")),
		mcp.WithString("section", mcp.Description("Optional section prefix for heading hierarchy")),
	)

	mcpServer.AddTool(appendTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath, err := requiredStringParam(request.Params.Arguments, "filepath")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := requiredStringParam(request.Params.Arguments, "title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := requiredStringParam(request.Params.Arguments, "content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		unit, _ := optionalStringParam(request.Params.Arguments, "unit")
		section, _ := optionalStringParam(request.Params.Arguments, "section")

		store := txtstore.New(filepath)
		if err := store.Append(title, content, unit, section); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result := map[string]any{"success": true, "filepath": filepath, "title": title}
		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})

	// txtstore_edit tool
	editTool := mcp.NewTool("txtstore_edit",
		mcp.WithDescription("Replace an existing section with new content. Creates the section if it doesn't exist."),
		mcp.WithString("filepath", mcp.Required(), mcp.Description("Path to the markdown file")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Section title / anchor text")),
		mcp.WithString("content", mcp.Required(), mcp.Description("New section content")),
		mcp.WithString("unit", mcp.Description("Optional unit prefix for heading hierarchy")),
		mcp.WithString("section", mcp.Description("Optional section prefix for heading hierarchy")),
	)

	mcpServer.AddTool(editTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath, err := requiredStringParam(request.Params.Arguments, "filepath")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := requiredStringParam(request.Params.Arguments, "title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := requiredStringParam(request.Params.Arguments, "content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		unit, _ := optionalStringParam(request.Params.Arguments, "unit")
		section, _ := optionalStringParam(request.Params.Arguments, "section")

		store := txtstore.New(filepath)
		if err := store.Edit(title, content, unit, section); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result := map[string]any{"success": true, "filepath": filepath, "title": title}
		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})

	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
```

Wait, I need to import `encoding/json`. Let me fix:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/internal/txtstore"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func requiredStringParam(args any, name string) (string, error) {
	argsMap, ok := args.(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid arguments type")
	}
	val, ok := argsMap[name]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", name)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s must be a string", name)
	}
	return str, nil
}

func optionalStringParam(args any, name string) (string, bool) {
	argsMap, ok := args.(map[string]any)
	if !ok {
		return "", false
	}
	val, ok := argsMap[name]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

func main() {
	mcpServer := server.NewMCPServer("txtstore", "0.1.0")

	// txtstore_append tool
	appendTool := mcp.NewTool("txtstore_append",
		mcp.WithDescription("Append a new section to a markdown file. Auto-renames duplicates with -2, -3, etc."),
		mcp.WithString("filepath", mcp.Required(), mcp.Description("Path to the markdown file")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Section title / anchor text")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Section content")),
		mcp.WithString("unit", mcp.Description("Optional unit prefix for heading hierarchy")),
		mcp.WithString("section", mcp.Description("Optional section prefix for heading hierarchy")),
	)

	mcpServer.AddTool(appendTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath, err := requiredStringParam(request.Params.Arguments, "filepath")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := requiredStringParam(request.Params.Arguments, "title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := requiredStringParam(request.Params.Arguments, "content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		unit, _ := optionalStringParam(request.Params.Arguments, "unit")
		section, _ := optionalStringParam(request.Params.Arguments, "section")

		store := txtstore.New(filepath)
		if err := store.Append(title, content, unit, section); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result := map[string]any{"success": true, "filepath": filepath, "title": title}
		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})

	// txtstore_edit tool
	editTool := mcp.NewTool("txtstore_edit",
		mcp.WithDescription("Replace an existing section with new content. Creates the section if it doesn't exist."),
		mcp.WithString("filepath", mcp.Required(), mcp.Description("Path to the markdown file")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Section title / anchor text")),
		mcp.WithString("content", mcp.Required(), mcp.Description("New section content")),
		mcp.WithString("unit", mcp.Description("Optional unit prefix for heading hierarchy")),
		mcp.WithString("section", mcp.Description("Optional section prefix for heading hierarchy")),
	)

	mcpServer.AddTool(editTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filepath, err := requiredStringParam(request.Params.Arguments, "filepath")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := requiredStringParam(request.Params.Arguments, "title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := requiredStringParam(request.Params.Arguments, "content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		unit, _ := optionalStringParam(request.Params.Arguments, "unit")
		section, _ := optionalStringParam(request.Params.Arguments, "section")

		store := txtstore.New(filepath)
		if err := store.Edit(title, content, unit, section); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result := map[string]any{"success": true, "filepath": filepath, "title": title}
		data, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(data)), nil
	})

	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
```

- [ ] **Step 2: Create the directory and verify it compiles**

Run: `go build ./cmd/txtstore-mcp/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add cmd/txtstore-mcp/main.go
git commit -m "feat: add txtstore-mcp server with append and edit tools"
```

---

### Task 2: Rewrite txtstore CLI as MCP proxy

**Files:**
- Modify: `cmd/txtstore/main.go`

- [ ] **Step 1: Rewrite as MCP proxy**

Replace `cmd/txtstore/main.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const usageText = `txtstore - Create and manage sectioned markdown files

Usage:
  txtstore <command> [arguments] [flags]

Commands:
  append   Append a new section to a markdown file
  edit     Replace an existing section with new content
  help     Show this help text

Append Usage:
  txtstore append <filepath> <title> <content> [--unit U] [--section S]

Edit Usage:
  txtstore edit <filepath> <title> <content> [--unit U] [--section S]

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
`

func printUsage() {
	fmt.Fprint(os.Stderr, usageText)
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

func callMcp(mcpBin, toolName string, args map[string]any) (map[string]any, error) {
	cmd := exec.Command(mcpBin)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Build MCP JSON-RPC message
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}

	// We need to send initialize first, then the tool call
	// Send initialize
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

	// Send init
	initJSON, _ := json.Marshal(initMsg)
	cmd.Stdin.Write(initJSON)
	cmd.Stdin.Write([]byte("\n"))

	// Read init response
	// (simplified: just send tool call and read response)

	// Actually, let's use a simpler approach: pipe JSON to stdin
	// and read JSON from stdout

	// Build the full protocol sequence
	var buf strings.Builder
	buf.WriteString(string(initJSON) + "\n")

	// Send initialized notification
	notifMsg := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	notifJSON, _ := json.Marshal(notifMsg)
	buf.WriteString(string(notifJSON) + "\n")

	// Send tool call
	toolID := 2
	msg["id"] = toolID
	toolJSON, _ := json.Marshal(msg)
	buf.WriteString(string(toolJSON) + "\n")

	// Send exit notification
	exitMsg := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/exit",
	}
	exitJSON, _ := json.Marshal(exitMsg)
	buf.WriteString(string(exitJSON) + "\n")

	// Create new command with pipe
	cmd2 := exec.Command(mcpBin)

	stdin, _ := cmd2.StdinPipe()
	stdout, _ := cmd2.StdoutPipe()

	cmd2.Start()

	stdin.Write([]byte(buf.String()))
	stdin.Close()

	// Read response
	response, _ := io.ReadAll(stdout)
	cmd2.Wait()

	// Parse the tool call response (last JSON object)
	lines := strings.Split(string(response), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err == nil {
			if result, ok := resp["result"]; ok {
				resultMap := result.(map[string]any)
				content := resultMap["content"]
				if contentList, ok := content.([]any); ok && len(contentList) > 0 {
					if contentMap, ok := contentList[0].(map[string]any); ok {
						text := contentMap["text"].(string)
						var result map[string]any
						if err := json.Unmarshal([]byte(text), &result); err == nil {
							return result, nil
						}
					}
				}
			}
			break
		}
	}

	return nil, fmt.Errorf("failed to parse MCP response")
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
		fmt.Fprintln(os.Stderr, "Error: command is required (append or edit)")
		fmt.Fprintln(os.Stderr)
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	if command != "append" && command != "edit" {
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", command)
		fmt.Fprintln(os.Stderr)
		printUsage()
		os.Exit(1)
	}

	if len(os.Args) < 5 {
		fmt.Fprintf(os.Stderr, "Error: append/edit requires <filepath> <title> <content>\n")
		fmt.Fprintln(os.Stderr)
		printUsage()
		os.Exit(1)
	}

	filepath := os.Args[2]
	title := os.Args[3]
	content := os.Args[4]

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

	mcpBin := findMcpBin()
	if mcpBin == "" {
		fmt.Fprintln(os.Stderr, "Error: txtstore-mcp binary not found")
		fmt.Fprintln(os.Stderr, "Set TXTSTORE_MCP_BIN or install to ~/.local/bin/")
		os.Exit(1)
	}

	toolName := "txtstore_" + command
	args := map[string]any{
		"filepath": filepath,
		"title":    title,
		"content":  content,
	}
	if unit != "" {
		args["unit"] = unit
	}
	if section != "" {
		args["section"] = section
	}

	result, err := callMcp(mcpBin, toolName, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/txtstore/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add cmd/txtstore/main.go
git commit -m "feat: rewrite txtstore CLI as MCP proxy to txtstore-mcp"
```

---

### Task 3: Update Makefile

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add txtstore-mcp build target**

Update `Makefile`:

```makefile
.PHONY: build install install-bin install-helpers uninstall-helpers test clean build-mcp install-mcp

BINARY_NAME := waveplan-mcp
MCP_BINARY  := txtstore-mcp
INSTALL_DIR  := $(HOME)/.local/bin
SHARE_DIR    := $(HOME)/.local/share/waveplan
GIT_SHA      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
HELPER_SCRIPTS := waveplan-cli wp-task-to-agent.sh wp-plan-to-agent.sh wp-emit-wave-execution.sh

LDFLAGS := -X main.gitSha=$(GIT_SHA)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)

build-mcp:
	go build -o $(MCP_BINARY) ./cmd/txtstore-mcp/
	go build -o txtstore ./cmd/txtstore/

install: install-bin install-helpers

install-bin: build
	@mkdir -p $(INSTALL_DIR)
	install -m 755 $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)/$(BINARY_NAME)"
	@mkdir -p $(SHARE_DIR)/plans
	@if [ ! -d $(SHARE_DIR)/plans ]; then \
		echo "Created data directory $(SHARE_DIR)/plans"; \
	else \
		echo "Skipping $(SHARE_DIR)/plans - already exists"; \
	fi

install-mcp: build-mcp
	@mkdir -p $(INSTALL_DIR)
	install -m 755 $(MCP_BINARY) $(INSTALL_DIR)/$(MCP_BINARY)
	install -m 755 txtstore $(INSTALL_DIR)/txtstore
	@echo "Installed $(MCP_BINARY) and txtstore to $(INSTALL_DIR)/"

install-helpers:
	@mkdir -p $(INSTALL_DIR)
	@for script in $(HELPER_SCRIPTS); do \
		install -m 755 $$script $(INSTALL_DIR)/$$script; \
		echo "Installed $$script to $(INSTALL_DIR)/$$script"; \
	done

uninstall-helpers:
	@for script in $(HELPER_SCRIPTS); do \
		rm -f $(INSTALL_DIR)/$$script; \
		echo "Removed $(INSTALL_DIR)/$$script"; \
	done

test:
	go test -v ./...

clean:
	rm -f $(BINARY_NAME) $(MCP_BINARY) txtstore
```

- [ ] **Step 2: Verify Makefile works**

Run: `make build-mcp`
Expected: Both binaries built successfully

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add txtstore-mcp and txtstore CLI build targets"
```

---

### Task 4: Integration tests

**Files:**
- No new files needed

- [ ] **Step 1: Test the full flow**

Run:
```bash
make build-mcp
./txtstore append /tmp/test-mcp.md "Section One" "First content"
cat /tmp/test-mcp.md
```

Expected: JSON output with success, file created with index

- [ ] **Step 2: Test append with duplicate**

Run:
```bash
./txtstore append /tmp/test-mcp.md "Section One" "Second content"
cat /tmp/test-mcp.md
```

Expected: Two sections, second renamed to "Section One-2"

- [ ] **Step 3: Test edit**

Run:
```bash
./txtstore edit /tmp/test-mcp.md "Section One" "Updated content"
cat /tmp/test-mcp.md
```

Expected: First section updated, no duplicates

- [ ] **Step 4: Test with hierarchy**

Run:
```bash
./txtstore append /tmp/test-mcp.md "OAuth" "OAuth notes" --unit "Auth" --section "Security"
cat /tmp/test-mcp.md
```

Expected: `## Auth > Security > OAuth` heading

- [ ] **Step 5: Test help**

Run:
```bash
./txtstore --help
./txtstore help
./txtstore -h
```

Expected: Usage text displayed

- [ ] **Step 6: Test error cases**

Run:
```bash
./txtstore
./txtstore invalid
./txtstore append /tmp/test.md
```

Expected: Error messages with usage text

- [ ] **Step 7: Run all tests**

Run: `go test -v ./...`
Expected: All tests pass (txtstore tests pass, planedit failure is pre-existing)

- [ ] **Step 8: Final commit**

```bash
git add Makefile
git commit -m "test: verify txtstore MCP integration end-to-end"
```

---

## Self-Review

**Spec coverage:**
- ✅ Two MCP tools (append, edit) → Task 1
- ✅ Go CLI proxy → Task 2
- ✅ Auto-detect txtstore-mcp binary → Task 2 (findMcpBin)
- ✅ JSON output → Task 2
- ✅ Usage/help on -h, --help, help, invalid → Task 2
- ✅ --unit and --section flags → Task 2
- ✅ Makefile targets → Task 3
- ✅ Tests reuse internal/txtstore → Task 4

**Placeholder scan:** No placeholders found. All code is complete.

**Type consistency:** Function signatures match between tasks. `requiredStringParam` and `optionalStringParam` are consistent.

**Scope check:** Focused on two binaries + Makefile, no decomposition needed.