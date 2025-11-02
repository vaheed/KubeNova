# .github/

GitHub configuration for CI/CD.

- `workflows/ci.yml` – single workflow with parallel jobs:
  - lint_unit: vet, unit tests, Postgres-backed integration tests
  - gosec_scan, trivy_fs, trivy_config, trivy_images: security scans (run in parallel)
  - build_push_amd64 / build_push_arm64 (main only): container builds + push to GHCR
  - charts_publish: chart repo index to gh-pages (dev/stable)
  - charts_push_oci: charts as OCI to GHCR + lightweight tags
  - e2e_kind: runs API+DB via compose, registers cluster, asserts Agent/Addons ready, CRUD, heartbeats, events; uploads logs
  - docs_publish: VitePress docs build and publish

Notes
- Jobs gate builds on security scans.
- Artifacts include full compose and cluster logs for E2E.
