# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
