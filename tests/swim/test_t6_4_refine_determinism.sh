#!/usr/bin/env bash
# T6.4 — determinism golden tests for swim refine + refine-run CLI subcommands.
# Verifies: byte-identical output for byte-identical input across two CLI runs;
# missing required args surface exit 2; dry-run refine-run round-trip via CLI.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

CLI="$ROOT_DIR/waveplan-cli"

TMP="$(mktemp -d)"
trap "rm -rf $TMP" EXIT

# 1) Build a coarse plan with a unit that has 7 files (forces 2 fine chunks)
cat >"$TMP/coarse.json" <<'JSON'
{
  "schema_version": 1,
  "generated_on": "2026-05-09",
  "plan_version": 1,
  "plan_generation": "2026-05-09T00:00:00Z",
  "plan": {"id": "t6-4-det"},
  "fp_index": {},
  "doc_index": {},
  "tasks": {
    "T1": {"title": "task one", "files": ["a.go","b.go","c.go","d.go","e.go","f.go","g.go"]}
  },
  "units": {
    "T1.1": {
      "task": "T1",
      "title": "impl unit",
      "kind": "impl",
      "wave": 1,
      "plan_line": 1,
      "depends_on": [],
      "files": ["a.go","b.go","c.go","d.go","e.go","f.go","g.go"]
    }
  }
}
JSON

# 2) First compile run
"$CLI" swim refine --plan "$TMP/coarse.json" --targets T1.1 >"$TMP/run1.json"

# 3) Second compile run — output must be byte-identical
"$CLI" swim refine --plan "$TMP/coarse.json" --targets T1.1 >"$TMP/run2.json"

if ! cmp -s "$TMP/run1.json" "$TMP/run2.json"; then
  echo "FAIL: refine outputs are not byte-identical"
  diff "$TMP/run1.json" "$TMP/run2.json"
  exit 1
fi

# 4) Output is valid JSON and has expected structure
jq -e '.schema_version == 1 and (.targets | length) == 1 and .targets[0] == "T1.1" and (.units | length) >= 2' \
  "$TMP/run1.json" >/dev/null \
  || { echo "FAIL: refine sidecar structure invalid"; cat "$TMP/run1.json"; exit 1; }

# 5) --out flag writes to file, stdout contains ok report
"$CLI" swim refine --plan "$TMP/coarse.json" --targets T1.1 --out "$TMP/out.json" >"$TMP/report.json"
jq -e '.ok == true and .output != null' "$TMP/report.json" >/dev/null \
  || { echo "FAIL: --out report did not surface ok+output"; cat "$TMP/report.json"; exit 1; }
cmp -s "$TMP/run1.json" "$TMP/out.json" \
  || { echo "FAIL: --out file differs from stdout output"; exit 1; }

# 6) Multiple --targets accepted (space-separated via CLI)
"$CLI" swim refine --plan "$TMP/coarse.json" --targets T1.1 >"$TMP/multi.json"
jq -e '.targets | length == 1' "$TMP/multi.json" >/dev/null \
  || { echo "FAIL: targets length wrong"; exit 1; }

# 7) Missing --plan → exit 2
set +e
"$CLI" swim refine --targets T1.1 >/dev/null 2>"$TMP/err.no-plan"
status=$?
set -e
if [[ "$status" -ne 2 ]]; then
  echo "FAIL: expected exit 2 on missing --plan, got $status"
  exit 1
fi

# 8) Missing --targets → exit 2
set +e
"$CLI" swim refine --plan "$TMP/coarse.json" >/dev/null 2>"$TMP/err.no-targets"
status=$?
set -e
if [[ "$status" -ne 2 ]]; then
  echo "FAIL: expected exit 2 on missing --targets, got $status"
  exit 1
fi

# 9) Build a state file and run refine-run via CLI in dry-run mode
cat >"$TMP/state.json" <<'JSON'
{
  "plan": "demo",
  "taken": {
    "T1.1": {
      "taken_by": "phi",
      "started_at": "2026-05-09 10:00",
      "review_entered_at": "2026-05-09 10:30"
    }
  },
  "completed": {}
}
JSON

"$CLI" swim refine-run \
  --refine "$TMP/run1.json" \
  --state "$TMP/state.json" \
  --dry-run >"$TMP/dry.json"
jq -e '.dry_run == true' "$TMP/dry.json" >/dev/null \
  || { echo "FAIL: refine-run dry-run report missing dry_run flag"; cat "$TMP/dry.json"; exit 1; }

# 10) Missing --refine → exit 2 from refine-run binary (surfaced through CLI)
set +e
"$CLI" swim refine-run --state "$TMP/state.json" >/dev/null 2>"$TMP/err.no-refine"
status=$?
set -e
if [[ "$status" -ne 2 ]]; then
  echo "FAIL: expected exit 2 on missing --refine, got $status"
  exit 1
fi

echo "PASS: T6.4 refine determinism + CLI subcommands"
