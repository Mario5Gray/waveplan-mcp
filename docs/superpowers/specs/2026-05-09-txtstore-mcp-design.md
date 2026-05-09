# txtstore MCP Server Design

**Date:** 2026-05-09

## Overview

Convert the existing `txtstore` CLI into an MCP server (Go binary) with a thin Python CLI wrapper, following the waveplan pattern: Go MCP server + Python CLI proxy.

## Architecture

```
txtstore-cli (Python)
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

## CLI Wrapper (`txtstore-cli`)

Python script following waveplan-cli pattern:
- `txtstore-cli append <filepath> <title> <content> [--unit U] [--section S]`
- `txtstore-cli edit <filepath> <title> <content> [--unit U] [--section S]`
- Auto-detects `txtstore-mcp` binary from `~/.local/bin/` or `TXTSTORE_MCP_BIN` env var
- Outputs JSON to stdout

## File Structure

- `cmd/txtstore-mcp/main.go` — MCP server (Go)
- `internal/txtstore/filestore.go` — Core logic (shared with CLI, already exists)
- `internal/txtstore/filestore_test.go` — Tests (already exists)
- `txtstore-cli` — Python CLI wrapper
- `txtstore-mcp` — Go binary (built from cmd/)

## Implementation Approach

1. Extract `internal/txtstore` logic into a reusable package (already done)
2. Create `cmd/txtstore-mcp/main.go` — MCP server using `mcp-go` library
3. Create `txtstore-cli` — Python wrapper using `MCPPhone` pattern from waveplan-cli
4. Update `Makefile` to build `txtstore-mcp`
5. Tests: reuse existing `internal/txtstore` tests