#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'chmod -R u+w "$TMP_DIR" 2>/dev/null || true; rm -rf "$TMP_DIR" 2>/dev/null || true' EXIT

INSTALL_HOME="$TMP_DIR/home"
PROJECT_DIR="$TMP_DIR/project"
mkdir -p "$PROJECT_DIR"

cd "$ROOT_DIR"
HOME="$INSTALL_HOME" make install >/dev/null

cp "$ROOT_DIR/docs/plans/2026-04-25-txt2art-amiga-execution-waves.json" \
  "$PROJECT_DIR/2026-04-25-txt2art-amiga-execution-waves.json"

export PATH="$INSTALL_HOME/.local/bin:$PATH"

cd "$PROJECT_DIR"
waveplan-cli swim compile-schedule \
  --plan ./2026-04-25-txt2art-amiga-execution-waves.json \
  --out ./txt2art-schedule.json \
  --bootstrap-state > ./compile.json

jq -e '.ok == true and .state_bootstrapped == true' ./compile.json >/dev/null
test -f ./txt2art-schedule.json
test -f ./2026-04-25-txt2art-amiga-execution-waves.json.state.json

waveplan-cli swim next --schedule ./txt2art-schedule.json > ./next.json
jq -e '.action == "ready" and .row.step_id == "S1_T1.1_implement"' ./next.json >/dev/null

echo "PASS: installed artifacts work from any directory"
