#!/usr/bin/env bash
# T5.x — MCP swim tools and CLI/MCP parity surface.
set -euo pipefail
cd "$(dirname "$0")/../.."

go test . -run 'Swim' -count=1

echo "PASS: T5 MCP swim tools"
