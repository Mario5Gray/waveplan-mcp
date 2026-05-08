#!/usr/bin/env bash
# T1.3 — deterministic schedule emission: same inputs => byte-identical output.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

PLAN="docs/plans/2026-05-05-swim-execution-waves.json"
AGENTS="tests/swim/fixtures/waveagents.json"
WAVEPLAN_CLI_BIN="${WAVEPLAN_CLI_BIN:-$REPO/waveplan-cli}"

OUT1="$(mktemp)"
OUT2="$(mktemp)"
trap 'rm -f "$OUT1" "$OUT2"' EXIT

WAVEPLAN_CLI_BIN="$WAVEPLAN_CLI_BIN" bash wp-emit-wave-execution.sh \
  --plan "$PLAN" \
  --agents "$AGENTS" \
  --task-scope all > "$OUT1"

WAVEPLAN_CLI_BIN="$WAVEPLAN_CLI_BIN" bash wp-emit-wave-execution.sh \
  --plan "$PLAN" \
  --agents "$AGENTS" \
  --task-scope all > "$OUT2"

H1="$(shasum -a 256 "$OUT1" | awk '{print $1}')"
H2="$(shasum -a 256 "$OUT2" | awk '{print $1}')"

if [[ "$H1" != "$H2" ]]; then
  echo "FAIL: deterministic emission mismatch" >&2
  echo "hash1=$H1" >&2
  echo "hash2=$H2" >&2
  exit 1
fi

# step_id format + uniqueness pinned here (reject semantics land in T1.5)
jq -e '.execution | all(.step_id | test("^S[0-9]+_T[0-9]+\\.[0-9]+_(implement|review|end_review|finish)$"))' "$OUT1" >/dev/null \
  || { echo "FAIL: step_id format invalid" >&2; exit 1; }

jq -e '(.execution | map(.step_id) | length) == (.execution | map(.step_id) | unique | length)' "$OUT1" >/dev/null \
  || { echo "FAIL: duplicate step_id emitted" >&2; exit 1; }

echo "PASS: T1.3 deterministic emission + step_id uniqueness"
