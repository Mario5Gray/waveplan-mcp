#!/usr/bin/env bash
# T3.3 — run --until orchestration and dry-run.
set -euo pipefail
cd "$(dirname "$0")/../.."

go test ./internal/swim/... -run 'Run_' -count=1

echo "PASS: T3.3 run until"
