#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'chmod -R u+w "$TMP_DIR" 2>/dev/null || true; rm -rf "$TMP_DIR" 2>/dev/null || true' EXIT

INSTALL_HOME="$TMP_DIR/home"
CONFIG_DIR="$INSTALL_HOME/.config/waveplan-mcp"
CONFIG_FILE="$CONFIG_DIR/waveagents.json"

cd "$ROOT_DIR"

HOME="$INSTALL_HOME" make install >/dev/null
test -f "$CONFIG_FILE"
cmp -s "$CONFIG_FILE" "$ROOT_DIR/docs/specs/swim-ops-examples/waveagents.json"

cat >"$CONFIG_FILE" <<'JSON'
{"custom":true}
JSON

HOME="$INSTALL_HOME" make install-mcp >/dev/null
cmp -s "$CONFIG_FILE" <(printf '%s\n' '{"custom":true}')

echo "PASS: install seeds config"
