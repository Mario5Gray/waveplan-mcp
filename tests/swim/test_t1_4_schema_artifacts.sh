#!/usr/bin/env bash
# T1.4 — schema artifacts exist and schedule output validates.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO"

# Both schema files must exist and parse as JSON.
python3 -c "import json; json.load(open('docs/specs/swim-schedule-schema-v2.json', encoding='utf-8'))"
python3 -c "import json; json.load(open('docs/specs/swim-journal-schema-v1.json', encoding='utf-8'))"

if ! python3 -c "import jsonschema" >/dev/null 2>&1; then
  echo "SKIP: jsonschema not installed (pip install jsonschema)"
  exit 0
fi

OUT="$(mktemp)"
trap 'rm -f "$OUT"' EXIT

bash wp-emit-wave-execution.sh \
  --plan docs/plans/2026-05-05-swim-execution-waves.json \
  --agents tests/swim/fixtures/waveagents.json \
  --task-scope all > "$OUT"

python3 - "$OUT" <<'PY'
import json
import sys
import jsonschema

schema = json.load(open("docs/specs/swim-schedule-schema-v2.json", encoding="utf-8"))
data = json.load(open(sys.argv[1], encoding="utf-8"))
jsonschema.validate(instance=data, schema=schema)
PY

echo "PASS: T1.4 schema artifacts"
