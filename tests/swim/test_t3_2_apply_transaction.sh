#!/usr/bin/env bash
# T3.2 — apply transaction wrapper with lock-busy and unknown diagnostics.
set -euo pipefail
cd "$(dirname "$0")/../.."

go test ./internal/swim/... -run 'Apply_' -count=1

echo "PASS: T3.2 apply transaction"
