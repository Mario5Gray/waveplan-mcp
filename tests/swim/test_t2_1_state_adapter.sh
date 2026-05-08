#!/usr/bin/env bash
# T2.1 — verify state adapter parses waveplan state and derives canonical
# Status enum, including review_taken/review_ended via timestamp presence.
# Token() must be deterministic and change on any content mutation.
set -euo pipefail
cd "$(dirname "$0")/../.."

go test ./internal/swim/... \
  -run 'StatusOf|Token|ReadStateSnapshot|Fixture' \
  -count=1

echo "PASS: T2.1 state adapter"
