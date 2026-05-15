# Legacy Waveplan YAML Manifests

`*waveplan-manifest.yaml` is deprecated in this repository.

These files were transitional adapter inputs used before the canonical plan
authoring chain was standardized as:

1. `*-swim.json`
2. `*-swim.md`
3. `*-execution-waves.json`

Rules:

- Do not create new `*waveplan-manifest.yaml` files.
- Do not treat this directory as an active authoring surface.
- Keep archived files here only for historical reference or import work.

The repo guard test `tests/swim/test_legacy_waveplan_manifest_guard.sh`
enforces that any remaining YAML manifests live only under this directory.
