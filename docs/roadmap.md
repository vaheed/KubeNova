# KubeNova Roadmap (0 → 100)

This roadmap turns the RFC into concrete milestones from current skeleton to a production-ready 1.0. Each milestone lists scope, deliverables, and acceptance criteria.

## Milestone 0.1 – API & Store Foundations
- Scope: Fully implement `/api/v1` routes in `internal/manager` per OpenAPI; enforce KN-### error shapes; JWT/RBAC guardrails.
- Deliverables: Complete OpenAPI examples, handler coverage, Postgres migrations with health checks, request/trace logs, Prom metrics for HTTP/store.
- Acceptance: `go fmt/vet/build/test` pass; readyz checks store + deps; 100% implemented paths match OpenAPI examples.

## Milestone 0.2 – CRDs & Operator Plumbing
- Scope: Define NovaTenant/NovaProject/NovaApp CRDs with validation/defaults/status; publish CRDs under deploy/; reconcile CRDs → Capsule/Capsule Proxy/KubeVela.
- Deliverables: CRD YAMLs, controller-runtime reconcilers with status updates, Capsule Tenant and Vela Application translations, Capsule Proxy publish via API (fallback configmap).
- Acceptance: Fake/e2e tests proving tenant/project/app reconciliation; CRDs installed and reported Ready; manager sees status updates.

## Milestone 0.3 – Plans, PolicySets, and Bootstrap
- Scope: Load `pkg/catalog` plans/policysets; expose `/plans` endpoints; apply defaults on tenant creation; bootstrap flows for clusters/components.
- Deliverables: Plan application logic, bootstrap status transitions (`pending_bootstrap` → `bootstrapping` → `connected`), capability flags.
- Acceptance: New tenants get default plan when set; bootstrap action updates cluster status; capability queries reflect installed components.

## Milestone 0.4 – Usage, Billing, and Kubeconfigs
- Scope: Operator aggregates usage hourly; manager ingests and stores usage; scoped kubeconfig issuance for tenants/projects via Capsule Proxy endpoints.
- Deliverables: Usage ingestion API/storage, `/usage` endpoints (tenant/project), kubeconfig generator with TTL/roles/groups.
- Acceptance: Usage visible via API; kubeconfigs download with correct namespaces/groups; auth works via proxy.

## Milestone 0.5 – Security & Compliance
- Scope: Envelope encryption for stored kubeconfigs/secrets, audit logging, rate limiting/idempotency, mTLS to proxy API.
- Deliverables: Encryption utilities applied, audit log stream, configurable request limits, documented key rotation runbook.
- Acceptance: Secrets at rest encrypted; audit trail covers mutating calls; load-tested rate limiting; mTLS validated in staging.

## Milestone 0.6 – Observability & SRE
- Scope: Deep metrics/traces for manager and operator; alerting SLOs; leader election/HA; retry/backoff policies documented.
- Deliverables: Prometheus dashboards, OpenTelemetry traces, runbooks for outages, leader election tuning.
- Acceptance: Dashboards populated in staging; chaos tests show graceful recovery; SLOs defined and monitored.

## Milestone 0.7 – CI/CD & Release Engineering
- Scope: CI for fmt/vet/build/test/staticcheck/gosec/trivy; Helm charts for manager/operator; image builds & signing; automated changelog/tag flow.
- Deliverables: GitHub Actions (or equivalent), Helm lint gates, release scripts, signed images.
- Acceptance: Green CI on PRs; `helm install` works locally; release tags produce artifacts and changelog entries.

## Milestone 0.8 – UX & Docs
- Scope: Expand docs (quickstart, CRD guides, troubleshooting, API examples per path); diagrams from RFC; public site build.
- Deliverables: Complete `docs/index.md` coverage, CRD how-tos, diagrams in `docs/diagrams/`, published site (VitePress or similar).
- Acceptance: Docs match shipped behavior; every API path has an example payload; diagrams updated with each milestone.

## Milestone 0.9 – Hardening & Scale
- Scope: Load/soak tests; perf tuning for Postgres and operator; multi-datacenter validation; failure-mode drills (network partitions, proxy loss).
- Deliverables: Benchmarks, scale test reports, tuned connection pools, backpressure and retry policies refined.
- Acceptance: Meets target throughput/latency; no data loss on simulated failures; documented limits and guidance.

## Validation Runbook (applies to milestones 0.2+)
- Register cluster via manager `/api/v1/clusters` (with kubeconfig) → `/bootstrap/operator`.
- Operator installs cert-manager, Capsule, Capsule Proxy, KubeVela (charts baked at `/charts` or pulled with `HELM_USE_REMOTE=true`).
- Check readiness: deployments in `kubenova-system` READY; Nova CRDs show `Ready` conditions; Capsule Tenant and Vela Application objects are created for Nova CRDs.
- Status surfaces: manager cluster/tenant/project/app endpoints return status/conditions; CRDs’ status updated by reconcilers.
- Upgrade: bump chart versions/operator image, apply manifests, watch readiness; ensure backward compatibility on API/CRD versions.

## Milestone 1.0 – Production GA
- Scope: Final polish, backward compatibility guarantees, upgrade guides, support SLAs.
- Deliverables: GA tag, migration guide, support policy, security review sign-off.
- Acceptance: All previous milestones met; upgrade path tested; GA announcement ready.
