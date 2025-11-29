---
title: Configuration
---

# Configuration

`env.example` is the single source of truth for required environment variables. Copy it to `.env` and edit before running the manager or operator. Docker Compose reads only from `.env`; no inline defaults exist in compose files.

## Required
- `DATABASE_URL` – Postgres DSN; the manager refuses to start without it.
- `KUBENOVA_REQUIRE_AUTH` – `true|false`; when true, `JWT_SIGNING_KEY` is mandatory.
- `JWT_SIGNING_KEY` – HS256 signing key for issuing/verifying JWTs.

## Manager / operator connectivity
- `MANAGER_URL` – externally reachable manager URL used by the operator heartbeat and Helm bootstrap.
- `BATCH_INTERVAL_SECONDS` – operator heartbeat interval (seconds).
- `PROXY_API_URL` – Capsule Proxy API base URL (for publishing tenant endpoints).

## Observability
- `OTEL_EXPORTER_OTLP_ENDPOINT` – OTLP/gRPC or HTTP collector endpoint.
- `OTEL_EXPORTER_OTLP_INSECURE` – set true for HTTP endpoints.
- `KUBENOVA_ENV` – environment tag in traces (dev|staging|prod).
- `KUBENOVA_VERSION` – version reported in traces/metrics (default `v0.1.2`).
- `OTEL_RESOURCE_ATTRIBUTES` – optional comma-separated attributes.
- `TELEMETRY_SPOOL_DIR` – local directory where the operator persists telemetry events when the manager is unreachable (defaults to `$TMPDIR/kubenova/telemetry`).

## Bootstrap & addons
- Charts baked into images: operator chart at `/charts/operator`; Helm bundled in manager image.
- Overrides:
  - Versions: `CERT_MANAGER_VERSION`, `CAPSULE_VERSION`, `CAPSULE_PROXY_VERSION`, `VELA_VERSION`, `FLUXCD_VERSION`, `VELAUX_VERSION`, `VELA_CLI_VERSION`.
  - Repos: `VELAUX_REPO`, `FLUXCD_REPO`, `OPERATOR_REPO`.
  - Toggle installs: `BOOTSTRAP_CERT_MANAGER`, `BOOTSTRAP_CAPSULE`, `BOOTSTRAP_CAPSULE_PROXY`, `BOOTSTRAP_KUBEVELA`, `BOOTSTRAP_FLUXCD`, `BOOTSTRAP_VELAUX`.
  - Source selection: `HELM_CHARTS_DIR` (local charts path), `HELM_USE_REMOTE=true` to pull charts instead of using baked charts.
  - Reconcile cadence: `COMPONENT_RECONCILE_SECONDS`.
  - Velaux exposure: `VELAUX_SERVICE_TYPE` (ClusterIP|NodePort|LoadBalancer) and `VELAUX_NODE_PORT` when nodePort is required.

## Proxy / network
- `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` – forwarded to Helm when installing remote charts.

## Testing aids
- `KUBERNOVA_E2E_BASE_URL`, `KUBERNOVA_E2E_KUBECONFIG` or `KUBERNOVA_E2E_KUBECONFIG_B64`, `KUBERNOVA_E2E_TOKEN`, `RUN_LIVE_E2E` – used by the live integration test (see [kind E2E setup](../operations/kind-e2e.md)).
