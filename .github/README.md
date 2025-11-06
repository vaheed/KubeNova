# .github/

GitHub configuration for CI/CD.

- `workflows/ci.yml` – single workflow with parallel jobs:
  - lint_unit: vet, unit tests, Postgres-backed integration tests
  - gosec_scan, trivy_fs, trivy_config, trivy_images: security scans (run in parallel)
  - build_push_amd64 / build_push_arm64 (main only): container builds + push to GHCR
  - e2e_suite: Kind-based end-to-end flow that builds local images, deploys Manager/Agent, validates Capsule, capsule-proxy, and KubeVela, and uploads logs
  - charts_publish: chart repo index to gh-pages (dev/stable)
  - charts_push_oci: charts as OCI to GHCR + lightweight tags
  - docs_publish: VitePress docs build and publish

Notes
- Jobs gate builds on security scans.
- Artifacts include Kind diagnostics and go test output for E2E.
