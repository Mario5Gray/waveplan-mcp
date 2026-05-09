#!/usr/bin/env bash
# T4.3 — verify swim ops doc and README SWIM section reference every
# subcommand and recovery flow promised by the doc contract.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
OPS_DOC="$ROOT_DIR/docs/specs/2026-05-05-swim-ops.md"
README="$ROOT_DIR/README.md"

# 1) Files exist
test -f "$OPS_DOC" || { echo "FAIL: missing $OPS_DOC"; exit 1; }
test -f "$README"  || { echo "FAIL: missing $README"; exit 1; }

# 2) Every subcommand named in the ops doc (7 commands)
for cmd in compile-schedule next step run journal validate compile-plan-json; do
  grep -q -- "swim $cmd" "$OPS_DOC" \
    || { echo "FAIL: ops doc missing 'swim $cmd' reference"; exit 1; }
done

# 3) Recovery section anchors (3 flows)
for anchor in "unknown" "lock_busy" "cursor_drift"; do
  grep -q "$anchor" "$OPS_DOC" \
    || { echo "FAIL: ops doc missing recovery anchor '$anchor'"; exit 1; }
done

# 4) Exit-code reference present (mentions all 4 codes)
for code in "Exit codes" "0 " "2 " "3 " "4 "; do
  grep -q "$code" "$OPS_DOC" \
    || { echo "FAIL: ops doc missing exit-code reference '$code'"; exit 1; }
done

# 5) JSON contract glossary names the four primary report types
for shape in "Decision" "ApplyReport" "RunReport" "Journal"; do
  grep -q "$shape" "$OPS_DOC" \
    || { echo "FAIL: ops doc missing glossary entry for '$shape'"; exit 1; }
done

# 6) README has a SWIM section
grep -q "^## SWIM" "$README" \
  || { echo "FAIL: README missing '## SWIM' section heading"; exit 1; }

# 7) README links to ops doc
grep -q "2026-05-05-swim-ops.md" "$README" \
  || { echo "FAIL: README does not link to ops doc"; exit 1; }

# 8) Example fixtures exist (referenced by quick-start snippets)
test -f "$ROOT_DIR/docs/specs/swim-ops-examples/plan.json" \
  || { echo "FAIL: missing example plan fixture"; exit 1; }
test -f "$ROOT_DIR/docs/specs/swim-ops-examples/waveagents.json" \
  || { echo "FAIL: missing example agents fixture"; exit 1; }

echo "PASS: T4.3 docs present"
