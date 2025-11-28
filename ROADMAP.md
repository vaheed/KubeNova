# KubeNova Roadmap (v0.1.2 baseline)

## Completed / Available
- Core manager HTTP API (`/api/v1`) with structured errors, optional JWT/RBAC, and OpenAPI v0.1.2.
- Postgres-backed store with in-memory fallback for tests; health/readiness checks fail fast when `DATABASE_URL` is missing.
- Operator bootstrap pipeline with bundled Helm + charts for operator/cert-manager/Capsule/Capsule Proxy/KubeVela/FluxCD/Velaux; periodic reconcile loop.
- Nova CRDs (tenant/project/app) and controller-runtime scaffolding with Capsule/Vela adapters.
- Dev ergonomics: docker-compose stack, kind assets + helper script, VitePress docs, cleaned env example, Helm charts versioned at v0.1.2.

## In Progress
- Harden Postgres migrations and persistence for clusters/tenants/projects/apps/usage.
- End-to-end reconciliation from Nova CRDs to Capsule/Vela with status surfacing and kubeconfig issuance.
- Auth/RBAC hardening across manager API and capsule-proxy integration; envelope encryption for stored secrets.
- Observability/CI hardening: OTEL traces, Prometheus metrics, gosec/trivy/staticcheck in CI.

## Next Up
- Plan and policy catalog ingestion with `/plans` APIs and defaulting on tenant creation.
- Usage ingestion from operators (hourly) feeding tenant/project usage endpoints and billing exports.
- Release automation: signed images, chart publishing, changelog automation per tag.
- Multi-cluster upgrade playbooks, failure-mode drills (proxy loss, network partitions), and performance testing.

## Testing focus
- Live API integration against kind (see `docs/operations/kind-e2e.md` and `internal/manager/live_api_e2e_test.go`).
- Operator bootstrap/upgrade validation via `/bootstrap/{component}` actions and upgrade runbook.
