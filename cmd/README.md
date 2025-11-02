# cmd/

Entrypoints for KubeNova binaries.

- `api/` – Manager API service (out-of-cluster).
  - Starts HTTP API, OpenAPI docs, Prom metrics.
  - Connects to Postgres when `DATABASE_URL` is set (with retry at startup), otherwise in-memory for dev.
- `agent/` – In-cluster controller/telemetry Agent.
  - controller-runtime manager (leader election), reconcilers, telemetry.

Run locally
```
# API (memory mode)
KUBENOVA_REQUIRE_AUTH=false go run ./cmd/api
# Agent (in cluster via Helm chart normally)
```

Key env vars
- `DATABASE_URL`, `KUBENOVA_REQUIRE_AUTH`, `JWT_SIGNING_KEY`, `MANAGER_URL`.
