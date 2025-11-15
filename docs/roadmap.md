---
title: KubeNova Roadmap (API v1)
---

# KubeNova Roadmap

This roadmap tracks bringing the current API implementation in line with `docs/index.md` and `docs/openapi/openapi.yaml`, and getting the Manager/Agent ready for production use.

## Phase 1 — Tenants & Capsule Integration

- **Persist tenant metadata** ✅
  - Store `owners` and `labels` for tenants in Postgres (not just in-memory).
  - Ensure `ListTenants` returns full metadata in both memory and Postgres stores.
  - Make `/api/v1/clusters/{c}/tenants?labelSelector=...` and `?owner=...` filters work against persisted fields.
- **Stabilize Capsule quotas/limits/netpolicies** ✅
  - Preserve `spec.resourceQuotas` when updating `limitRanges` and `networkPolicies`.
  - Store quotas in a KubeNova-owned annotation for compatibility across Capsule versions.
  - Make `/summary` stable even after multiple quota/limits/netpol updates.
- **Tenant summary** ✅
  - List namespaces belonging to a tenant (via Capsule labels).
  - Return effective quotas and (later) usage in `/summary`.

## Phase 2 — Projects → Namespaces & Access

- **Project to Namespace mapping** ✅
  - Introduce a controller that mirrors Projects from the store into real Namespaces on the target cluster.
  - Label namespaces with `kubenova.project` and `capsule.clastix.io/tenant` for Capsule and reporting.
- **Project access & RBAC** ✅
  - Implement `PUT /projects/{p}/access` to create/update Roles and RoleBindings in the project namespace.
  - Map roles (`tenantOwner`, `projectDev`, `readOnly`) to concrete RBAC rules.
- **Scoped project kubeconfig** ✅
  - Replace the current “raw cluster kubeconfig” stub with a project-scoped kubeconfig from capsule-proxy.
  - Ensure project kubeconfigs cannot list or mutate resources outside their namespace.
- **Tenant/Project → Cluster mapping** ✅
  - Record the primary cluster UID on tenants created via `/clusters/{c}/tenants` for usage and kubeconfig resolution.
  - Use this mapping when computing `usage` and kubeconfigs, falling back to the first cluster only for legacy tenants.

## Phase 3 — Usage & Metrics

- **Metrics ingestion** ✅
  - Use Kubernetes `ResourceQuota` status (or hard limits) as the source of CPU/memory/pods usage per namespace.
  - Compute tenant and project usage on demand from the target cluster using the stored kubeconfig.
- **Usage endpoints** ✅
  - Implement `GET /api/v1/tenants/{t}/usage` using live cluster data when available, falling back to stub values for tests/dev.
  - Implement `GET /api/v1/projects/{p}/usage` using live cluster data when available, falling back to stub values for tests/dev.
  - Optionally surface usage aggregates in tenant `/summary` in a future iteration.

## Phase 4 — PolicySets & Catalog

- **PolicySets persistence** ✅
  - Persist PolicySets in Postgres or a CRD instead of in-memory maps.
  - Wire `GET/POST/PUT/DELETE /tenants/{t}/policysets` to the persistent store.
- **PolicySet catalog** ✅
  - Move the hard-coded PolicySet catalog into data (JSON-backed config).
  - Serve `/clusters/{c}/policysets/catalog` from `docs/catalog/policysets.json`, with a safe built-in fallback.
  - Allow the catalog to be extended without code changes to the manager.

## Phase 5 — Auth, RBAC & Dev/Prod Parity

- **Auth & “me” endpoint**
  - Make `GET /api/v1/me` return the real subject from JWT (`sub`) and effective roles.
  - Align token issuance (`/tokens`) with production RBAC expectations.
- **Readiness & health**
  - Extend `/readyz` to check DB, critical external services, and migration status.
- **Dev vs production behavior**
  - Reduce stubs where behavior differs significantly (kubeconfigs, usage, quotas/limits synchronization).
  - Document remaining dev-only shortcuts (if any) clearly in `docs/index.md`.

## Phase 6 �?" Provider Integrations (CaaS / PaaS)

- **Apps reconciliation (AppReconciler placeholder)**
  - Promote `internal/reconcile/AppReconciler` from a ConfigMap-driven placeholder to a production-grade controller that projects the KubeNova App model onto real Vela `Application` and `Workflow` resources.
  - Define clear contracts for how app specs, traits, and policies flow from the Manager into the CaaS app-delivery layer.
- **Proxy backend (kubeconfig issuance via capsule-proxy)**
  - Replace the noop `internal/backends/proxy` client (which currently issues placeholder kubeconfigs) with a real integration against capsule-proxy or the configured access proxy for scoped kubeconfig/token issuance.
  - Align this backend with the JWT/group semantics already used by the Manager so that different CaaS/PaaS providers can plug in their own proxy endpoints.
- **Placeholder cleanup**
  - Track and remove remaining non-test placeholders as platform features are implemented, keeping this roadmap and the docs in sync with actual behavior.
