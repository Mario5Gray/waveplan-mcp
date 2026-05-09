#!/usr/bin/env bash
# T2.4 — read-only next-step resolver.
set -euo pipefail
cd "$(dirname "$0")/../.."

go test ./internal/swim/... -run 'ResolveNext' -count=1

echo "PASS: T2.4 resolver"
