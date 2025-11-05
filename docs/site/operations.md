# Operations

- Scale Agent: Adjust HPA or deployment replicas in `internal/cluster/manifests` or helm values.
- Upgrade Agent: Set `env.AGENT_IMAGE` for the API helm chart and roll the Agent.
- Rollback: Reapply a prior Agent image; helm job installs add-ons idempotently.
- Health Checks: `/healthz` and `/readyz` on both Manager and Agent.
- Metrics: `/metrics` Prometheus endpoint on Manager; Controller-runtime metrics on Agent.
## API Migration to /api

- The new contract-first API is served by the generated router. To migrate fully to `/api` and deprecate legacy handlers, use:
```
export KUBENOVA_NEW_API=1
export KUBENOVA_NEW_API_PREFIX=""   # mount at /
export KUBENOVA_DISABLE_LEGACY=1     # disable legacy /api/v1 handlers
```
- Legacy routes (when enabled) carry `Deprecation: true` and a `Sunset` header to encourage migration.
- The OpenAPI spec remains available at `/openapi.yaml`.
