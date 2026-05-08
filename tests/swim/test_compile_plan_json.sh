#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

CLI="$ROOT_DIR/waveplan-cli"

run_expect_fail() {
  local name="$1"
  local in_json="$2"
  local in_path="$TMP_DIR/${name}.in.json"
  local out_path="$TMP_DIR/${name}.out.json"
  local err_path="$TMP_DIR/${name}.err"

  printf '%s\n' "$in_json" >"$in_path"

  if "$CLI" swim compile-plan-json --in "$in_path" --out "$out_path" >"$TMP_DIR/${name}.stdout" 2>"$err_path"; then
    echo "FAIL: expected compile-plan-json to reject case: $name" >&2
    exit 1
  fi
}

run_expect_pass() {
  local name="$1"
  local in_json="$2"
  local in_path="$TMP_DIR/${name}.in.json"
  local out_path="$TMP_DIR/${name}.out.json"

  printf '%s\n' "$in_json" >"$in_path"
  "$CLI" swim compile-plan-json --in "$in_path" --out "$out_path" >"$TMP_DIR/${name}.stdout"
  printf '%s\n' "$out_path"
}

# Base valid fixture. Keep minimal, then mutate per-case.
read -r -d '' BASE_JSON <<'JSON' || true
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

# 1) Preserve unknown-but-valid fields (no whitelist data loss)
WITH_UNKNOWNS=$(printf '%s' "$BASE_JSON" | jq '. + {
  waves: {"1": ["T1.1"]},
  custom_top: {"enabled": true}
} | .units["T1.1"] += {
  command_hint: "echo hi",
  custom_unit: {"note": "keep me"}
} | .tasks["T1"] += {
  custom_task: [1,2,3]
}')
OUT_PATH=$(run_expect_pass "preserve_unknowns" "$WITH_UNKNOWNS")

jq -e '.waves["1"][0] == "T1.1"' "$OUT_PATH" >/dev/null
jq -e '.custom_top.enabled == true' "$OUT_PATH" >/dev/null
jq -e '.units["T1.1"].command_hint == "echo hi"' "$OUT_PATH" >/dev/null
jq -e '.units["T1.1"].custom_unit.note == "keep me"' "$OUT_PATH" >/dev/null
jq -e '.tasks["T1"].custom_task == [1,2,3]' "$OUT_PATH" >/dev/null

# 2) Referential integrity: task/unit doc_refs and unit fp_refs must resolve
BAD_TASK_DOC=$(printf '%s' "$BASE_JSON" | jq '.tasks["T1"].doc_refs = ["missing-doc"]')
run_expect_fail "bad_task_doc_ref" "$BAD_TASK_DOC"

BAD_UNIT_DOC=$(printf '%s' "$BASE_JSON" | jq '.units["T1.1"].doc_refs = ["missing-doc"]')
run_expect_fail "bad_unit_doc_ref" "$BAD_UNIT_DOC"

BAD_UNIT_FP=$(printf '%s' "$BASE_JSON" | jq '.units["T1.1"].fp_refs = ["missing-fp"]')
run_expect_fail "bad_unit_fp_ref" "$BAD_UNIT_FP"

# 3) Scalar type checks
BAD_SCHEMA_VERSION=$(printf '%s' "$BASE_JSON" | jq '.schema_version = "one"')
run_expect_fail "bad_schema_version" "$BAD_SCHEMA_VERSION"

BAD_GENERATED_ON=$(printf '%s' "$BASE_JSON" | jq '.generated_on = 123')
run_expect_fail "bad_generated_on_type" "$BAD_GENERATED_ON"

# 4) Task key regex should accept legacy kebab/dot/underscore IDs
LEGACY_TASK_ID=$(printf '%s' "$BASE_JSON" | jq '.tasks = {
  "tail-hotfix.v1_2": .tasks["T1"]
} | .units["T1.1"].task = "tail-hotfix.v1_2"')
OUT_PATH=$(run_expect_pass "legacy_task_id" "$LEGACY_TASK_ID")
jq -e '.tasks["tail-hotfix.v1_2"].title == "Base task"' "$OUT_PATH" >/dev/null

echo "PASS: swim compile-plan-json regression suite"
