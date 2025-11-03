# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
