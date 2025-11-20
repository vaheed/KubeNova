# KubeNova PaaS Roadmap

This roadmap turns the existing Manager‑Agent control plane into a full multi‑tenant PaaS without sacrificing the API‑first, Capsule/KubeVela architecture. Each phase ends in a shippable surface (API + persisted store + Agent behavior) so future teams can build CLI/console workflows on a stable base.

## Phase 0 – Tenancy Model (done)

### Goal
Establish the safety boundaries tenants expect before any catalog or source work: read-only App namespaces managed by the platform, full-edit sandboxes for developers, predictable kubeconfigs via Capsule, and a manager-driven namespace model.

### Deliverables
1. **Namespace model**
   - App namespaces follow `tn-<tenant>-app-<project>` so every App maps to exactly one namespace.
   - Sandbox namespaces (`tn-<tenant>-sandbox-<name>`) can be created through the new `/api/v1/tenants/{t}/sandbox` handler; they are labeled `kubenova.io/sandbox=true` so the Agent ignores them.
2. **Capsule + RBAC integration**
   - Capsule Tenants now declare `allowedGroups = <tenant>-devs` and the manager ensures every tenant namespace carries the right labels/quotas.
   - App namespaces bind `<tenant>-devs` to a read-only `ClusterRole` (`kubenova-app-reader`); sandbox namespaces bind the same group to the full-edit `kubenova-sandbox-editor` role.
3. **Manager logic**
   - Manager auto-creates namespace/RBAC/RoleBindings for App and sandbox namespaces.
   - Tenant kubeconfigs continue to go through Capsule-proxy, but they target manager-controlled service accounts with read-only scope inside App namespaces and full-edit scope within sandboxes.

### Milestones
- [x] Namespace naming, labels, and Agent changes complete.
- [x] Capsule tenant templates now include `allowedGroups`.
- [x] Manager issues sandbox kubeconfigs + stores sandbox metadata.
- [x] RBAC cluster roles + bindings enforced for app vs sandbox.

### Status
Phase 0 is complete; the stack is ready to focus on Phase 1 (App sources, UUID normalization, and catalog work).

## Phase 1 – App Source Model & UUID Baseline

### Goal
Ensure the Manager remains the source of truth for Apps while supporting multiple deploy sources. At the same time, eliminate legacy `uid` payloads so every API response, store column, ConfigMap label, and generated type flows through a canonical `id` (UUIDv4). Backward compatibility is intentionally broken in this phase to unblock new sources quickly.

### Deliverables
1. **API Contract**
   - Update `App` schemas in `docs/openapi/openapi.yaml` to emit `id` (format: uuid) in place of `uid` for every resource, keeping route parameters as-is.
   - Extend `AppSpec` with `source` object, `catalogRef`, and per-kind credential references supporting `helmHttp`, `helmOci`, `velaTemplate`, `kubeManifest`, `containerImage`, and `gitRepo`.
   - Regenerate `knapi` types/servers after the OpenAPI change.
2. **Go Models & Store**
   - Align `pkg/types` with the new schema: remove `UID` fields or deprecate them internally, expose `ID types.ID` on all DTOs, and ensure App/spec payloads round-trip via JSONB columns.
   - Update store interfaces + Postgres/memory implementations to use `ID`/`id` (UUID) everywhere; plan migrations that drop `uid` columns and rename them to `id` with default `gen_random_uuid()`.
   - Persist `apps.spec` JSONB with fields for description, components, traits, policies, and the new `source` tree (including `credentialsSecretRef` references).
3. **Config & Agent**
   - When projecting KubeVela Applications via ConfigMaps, tag them with `kubenova.io/app-id`, `kubenova.io/project-id`, `kubenova.io/tenant-id`, and `kubenova.io/source-kind` so the Agent/Manager can correlate and enforce ownership.
   - Agent-driven renderers should translate each source kind into a Vela Application: container images map to a web service component (ports/env/resources), Helm sources to Helm components, etc.
4. **UUID Normalization Note**
   - Document the scope change in `roadmap.md` (this file), `README.md`, and `docs/index.md` so contributors know that API payloads now emit `id` only and the database stores UUID columns rather than `uid`.

### Milestones
- [ ] OpenAPI + Go type adjustments complete.
- [ ] Postgres migration script finalised (`uid` -> `id`, JSONB spec column).
- [ ] ConfigMap label changes + Agent spec renderer validated via unit tests.
- [ ] Regenerated clients/tests use helper `uidStr` → `idStr` (or similar).

## Phase 2 – Local App Store / Catalog

### Goal
Give tenants a catalog of curated App templates with RBAC scope (global/tenant/project) and a managed install path that creates App records via existing mechanisms.

### Deliverables
1. **Data Model**
   - `catalog_items` table with `id UUID PRIMARY KEY`, `slug`, `name`, `description`, `icon`, `category`, `version`, `maintainer`, `scope (enum)`, optional `tenant_id`, JSON `source`, `created_at`, `updated_at`.
   - JSON payload mirrors App source model (`source.kind`, nested helm/git/image descriptors, credential refs).
2. **Service + API**
   - `/api/v1/catalog` resource for list/get (global + tenant scope), CRUD (global for admin/ops, tenantOwner for tenant-scoped).
   - `/api/v1/clusters/{cluster}/tenants/{tenant}/projects/{project}/catalog/install` to instantiate `App` records from catalog items and optional overrides.
   - Enforce RBAC: `tenantOwner` can manage tenant scope; `projectDev` can install but not mutate catalog items; `readOnly` can list only.
3. **Implementation**
   - Merge catalog overrides into App source before persisting; store the `catalogRef` back on the App for traceability.
   - Support version bumps by allowing re-install with new `version` string and updating the App’s spec (Vela will track revision).

## Phase 3 – Install + Upgrade Flow

### Goal
Make catalog installs first-class (App creation + Agent reconciliation) and prepare for future upgrade semantics.

### Deliverables
1. `catalog/install` handler creates App in target project, triggers existing deployment flow, stores install metadata (e.g. `catalog_item_id`, `catalog_version`, overrides) in App spec.
2. Provide a simple upgrade path: POST install with new `version` updates App spec, causing Agent to re-render Vela Application (and Vela handles revision history).
3. Document how apps can be managed via both Manager API and tenant kubeconfigs (labels/annotations for owner correlation).

## Phase 4 – Kubectl Path & Discovery

1. Define label convention (`kubenova.io/app-id`, `project-id`, `tenant-id`, `source-kind`) on every Vela Application the Agent manages.
2. Agent discovers Vela Applications created directly via kubectl when they carry the labels; optionally surface them in Manager’s App.list results (read-only reflection).
3. When Manager App is deleted, Agent removes the corresponding Vela Application; orphaned Vela Applications are surfaced via status.

## Phase 5 – Credentials & Registries

1. Introduce `SecretRef` objects (name + namespace) for Helm repos, OCI registries, and container images—never persist raw credentials.
2. Document how to pre-create secrets in target namespaces (e.g. `docker-registry`, basic auth) and reference them in App/Catalog payloads.
3. Ensure Agent pushes secret references into Vela components (imagePullSecrets, Helm repo auth, etc.).

## Phase 6 – Tests & Validation

1. Unit tests for store rendering, service layer conversions per source kind, catalog RBAC, install handler.
2. Update existing HTTP tests to expect `id` fields; add helpers for stringifying UUIDs consistently.
3. Add integration test (e.g. Kind-based smoke) that registers cluster, tenant/project, and installs a catalog item (nginx container) verifying Vela Application + Pod existence.

## Phase 7 – Docs, Quickstarts & DX

1. Update `README.md` and `docs/index.md` with:
   - Full PaaS narrative (kubectl path vs Manager API path).
   - App source definitions and credential guidance.
   - Local app store usage and install payloads (WordPress Helm example, container image example).
   - Kubectl discovery flow (how to label Applications and how Manager sees them).
2. Provide JSON examples for every new API path (catalog list, install, App spec).
3. Update `docs/site` and OpenAPI examples to match payloads.

## Cross-Cutting – UUID Normalization

1. Remove all `uid` fields from responses; replace with `id types.ID` typed as `uuid.UUID` (Go) and `format: uuid` in OpenAPI.
2. Store tables (`clusters`, `tenants`, `projects`, `apps`, `catalog_items`, `policysets`) should use UUID PKs with default `gen_random_uuid()` and `uuid`-typed foreign keys. Drop legacy `uid` columns and update FK constraints accordingly.
3. Regenerate `internal/http/knapi` code and client helpers to emit/consume `id`.
4. Document the breaking change (IDs, `id` usage) in README/docs so integrators are aware.

## Next Steps for Phase 1

1. Finalize the OpenAPI schema (App spec + `id` field) and regenerate server/client types.
2. Update store/migrations to persist the new `spec` JSONB and convert `uid` → `id`.
3. Adjust handlers/tests to parse string IDs from paths/bodies, rename helpers (e.g. `idStr`), and ensure ConfigMaps include new labels.
4. Follow Phase 2 once Phase 1 is validated with tests and schema updates in place.
