#!/usr/bin/env bash
# Guardrail: waveplan YAML manifests are legacy-only and must stay buried.
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
LEGACY_DIR="docs/legacy/waveplan-manifests"

mapfile -t manifests < <(cd "$ROOT_DIR" && rg --files -g '*waveplan-manifest.yaml')

for path in "${manifests[@]}"; do
  if [[ "$path" != "$LEGACY_DIR/"* ]]; then
    echo "FAIL: deprecated YAML manifest outside legacy archive: $path"
    exit 1
  fi
done

grep -Eq 'waveplan-manifest\.yaml.*deprecated|Do not create new YAML manifests' "$ROOT_DIR/README.md" \
  || { echo "FAIL: README missing YAML manifest deprecation note"; exit 1; }

test -f "$ROOT_DIR/$LEGACY_DIR/README.md" \
  || { echo "FAIL: missing legacy manifest archive README"; exit 1; }

echo "PASS: legacy waveplan manifest guard"
