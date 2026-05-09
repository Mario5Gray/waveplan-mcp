#!/usr/bin/env bash
# T2.5 — apply-time race closure and unknown recovery.
set -euo pipefail
cd "$(dirname "$0")/../.."

go test ./internal/swim/... -run 'ExecuteNextStepSafe|DetectAndMarkUnknown' -count=1

echo "PASS: T2.5 race closure"
