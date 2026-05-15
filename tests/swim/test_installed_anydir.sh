#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'chmod -R u+w "$TMP_DIR" 2>/dev/null || true; rm -rf "$TMP_DIR" 2>/dev/null || true' EXIT

INSTALL_HOME="$TMP_DIR/home"
PROJECT_DIR="$TMP_DIR/project"
XDG_CONFIG_HOME="$TMP_DIR/xdg"
mkdir -p "$PROJECT_DIR"
mkdir -p "$XDG_CONFIG_HOME/waveplan-mcp"

cat >"$XDG_CONFIG_HOME/waveplan-mcp/waveagents.json" <<'JSON'
{
  "agents": [
    {"name": "phi", "provider": "codex"},
    {"name": "sigma", "provider": "claude"}
  ],
  "schedule": ["phi", "sigma"]
}
JSON

cd "$ROOT_DIR"
HOME="$INSTALL_HOME" XDG_CONFIG_HOME="$XDG_CONFIG_HOME" make install >/dev/null

cp "$ROOT_DIR/docs/plans/2026-05-05-swim-execution-waves.json" \
  "$PROJECT_DIR/2026-05-05-swim-execution-waves.json"

export PATH="$INSTALL_HOME/.local/bin:$PATH"
export XDG_CONFIG_HOME="$XDG_CONFIG_HOME"

cd "$PROJECT_DIR"
waveplan-cli swim compile-schedule \
  --plan ./2026-05-05-swim-execution-waves.json \
  --out ./swim-schedule.json \
  --bootstrap-state > ./compile.json

jq -e '.ok == true and .state_bootstrapped == true' ./compile.json >/dev/null
test -f ./swim-schedule.json
test -f ./2026-05-05-swim-execution-waves.json.state.json

waveplan-cli swim next --schedule ./swim-schedule.json > ./next.json
jq -e '.action == "ready" and .row.step_id == "S1_T1.1_implement"' ./next.json >/dev/null

echo "PASS: installed artifacts work from any directory"
