#!/usr/bin/env bash
# T7.4 — fixture-backed waveplan-ps observer coverage for review sidecars.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

REVIEW_SIDECAR="$ROOT_DIR/tests/swim/fixtures/review-loop-sidecar.json"

(
  cd "$ROOT_DIR/waveplan-ps"
  unset WAVEPLAN_PLAN WAVEPLAN_STATE WAVEPLAN_JOURNAL WAVEPLAN_SCHED_REVIEW
  GOCACHE="$TMP_DIR/go-cache" go run ./cmd/waveplan-ps --once --review-schedule "$REVIEW_SIDECAR" >"$TMP_DIR/observer.txt"
)

grep -q 'No plans loaded' "$TMP_DIR/observer.txt"
grep -q 'Review Schedules' "$TMP_DIR/observer.txt"
grep -q 'review-loop-sidecar.json (insertions: 2, base: review-loop-schedule.json)' "$TMP_DIR/observer.txt"

echo "PASS: T7.4 review-schedule observer fixture"
