# Raft CLI Split Design

## Goal

Split the current SWIM command surface out of `waveplan-cli` into a dedicated
Python CLI named `raft`, while preserving `waveplan-mcp` as the only MCP
server and keeping the existing `waveplan_swim_*` MCP tool names unchanged.

## Problem

The repository currently has three overlapping SWIM surfaces:

1. SWIM logic and MCP handlers live in the Waveplan core.
2. `waveplan-cli` exposes SWIM as a nested `swim` subtree.
3. SWIM also has several lower-level helper binaries (`swim-run`,
   `swim-step`, etc.).

This makes the product boundary blurry. A user-facing CLI split is desirable,
but a server/core split is not: SWIM still operates on the same Waveplan state
model and should remain near the core.

## Locked Decisions

- `waveplan-mcp` remains the only MCP server.
- SWIM MCP tool names stay exactly as they are today.
- SWIM MCP handlers stay in `waveplan-mcp`.
- `waveplan-cli` becomes Waveplan-only and loses its `swim` subtree.
- A new Python CLI named `raft` becomes the canonical SWIM CLI.
- `raft` talks to `waveplan-mcp` using the same MCP proxy pattern as
  `waveplan-cli`.
- No compatibility alias is kept under `waveplan-cli swim`.

## Product Boundary

### `waveplan-mcp`

Remains authoritative for:

- coarse Waveplan lifecycle tools
- SWIM MCP tools
- state mutation and validation

### `waveplan-cli`

Retains only coarse Waveplan commands:

- `peek`
- `pop`
- `start_review`
- `start_fix`
- `end_review`
- `fin`
- `get`
- `list_plans`
- `deptree`
- `version`

It must no longer advertise or parse any SWIM command surface.

### `raft`

Owns the user-facing SWIM CLI:

- `compile-plan-json`
- `compile-schedule`
- `next`
- `step`
- `run`
- `journal`
- `insert-fix-loop`
- `validate`
- `refine`
- `refine-run`

`raft` is a Python MCP proxy client, not a new execution core.

## Architecture

The split is at the CLI boundary only.

`waveplan-mcp` continues to host both Waveplan and SWIM MCP handlers. `raft`
is introduced as a sibling of `waveplan-cli` that invokes those existing SWIM
MCP tools. This keeps one server entrypoint, one authoritative state model,
and one MCP configuration story while improving separation of concerns for end
users.

## Implementation Requirements

### 1. New CLI

Add a new installable Python script `raft` that mirrors the current SWIM
subcommands from `waveplan-cli`.

Requirements:

- same MCP connection strategy as `waveplan-cli`
- same flag meanings as current `waveplan-cli swim ...`
- user-facing help should present `raft` as a top-level SWIM tool

### 2. Remove SWIM From `waveplan-cli`

Delete the SWIM parser tree and all SWIM dispatch paths from `waveplan-cli`.

Requirements:

- `waveplan-cli --help` no longer lists `swim`
- `waveplan-cli` remains functional for coarse Waveplan commands
- any tests or docs referring to `waveplan-cli swim ...` are updated

### 3. Preserve MCP Compatibility

Do not rename or remove the existing SWIM MCP handlers in `waveplan-mcp`.

Requirements:

- `waveplan_swim_compile`
- `waveplan_swim_next`
- `waveplan_swim_step`
- `waveplan_swim_run`
- `waveplan_swim_journal`
- `waveplan_swim_refine`
- `waveplan_swim_refine_run`

These remain callable exactly as before.

### 4. Installation and Packaging

Install `raft` alongside the existing tools.

Requirements:

- `make install` places `raft` in `~/.local/bin`
- helper script installation includes `raft`
- uninstall/remove flows treat `raft` the same way as other installed Python
  helper scripts

### 5. Documentation Update

Rewrite user-facing docs so the CLI split is explicit.

Requirements:

- `waveplan-cli` docs mention only coarse Waveplan commands
- SWIM usage examples switch to `raft ...`
- MCP docs continue to describe SWIM as accessible through `waveplan-mcp`
- examples should not imply a second MCP server or a second state authority

## Non-Goals

- creating a separate `swim-mcp`
- moving SWIM handlers out of `waveplan-mcp`
- changing SWIM artifact schemas
- changing SWIM MCP tool names
- changing SWIM runtime semantics
- removing the lower-level `swim-*` helper binaries in this cut

## Risks

### Documentation Drift

If `raft` is added but `waveplan-cli swim` references remain in docs or tests,
the split will look incomplete and confuse users.

### Double Surface Regression

If `waveplan-cli swim` is accidentally left in place, the repo will still have
two public SWIM CLIs, defeating the boundary.

### Installation Gaps

If `raft` is not installed by the same pathways as `waveplan-cli`, the split
will be technically present but operationally incomplete.

## Verification

Minimum acceptance criteria:

- `waveplan-cli --help` shows no `swim` subtree
- `raft --help` exposes the full SWIM command set
- existing SWIM MCP tools remain callable through `waveplan-mcp`
- coarse Waveplan CLI tests still pass
- SWIM CLI tests are updated to call `raft`
- `make install` installs `raft`

## Migration Outcome

After this change:

- `waveplan-mcp` remains the single MCP core
- `waveplan-cli` is Waveplan-only
- `raft` is the canonical SWIM CLI
- SWIM stays near core without becoming a second server or a second authority
