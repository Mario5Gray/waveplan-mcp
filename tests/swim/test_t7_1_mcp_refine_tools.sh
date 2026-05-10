#!/usr/bin/env bash
# T7.1 — MCP refine parity for swim refine + refine-run tools.
set -euo pipefail
cd "$(dirname "$0")/../.."

go test . -run 'Test(CreateTools_IncludesSwimTools|HandleSwimRefine|HandleSwimRefineRunDryRun)$' -count=1

echo "PASS: T7.1 MCP refine tools"
