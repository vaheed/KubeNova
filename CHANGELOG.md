# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).


## [0.0.1] - 2025-11-01
### Fixed
- Fixed

### Added
- Baseline

### Changed
- Replace

### Security
- Fix

## [0.0.2] - 2025-11-23
### Added
- NovaTenant/NovaProject/NovaApp CRDs reconciled into Capsule, Capsule Proxy, and KubeVela.
- Capsule Proxy publishing via API when PROXY_API_URL is provided, with ConfigMap fallback.
- Postgres migrations and health checks for manager readiness.
- Quickstart docs updated for v0.0.2 surface and token/tenant/app flows.

### Changed
- OpenAPI version bumped to 0.0.2; reconcilers consume CRDs instead of ConfigMaps.
