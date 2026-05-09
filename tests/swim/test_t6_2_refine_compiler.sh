#!/usr/bin/env bash
# T6.2 — verify the swim-refine-compile binary produces byte-identical
# refinement sidecars from byte-identical input, validates targets,
# enforces the 8k profile lock, and conforms to swim-refine-schema-v1.json.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

# 1) Go-level unit tests
go test ./internal/swim/... -run 'Refine' -count=1

# 2) Build the refine-compile binary so exit codes propagate verbatim.
PLAN="docs/plans/2026-05-05-swim-execution-waves.json"
TMP="$(mktemp -d)"
trap "rm -rf $TMP" EXIT
BIN="$TMP/swim-refine-compile"
go build -o "$BIN" ./cmd/swim-refine-compile

# 3) Determinism: same args twice → byte-identical SHA-256
"$BIN" \
  --plan "$PLAN" --profile 8k --targets "T2.1,T1.1" \
  --out "$TMP/refine.1.json" >/dev/null
"$BIN" \
  --plan "$PLAN" --profile 8k --targets "T2.1,T1.1" \
  --out "$TMP/refine.2.json" >/dev/null

H1=$(sha256sum "$TMP/refine.1.json" | awk '{print $1}')
H2=$(sha256sum "$TMP/refine.2.json" | awk '{print $1}')
if [[ "$H1" != "$H2" ]]; then
  echo "FAIL: not byte-identical: $H1 vs $H2"
  exit 1
fi

# 4) Targets sorted+deduped in output regardless of input order
"$BIN" \
  --plan "$PLAN" --profile 8k --targets "T2.1,T1.1,T2.1,T1.1" \
  --out "$TMP/refine.3.json" >/dev/null

# Output should match the previous (deduped → same set, sorted → same order).
H3=$(sha256sum "$TMP/refine.3.json" | awk '{print $1}')
if [[ "$H1" != "$H3" ]]; then
  echo "FAIL: dedupe not honored: $H1 vs $H3 (with-dups input)"
  exit 1
fi

# 5) Reject non-8k profile
set +e
"$BIN" \
  --plan "$PLAN" --profile 16k --targets "T1.1" >/dev/null 2>"$TMP/err.16k"
status=$?
set -e
if [[ "$status" -ne 3 ]]; then
  echo "FAIL: expected exit 3 on non-8k profile, got $status"
  exit 1
fi
grep -q "v1 supports only" "$TMP/err.16k" \
  || { echo "FAIL: 16k error did not mention v1 lock"; exit 1; }

# 6) Reject empty targets
set +e
"$BIN" --plan "$PLAN" --profile 8k --targets "" >/dev/null 2>"$TMP/err.empty"
status=$?
set -e
if [[ "$status" -ne 2 ]]; then
  echo "FAIL: expected exit 2 on empty targets, got $status"
  exit 1
fi

# 7) Reject unknown target
set +e
"$BIN" \
  --plan "$PLAN" --profile 8k --targets "T999.999" >/dev/null 2>"$TMP/err.unknown"
status=$?
set -e
if [[ "$status" -ne 3 ]]; then
  echo "FAIL: expected exit 3 on unknown target, got $status"
  exit 1
fi

# 8) Step IDs in output match locked F-prefix format
python3 - "$TMP/refine.1.json" <<'PY' || exit 1
import json, re, sys
side = json.load(open(sys.argv[1]))
pat = re.compile(r"^F[0-9]+_[A-Z][0-9]+\.[0-9]+_s[0-9]+$")
for u in side["units"]:
    if not pat.match(u["step_id"]):
        print(f"FAIL: bad step_id {u['step_id']!r}")
        sys.exit(1)
PY

# 9) Schema validation when jsonschema is available
python3 - "$TMP/refine.1.json" <<'PY'
import json, sys
try:
    import jsonschema
except ImportError:
    print("SKIP (schema validation): jsonschema not installed")
    sys.exit(0)
schema = json.load(open("docs/specs/swim-refine-schema-v1.json"))
data = json.load(open(sys.argv[1]))
try:
    jsonschema.validate(data, schema)
except jsonschema.ValidationError as e:
    print(f"FAIL: refine output violates schema: {e.message}")
    sys.exit(1)
print("schema validation: ok")
PY

echo "PASS: T6.2 refine compiler"
