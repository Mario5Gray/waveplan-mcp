#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

./wp-plan-step.sh --help | grep -q -- '--task-id' || {
  echo "FAIL: wp-plan-step.sh help does not mention --task-id"
  exit 1
}

./wp-agent-dispatch.sh --help | grep -q -- '--task-json-file' || {
  echo "FAIL: wp-agent-dispatch.sh help does not mention --task-json-file"
  exit 1
}

if ./wp-agent-dispatch.sh --target opencode --plan /tmp/p --agent phi --mode implement --task-id T1.1 2>"$TMP/err.txt"; then
  echo "FAIL: wp-agent-dispatch.sh accepted --task-id"
  exit 1
fi

grep -q 'Unknown argument: --task-id' "$TMP/err.txt" || {
  echo "FAIL: wp-agent-dispatch.sh did not reject --task-id as an unknown argument"
  exit 1
}

echo "PASS: plan-step/agent-dispatch contract"
