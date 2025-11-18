# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.9.6] - 2025-11-18

### Changed
- Cluster registration now requires a per-cluster `capsuleProxyUrl` (and optional `capsuleProxyCa`) so all issued kubeconfigs target capsule-proxy instead of the raw kube-apiserver.
- Tenant and project kubeconfig endpoints issue ServiceAccount-based kubeconfigs via capsule-proxy; Manager-signed JWTs are no longer used for kubeconfigs.
- Removed global `CAPSULE_PROXY_URL` configuration; Capsule proxy is configured per cluster via the API and stored on the Cluster resource.
- Updated docs (`docs/index.md`, `docs/openapi/openapi.yaml`, `README.md`) to reflect the new kubeconfig and cluster registration behavior with concrete curl examples.

### Fixed
- Ensured PaaS bootstrap (`/api/v1/clusters/{c}/bootstrap/paas`) and all kubeconfig endpoints work with the new capsule-proxy-only model and pass the full Go test suite.

---

## [0.9.5] - 2025-11-15

### Changed
- Enforce UUIDs for all resource path params (clusters, tenants, projects, apps). Remove name-based fallbacks. API responses include `uid` consistently.
- Manager auto-installs Agent asynchronously on cluster registration; improved RBAC for bootstrap.
- Agent deployment defaults to a single replica; HPA min/max set to 1 to avoid scaling above one by default.
- Standardize in-cluster resource names to `agent` (Deployment/SA/Role/Binding/HPA) instead of `kubenova-agent`.
- CI builds multi-arch images on `main` and tags (amd64+arm64).
- Bump API and chart versions to 0.9.5.

### Fixed
- Postgres ListClusters now returns `uid` field; clusters list shows IDs.
- Policy/trait/image-update endpoints consistently resolve cluster by UID and use project/app names resolved from UIDs.

---

## [0.9.1] - 2025-11-07

### Added
- KubeVela operations wired: SetTraits, SetPolicies, ImageUpdate; Delete action invokes backend.
- Tenant list filters: `labelSelector` and `owner` for provider-grade queries.
- E2E harness auto-installs Vela Core when `E2E_VELA_OPS=1` (Kind).
- Helm chart icons and Manager JWT secret support (existing or inline).
- README/API maps linked to OpenAPI paths.

### Changed
- E2E uses GHCR `:dev` images; no local image builds.
- CI packages/pushes charts before E2E; E2E depends on image build + chart publish.
- OpenAPI examples for traits/policies/image-update; version endpoint returns `0.9.1`.

### Security
- Trivy config and image scans enforced; gosec clean.

---

## [0.3.3] - 2025-11-05
### Fixed
- Default the Go-based E2E suite to skip during lint/unit workflows unless `E2E_RUN=1`, addressing CI feedback that unit tests should not require Kind.
- Documented the explicit enable flag in `README.md`, `docs/tests.md`, and `docs/site/e2e.md`, and ensured `make test-e2e`/CI set `E2E_RUN` automatically.

## [0.3.2] - 2025-11-04
### Fixed
- Increased the Kind E2E suite HTTP timeout to respect `E2E_WAIT_TIMEOUT`, preventing cluster registration from failing while the Agent installs.
- Documented the extended timeout behaviour across README and testing guides.

## [0.3.1] - 2025-11-03
### Changed
- Replaced legacy shell-based E2E and smoke suites with a Go-based Kind test harness that provisions clusters, deploys Manager/Agent, registers clusters, and validates Capsule/capsule-proxy/KubeVela health.
- Consolidated CI into a single `e2e_suite` job that builds local images, runs `make test-e2e`, and publishes Kind diagnostics as artifacts.

### Added
- Documented the new test architecture in `docs/tests.md` with flow diagrams and updated instructions in `docs/site/e2e.md`.
- Added environment-aware configuration for E2E runs, including optional image builds and cleanup toggles.

## [0.3.0] - 2025-07-01
### Added
- Baseline release of Manager and Agent Helm charts with Capsule/KubeVela integration.
## [0.4.0] - 2025-11-06
### Cleanup & Dedupe
- Remove legacy HTTP routes and JWT middleware from manager; mount single OpenAPI server at `/api/v1`.
- Delete duplicate/unused helpers (`internal/http/errors.go`, `internal/http/auth.go`, manager `respond`, misc env helpers).
- Keep canonical spec at `docs/openapi/openapi.yaml`; remove duplicate `docs/openapi.yaml`.
- Implement Clusters/Tenants/Projects/Apps handlers in OpenAPI server and system/token endpoints.
- Add dynamic backends for tenancy/apps using client-go without vendor leakage.
- Add Postgres `ListClusters` with label filter + cursor and indexes; tests (integration + memory).
- Update docs to reflect single API surface; runnable curl remains.
