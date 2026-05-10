# txtstore MCP Server Design

**Date:** 2026-05-09

## Overview

Convert the existing `txtstore` CLI into an MCP server (Go binary) with a thin Go CLI proxy, following the waveplan pattern: Go MCP server + Go CLI proxy.

## Architecture

```
txtstore (Go CLI proxy)
  └─ spawns ─► txtstore-mcp (Go binary, stdio MCP server)
```

## MCP Tools

### `txtstore_append`
Append a new section to a markdown file. If the title already exists, auto-renames with `-2`, `-3`, etc.

**Parameters:**
- `filepath` (string, required) — Path to the markdown file
- `title` (string, required) — Section title / anchor text
- `content` (string, required) — Section content
- `unit` (string, optional) — Unit prefix for heading hierarchy
- `section` (string, optional) — Section prefix for heading hierarchy

### `txtstore_edit`
Replace an existing section with new content. Creates the section if it doesn't exist.

**Parameters:**
- `filepath` (string, required) — Path to the markdown file
- `title` (string, required) — Section title / anchor text
- `content` (string, required) — New section content
- `unit` (string, optional) — Unit prefix for heading hierarchy
- `section` (string, optional) — Section prefix for heading hierarchy

## File Format

```markdown
<!-- INDEX -->
- [Title](#anchor)
- [Title-2](#title-2)
<!-- /INDEX -->

## Title
Content...

## Title-2
More content...
```

## CLI Proxy (`txtstore`)

Go CLI following waveplan-cli proxy pattern:
- `txtstore append <filepath> <title> <content> [--unit U] [--section S]`
- `txtstore edit <filepath> <title> <content> [--unit U] [--section S]`
- Auto-detects `txtstore-mcp` binary from `~/.local/bin/` or `TXTSTORE_MCP_BIN` env var
- Outputs JSON to stdout
- Shows usage/help on `-h`, `--help`, `help`, missing args, invalid commands

## File Structure

- `cmd/txtstore-mcp/main.go` — MCP server (Go)
- `cmd/txtstore/main.go` — CLI proxy (Go) — replaces existing `cmd/txtstore/main.go`
- `internal/txtstore/filestore.go` — Core logic (already exists)
- `internal/txtstore/filestore_test.go` — Tests (already exists)

## Implementation Approach

1. Refactor `cmd/txtstore/main.go` from standalone CLI to MCP proxy
2. Create `cmd/txtstore-mcp/main.go` — MCP server using `mcp-go` library
3. Update `Makefile` to build both binaries
4. Tests: reuse existing `internal/txtstore` tests