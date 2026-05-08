#!/usr/bin/env bash
# T2.2 — evaluator precondition checks + deterministic failure codes.
set -euo pipefail
cd "$(dirname "$0")/../.."

go test ./internal/swim/... \
  -run 'Evaluate|Predict' \
  -count=1

echo "PASS: T2.2 evaluator"
