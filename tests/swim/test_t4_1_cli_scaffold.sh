#!/usr/bin/env bash
# T4.1 — waveplan-cli swim scaffold and argument contracts.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
CLI="$ROOT_DIR/waveplan-cli"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

"$CLI" swim --help >"$TMP_DIR/swim-help.txt"
grep -E -q 'compile-plan-json' "$TMP_DIR/swim-help.txt"
grep -E -q 'compile-schedule' "$TMP_DIR/swim-help.txt"
grep -E -q '\bnext\b' "$TMP_DIR/swim-help.txt"
grep -E -q '\bstep\b' "$TMP_DIR/swim-help.txt"
grep -E -q '\brun\b' "$TMP_DIR/swim-help.txt"
grep -E -q 'journal' "$TMP_DIR/swim-help.txt"
grep -E -q 'validate' "$TMP_DIR/swim-help.txt"

"$CLI" swim compile-schedule --help >"$TMP_DIR/compile-schedule-help.txt"
grep -q -- '--plan' "$TMP_DIR/compile-schedule-help.txt"
grep -q -- '--agents' "$TMP_DIR/compile-schedule-help.txt"
grep -q -- '--out' "$TMP_DIR/compile-schedule-help.txt"
grep -q -- '--task-scope' "$TMP_DIR/compile-schedule-help.txt"

"$CLI" swim next --help >"$TMP_DIR/next-help.txt"
grep -q -- '--schedule' "$TMP_DIR/next-help.txt"
grep -q -- '--journal' "$TMP_DIR/next-help.txt"
grep -q -- '--state' "$TMP_DIR/next-help.txt"

"$CLI" swim step --help >"$TMP_DIR/step-help.txt"
grep -q -- '--schedule' "$TMP_DIR/step-help.txt"
grep -q -- '--journal' "$TMP_DIR/step-help.txt"
grep -q -- '--state' "$TMP_DIR/step-help.txt"
grep -q -- '--seq' "$TMP_DIR/step-help.txt"
grep -q -- '--step-id' "$TMP_DIR/step-help.txt"
grep -q -- '--apply' "$TMP_DIR/step-help.txt"
grep -q -- '--ack-unknown' "$TMP_DIR/step-help.txt"

"$CLI" swim run --help >"$TMP_DIR/run-help.txt"
grep -q -- '--schedule' "$TMP_DIR/run-help.txt"
grep -q -- '--journal' "$TMP_DIR/run-help.txt"
grep -q -- '--state' "$TMP_DIR/run-help.txt"
grep -q -- '--until' "$TMP_DIR/run-help.txt"
grep -q -- '--dry-run' "$TMP_DIR/run-help.txt"
grep -q -- '--max-steps' "$TMP_DIR/run-help.txt"

"$CLI" swim journal --help >"$TMP_DIR/journal-help.txt"
grep -q -- '--schedule' "$TMP_DIR/journal-help.txt"
grep -q -- '--journal' "$TMP_DIR/journal-help.txt"
grep -q -- '--tail' "$TMP_DIR/journal-help.txt"

set +e
"$CLI" swim next --schedule x.json >"$TMP_DIR/stub.json" 2>"$TMP_DIR/stub.err"
status=$?
set -e
test "$status" -eq 2
jq -e '.ok == false and .error == "not_wired_yet" and .subcommand == "next"' "$TMP_DIR/stub.json" >/dev/null

cat >"$TMP_DIR/plan.json" <<'JSON'
{
  "schema_version": 1,
  "generated_on": "2026-05-05",
  "plan": {"name": "swim-test"},
  "doc_index": {
    "spec": {"path": "docs/spec.md", "line": 1, "kind": "plan"}
  },
  "fp_index": {
    "fp-1": {"issue_id": "FP-1"}
  },
  "tasks": {
    "T1": {
      "title": "Base task",
      "plan_line": 1,
      "doc_refs": ["spec"]
    }
  },
  "units": {
    "T1.1": {
      "task": "T1",
      "title": "Base unit",
      "kind": "impl",
      "wave": 1,
      "plan_line": 2,
      "depends_on": [],
      "doc_refs": ["spec"],
      "fp_refs": ["fp-1"]
    }
  }
}
JSON

"$CLI" swim compile-plan-json --in "$TMP_DIR/plan.json" >"$TMP_DIR/plan.out.json"
jq -e '.schema_version == 1 and .units["T1.1"].task == "T1"' "$TMP_DIR/plan.out.json" >/dev/null

echo "PASS: T4.1 cli scaffold"
