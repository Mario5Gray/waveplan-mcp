#!/usr/bin/env bash
# T6.1 — verify refinement schema artifacts and the v1 8k profile contract
# are present and self-consistent. Schema validation against the sample
# fixture runs when jsonschema is installed; otherwise SKIP.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
SCHEMA="$ROOT_DIR/docs/specs/swim-refine-schema-v1.json"
PROFILE="$ROOT_DIR/docs/specs/swim-refine-profile-8k.md"
FIXTURE="$ROOT_DIR/tests/swim/fixtures/swim-refine-sample.json"

# 1) Files exist
test -f "$SCHEMA"  || { echo "FAIL: missing $SCHEMA"; exit 1; }
test -f "$PROFILE" || { echo "FAIL: missing $PROFILE"; exit 1; }
test -f "$FIXTURE" || { echo "FAIL: missing $FIXTURE"; exit 1; }

# 2) Schema parses as JSON
python3 -c "import json; json.load(open('$SCHEMA'))" \
  || { echo "FAIL: schema is not valid JSON"; exit 1; }

# 3) Fixture parses as JSON
python3 -c "import json; json.load(open('$FIXTURE'))" \
  || { echo "FAIL: fixture is not valid JSON"; exit 1; }

# 4) step_id format pinned by spec §6.1: F{wave}_{parent_unit}_s{n}
python3 - "$FIXTURE" <<'PY' || exit 1
import json, re, sys
fixture = json.load(open(sys.argv[1]))
pattern = re.compile(r"^F[0-9]+_[A-Z][0-9]+\.[0-9]+_s[0-9]+$")
for u in fixture["units"]:
    if not pattern.match(u["step_id"]):
        print(f"FAIL: step_id {u['step_id']!r} violates F{{wave}}_{{parent}}_s{{n}}")
        sys.exit(1)
PY

# 5) Profile contract document mentions all three locked limits
for limit in "max_tokens" "max_files" "max_lines"; do
  grep -q "$limit" "$PROFILE" \
    || { echo "FAIL: profile doc missing limit '$limit'"; exit 1; }
done

# 6) Profile doc names the F{wave}_{parent}_s{n} format
grep -q "F{wave}_{parent_unit}_s{n}" "$PROFILE" \
  || { echo "FAIL: profile doc missing step_id format spec"; exit 1; }

# 7) Schema declares profile enum is exactly ["8k"] (v1 lock)
python3 - "$SCHEMA" <<'PY' || exit 1
import json, sys
schema = json.load(open(sys.argv[1]))
profile_enum = schema["properties"]["profile"]["enum"]
if profile_enum != ["8k"]:
    print(f"FAIL: profile enum should be exactly ['8k'] for v1, got {profile_enum}")
    sys.exit(1)
PY

# 8) command_hint is documented as debug-only in profile doc
grep -q -i "debug-only" "$PROFILE" \
  || { echo "FAIL: profile doc must document command_hint as debug-only"; exit 1; }

# 9) Optional: full schema validation when jsonschema available
python3 - "$SCHEMA" "$FIXTURE" <<'PY'
import json, sys
try:
    import jsonschema
except ImportError:
    print("SKIP (schema validation): jsonschema not installed (pip install jsonschema)")
    sys.exit(0)
schema = json.load(open(sys.argv[1]))
data = json.load(open(sys.argv[2]))
try:
    jsonschema.validate(data, schema)
except jsonschema.ValidationError as e:
    print(f"FAIL: fixture violates schema: {e.message}")
    sys.exit(1)
print("schema validation: ok")
PY

echo "PASS: T6.1 refine schema + 8k profile contract"
