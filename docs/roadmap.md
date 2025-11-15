---
title: KubeNova Roadmap
---

# KubeNova Roadmap

This roadmap summarizes what is already implemented in the current KubeNova codebase (API, Manager, Agent, adapters) and outlines the next logical areas of work. It is derived from the OpenAPI contract (`docs/openapi/openapi.yaml`), the HTTP implementation (`internal/http`), reconcilers (`internal/reconcile`), and the adapters/backends.

## Delivered Capabilities (v1)

- **Core API & Storage**
  - OpenAPI-first HTTP surface at `/api/v1` with generated types and handlers wired in `internal/http`.
  - Pluggable `Store` with in-memory and Postgres implementations (`internal/store`), including:
    - Clusters, Tenants, Projects, Apps, PolicySets, and event history.
  - Manager/Agent deployment via Helm charts with configuration in `env.example` and `deploy/helm`.

- **Tenancy (Capsule Integration)**
  - Tenants modeled in the API (`Tenant`, `/clusters/{c}/tenants`) and persisted in Postgres.
  - Capsule Tenant CRs ensured from the Agent via `ProjectReconciler` and `ensureCapsuleTenant` (`internal/reconcile/project.go`).
  - Tenant filters (`labelSelector`, `owner`) implemented in the HTTP layer.

- **Projects, Namespaces & Access**
  - Projects persisted per tenant and exposed over `/clusters/{c}/tenants/{t}/projects`.
  - `ProjectReconciler` keeps Kubernetes `Namespace` state in sync with projects and labels namespaces with `kubenova.project`, `kubenova.tenant`, and Capsule tenant labels.
  - Project-level access management via `PUT /projects/{p}/access`, translating roles (`tenantOwner`, `projectDev`, `readOnly`) into concrete `Role` and `RoleBinding` objects (`internal/cluster/projects.go`).

- **Kubeconfigs & Access Proxy**
  - Cluster registration stores kubeconfigs centrally (`POST /api/v1/clusters`), then installs the Agent into the target cluster.
  - Tenant- and project-scoped kubeconfig endpoints:
    - `POST /api/v1/tenants/{t}/kubeconfig`
    - `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/kubeconfig`
  - Kubeconfigs always target the configured access proxy (`CAPSULE_PROXY_URL`), never the raw kube-apiserver, and embed JWTs with:
    - `tenant`, optional `project`, `roles`, and `exp`.
    - Role→group mapping aligned with capsule-proxy expectations (`tenantOwner`→`tenant-admins`, `projectDev`→`tenant-maintainers`, `readOnly`→`tenant-viewers`).
  - Manager helper `internal/manager/kubeconfig.go` provides a consistent JWT + kubeconfig generator for internal consumers.

- **Apps & KubeVela Integration**
  - App model and operations implemented in `internal/http`:
    - CRUD under `/clusters/{c}/tenants/{t}/projects/{p}/apps`.
    - Operations wired to KubeVela backend (`internal/backends/vela`): deploy, suspend, resume, rollback, status, revisions, diff, logs, traits, policies, image-update, and delete.
  - Agent-level `AppReconciler` (`internal/reconcile/app.go`) projects ConfigMaps labeled with `kubenova.app`/`kubenova.tenant`/`kubenova.project` and JSON `spec`/`traits`/`policies` into KubeVela `Application` resources via the Vela backend.

- **PolicySets & Plans**
  - PolicySets persisted in Postgres and exposed via `/tenants/{t}/policysets` (`internal/store`, `internal/http`).
  - Catalog-backed PolicySets and Plans loaded from `docs/catalog`, then applied to tenants:
    - Plan application attaches quotas, limit ranges, and tenant/project-scoped PolicySets.
    - PolicySets are translated into Vela traits/policies before app deployment (`applyPolicySets` in `internal/http/server.go`).

- **Usage & Metrics**
  - Usage endpoints implemented:
    - `GET /api/v1/tenants/{t}/usage`
    - `GET /api/v1/projects/{p}/usage`
  - Usage derived from Kubernetes `ResourceQuota` status via Agent helpers (`internal/cluster/usage.go`), with safe fallbacks for tests/dev.
  - Manager exports Prometheus metrics and uses a lightweight Redis-backed telemetry buffer for events/metrics/logs from Agents.

- **Auth, Roles & Readiness**
  - JWT-based auth with roles parsed from `Authorization` or `X-KN-Roles`, enforced per-tenant in the HTTP handlers.
  - `/api/v1/me` and `/api/v1/tokens` provide self-introspection and token issuance for callers, using the same role semantics as kubeconfig grants.
  - Manager and Agent expose `/healthz` and `/readyz`; Manager’s `/wait` endpoint blocks until the store is ready.
  - OpenTelemetry tracing and structured logging wired through `internal/logging` and `internal/telemetry`.

## Next Focus Areas

- **Tenant Reconciliation & External Tenancy**
  - Promote `TenantReconciler` (`internal/reconcile/tenant.go`) from a no-op placeholder to a first-class integration with the underlying tenancy controller(s) beyond Capsule, while keeping Capsule as the default.
  - Define a clear adapter interface for plugging in other multi-tenant controllers.

- **Richer App Workflows & Insights**
  - Extend workflow run tracking beyond the in-memory `/apps/{a}/workflow/run` tracer (currently stored in-memory on the Manager).
  - Surface more detailed app health, rollout status, and historical diffs in dedicated status endpoints and docs.

- **Provider Integrations & Pluggable Proxies**
  - Generalize the access proxy integration to support multiple proxy backends (not just Capsule proxy) with a consistent contract for kubeconfig and token issuance.
  - Document and validate provider-specific expectations around groups/claims and RBAC.

- **Operational Tooling & Drift Detection**
  - Add higher-level summaries and “drift” indicators for Tenants, Projects, and Apps based on observed cluster state vs. desired state in the store.
  - Provide safer clean-up and migration helpers for clusters and tenants (beyond the existing `/clusters/{c}/cleanup` endpoint).

- **Multi-Cluster & HA Enhancements**
  - Improve multi-cluster awareness (beyond the primary `kubenova.cluster` label on Tenants) for usage and app placement.
  - Document and, where needed, adjust components for HA deployments of Manager and Agent in production environments.

- **Docs & DX**
  - Keep `docs/index.md` and `docs/README.md` as the single source for end-to-end flows (curl + kubectl) and update them alongside any behavioral changes.
  - Introduce more task-oriented guides (for operators and platform teams) that explain how KubeNova, Capsule, capsule-proxy, and KubeVela fit together in real clusters.

