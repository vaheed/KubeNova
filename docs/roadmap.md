# KubeNova PaaS Roadmap
Manager → Agent → Capsule → Capsule-Proxy → KubeVela Architecture  
One cluster per zone/datacenter.  
Includes multi-namespace tenancy model and “1 namespace per App”.

---

# Phase 0 — Tenancy Model 

## Goal
Establish a safe, predictable multi-tenant model across clusters:
- **App namespaces** → managed by KubeVela, read-only for tenants  
- **Sandbox namespaces** → tenant-owned, free-edit, isolated from platform  
- **Each App = exactly 1 namespace**  
- **Each Project = unlimited sandbox namespaces**  
- **One Capsule + Capsule-Proxy per cluster**  
- **Manager assigns apps to clusters explicitly (no multi-cluster magic)**  

This ensures no drift, clean RBAC, and a frictionless kubectl experience.

---

## Deliverables

### 0.1 Namespace Model (per cluster)
For tenant `acme`, project `shop`, apps `frontend`, `backend`:

- **App namespaces** (one per app):
  - `tn-acme-shop-app-frontend`
  - `tn-acme-shop-app-backend`
- **Sandbox namespaces**:
  - `tn-acme-shop-sbx-001`
  - `tn-acme-shop-sbx-002`
- Namespaces always created by the Manager (not users).

Manager rules:
- On App creation → generate deterministic namespace name.
- On Project creation → allow sandbox creation but not app creation directly.

---

### 0.2 Capsule Tenant Integration (per cluster)
- `allowedGroups = <tenant>-devs`
- Capsule Tenant governs:
  - namespace selection (`tn-<tenant>-*`)
  - quotas (cpu/mem/storage)
  - node selectors / topology
- Capsule-Proxy authenticates users and routes to Capsule Tenant.

Manager responsibilities:
- Create Capsule Tenant if missing.
- Sync Capsule Tenant updates across all clusters where tenant exists.

---

### 0.3 RBAC Split 
Two ClusterRoles deployed per cluster:

#### **A) Read-only for App namespaces**
Verbs:
- `get`, `list`, `watch`
- `pods/exec`, `pods/log`, `attach`
Forbidden:
- `create`, `update`, `patch`, `delete`

Bound to:
- Tenant group `<tenant>-devs`
- Only inside **app namespaces**

Ensures apps are managed *only* through Manager → Agent → Vela.

---

#### **B) Full-edit for Sandbox namespaces**
Verbs:
- `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`
Resources:
- pods, deployments, services, ingresses, cm, secrets, jobs, etc.

Bound to:
- Tenant group `<tenant>-devs`
- Only inside **sandbox namespaces**

Sandbox = playground for kubectl power users.

---

### 0.4 Manager Logic (Global API)
Manager enforces all rules:

- App creation:
  - Validates cluster assignment.
  - Creates app namespace in target cluster.
  - Creates RoleBinding (read-only).
  - Generates Vela template via Agent.
- Sandbox creation:
  - `/clusters/{cluster}/tenants/{tenant}/projects/{project}/sandbox`
  - Creates sandbox namespace.
  - Creates RoleBinding (full-edit).
  - Never used by Vela or the platform.

- Enforcement:
  - App must always have exactly 1 app namespace.
  - Sandbox namespaces must never contain Vela Applications.

---

### 0.5 Optional Hardening (later)
- Add label to all Vela-managed resources:
  - `kubenova.io/managed-by=vela`
  - `kubenova.io/app-id=<uuid>`
- Kyverno policies (optional):
  - deny edits to Vela-managed resources
  - allow edits only in sandbox namespaces

---

## Implementation status
- Namespace naming and RBAC are handled by `internal/cluster/namespaces.go`, `internal/cluster/projects.go`, and `internal/cluster/rbac.go`, which create `tn-<tenant>-app-<project>` and `tn-<tenant>-sandbox-<name>` namespaces, keep Capsule labels in sync, enforce the app vs sandbox ClusterRoles, and apply quotas/limits per namespace.
- Capsule tenants, quotas, and kubeconfigs live under `internal/backends/capsule` and `internal/cluster/kubeconfig.go`; the HTTP surface in `internal/http/server.go` issues project (readOnly/projectDev) and sandbox (tenantOwner) kubeconfigs, writes `kubenova.io/{app,tenant,project}-id` metadata, and exposes the sandbox API that is exercised by `internal/http/server_policysets_test.go`, `internal/http/server_apps_ops_test.go`, and `internal/http/server_sandbox_test.go`.
- The Agent (`internal/reconcile/project.go` and `internal/reconcile/app.go`) ignores namespaces labeled with `kubenova.io/sandbox=true`, keeps Capsule tenants up to date, and projects only app-configured ConfigMaps into Vela so sandbox namespaces stay isolated from managed workloads.

## Milestones
- [x] Namespace naming rules implemented in Manager  
- [x] RBAC ClusterRoles installed per cluster  
- [x] Manager auto-creates RoleBindings  
- [x] Capsule Tenant templates updated  
- [x] Tenant kubeconfig tested for RO(App) and RW(Sandbox)  
- [x] Agent updated to ignore sandbox namespaces  
- [ ] Full multi-tenant flow validated in Kind/multi-cluster setup  

---

# Phase 1 — App Source Model & UUID Baseline

## Goal
Replace legacy `uid` with canonical `id` (UUIDv4) everywhere.  
Unify all deployment methods into **App.source**.  
Ensure Manager is the **only writer** to app namespaces across clusters.

---

## Deliverables

### 1. API Contract
- Update `docs/openapi/openapi.yaml`:
  - All resources emit `id` (UUID).
  - Remove `uid` fields from App, Tenant, Project, Cluster structs.
  - Add unified `source` object with variants:
    - `helmHttp`
    - `helmOci`
    - `gitRepo`
    - `containerImage`
    - `kubeManifest`
    - `velaTemplate`
  - Add `catalogRef`.
- Regenerate `knapi` server + client.

---

### 2. Go Models & Store
- Remove `UID` fields; expose `ID types.ID`.
- Update Postgres schemas:
  - Rename `uid` → `id`
  - Default: `gen_random_uuid()`
  - Migrate FK relations.
- Store JSONB `spec` containing:
  - components
  - traits
  - source definition
  - credentials refs

---

### 3. Config & Agent
- ConfigMap labels added:
  - `kubenova.io/app-id`
  - `kubenova.io/project-id`
  - `kubenova.io/tenant-id`
  - `kubenova.io/source-kind`
- Agent responsibilities:
  - Deserialize App.source
  - Render Vela Application spec
  - Apply into app namespace
  - Never touch sandbox namespaces

--- 

### 4. UUID Normalization Note
Document breaking changes in:
- `README.md`
- `docs/index.md`
- `docs/roadmap.md`

---

## Implementation status
- OpenAPI + generated clients emit `id` UUIDs and the expanded `App.source` variants, keeping the HTTP schema and DTOs in sync with the new contract (`docs/openapi/openapi.yaml`, `internal/http/knapi_types.gen.go`, `internal/http/knapi_server.gen.go`).
- Go models and stores rely on `types.ID`; Postgres/memory layers insert/query the `id` columns, including the serialized App spec payload, so UUIDs propagate throughout the persistence layer (`pkg/types/types.go`, `internal/store/postgres.go`, `internal/store/memory.go`).
- HTTP handlers, policy set helpers, and the Agent reconciler now use the canonical IDs, pass the updated metadata labels, handle App.source decoding, and continue ignoring sandbox namespaces while projecting apps to Vela (`internal/http/server.go`, `internal/reconcile/app.go`).

---

## Milestones
- [x] OpenAPI updated  
- [x] Go models updated  
- [x] Store migration (`uid` → `id`) complete  
- [x] Agent Vela renderer updated  
- [x] Tests use new UUID fields  
- [ ] Manager + Agent E2E validated  

---

# Phase 2 — Local App Store / Catalog

## Goal
Provide curated templates with scope: global, tenant, or project.  
Users install apps directly into their projects → Manager creates App → Agent deploys Vela.

---

## Deliverables

### 1. Data Model
Table: `catalog_items`

Status: implemented via `db/migrations/0003_catalog_items.sql`.

Fields:
- `id UUID PK`
- `slug`
- `name`
- `description`
- `icon`
- `category`
- `version`
- `scope`: `global` | `tenant`
- `tenant_id` (nullable)
- JSONB `source`
- timestamps

App.source and Catalog.source share same schema.

---

### 2. API
- `/api/v1/catalog`:
  - list/get
  - CRUD (admin, tenantOwner)
- Install API:
  `/api/v1/clusters/{cluster}/tenants/{tenant}/projects/{project}/catalog/install`

Status: documented in `docs/openapi/openapi.yaml` and exercised by `internal/http/server.go`.

RBAC:
- admin → full CRUD  
- tenantOwner → CRUD for tenant-scope  
- projectDev → install only  
- readOnly → list only

---

### 3. Implementation
- Manager merges catalog item source with user overrides.
- Manager creates App row with merged source.
- Manager creates app namespace (if missing).
- Store `catalogRef` on App for rollback/upgrade visibility.

### Implementation status
- `db/migrations/0003_catalog_items.sql` defines the `catalog_items` schema that stores slug, scope, source JSONB, and timestamps.
- `internal/store/store.go`, `internal/store/postgres.go`, and `internal/store/memory.go` expose `Create/List/GetCatalogItem` so handlers can persist tenant/global entries.
- `docs/openapi/openapi.yaml` now exposes `/api/v1/catalog`, `/api/v1/catalog/{slug}`, and `/clusters/{c}/tenants/{t}/projects/{p}/catalog/install` plus the `CatalogItem`/`CatalogInstall` payloads.
- `internal/http/server.go` merges catalog overrides into `AppSpec.Source`, stamps a `catalogRef`, and mirrors the metadata via `ensureAppConfigMap`; `internal/http/server_catalog_test.go` covers the install flow.
- `docs/index.md` now walks through catalog listing and install commands so the documentation matches the shipped API.

---

### Sandbox Restriction
- Catalog install → **must target app namespaces only**.
- Installing into sandbox namespace → invalid.

---

# Phase 3 — Install + Upgrade Flow

## Goal
Make catalog installs first-class.  
Support version upgrades and predictable rollouts.

---

## Deliverables
- Install:
  - Create App row
  - Create app namespace if missing
  - Agent deploys Vela Application
- Upgrade:
  - `POST /install` with new version
  - Manager updates App.source.version
  - Agent re-renders Vela spec
  - Vela handles revisions

- Metadata stored:
  - `catalog_item_id`
  - `catalog_version`
  - `overrides`

### Implementation status
- Catalog installs persist the catalog ID, version, and overrides inside `AppSpec` so upgrades carry the metadata agents need.
- `internal/http/server_catalog_test.go` exercises the install/upgrade loop, ensuring repeated installs keep a single App and refresh the overrides.
- `AppSpec` and the OpenAPI contract now expose `catalogItemId`, `catalogVersion`, and `catalogOverrides` so consumers can read the stored metadata.

---

# Phase 4 — Kubectl Path & Discovery

## Goal
Support kubectl directly, safely, without drift.

---

## Deliverables
- Label conventions applied to all Vela Applications:
  - `kubenova.io/app-id`
  - `kubenova.io/tenant-id`
  - `kubenova.io/project-id`
  - `kubenova.io/source-kind`
- Manager lists Vela Applications by label filters.
- Orphan detection:
  - Vela Application exists but App record missing.
- Sandbox namespaces labeled:
  - `kubenova.io/sandbox=true`
- Manager displays sandbox namespaces separately (non-Vela-managed).

---

# Phase 5 — Credentials & Registries

## Goal
Secure cross-cluster credential handling for Helm, OCI, and private registries.

---

## Deliverables
- `SecretRef` object:
  - `name`
  - `namespace`
- No raw credentials stored in DB.
- Agent injects secretRef into:
  - imagePullSecrets
  - Helm repo auth
  - Git repo auth
- Documentation for required secrets:
  - docker-registry secrets
  - ssh keys for Git
  - helm repo credentials

Sandbox:
- Tenants may create secrets freely in sandbox namespaces.
- Manager never uses sandbox secrets for App deployments.

---

# Phase 6 — Tests & Validation

## Deliverables
### Unit Tests
- App.source → Vela rendering  
- Catalog RBAC  
- Install/upgrade handlers  
- Store round-trip consistency  

### Integration Tests (Kind)
- Register cluster  
- Create tenant  
- Create project  
- Create App → namespace created  
- Install catalog item → Vela Application exists  
- Pod/Service exist  
- Deletion flows validated  

### RBAC Tests
- Read-only in app namespaces  
- Full-edit in sandbox namespaces  
- Capsule + Capsule-proxy integration tested  

---

# Phase 7 — Docs, Quickstarts & DX

## Deliverables
- Update `README.md`:
  - Core concepts
  - One cluster per zone  
  - Capsule + Capsule-proxy requirements
- Update `docs/index.md`:
  - Namespace model
  - Tenant → Project → App hierarchy
  - App.source examples
  - Sandbox usage guide
- Add JSON examples for:
  - catalog items  
  - App creation  
  - upgrades  
  - secretRef usage  
- Quickstart:
  - Register cluster  
  - Create tenant  
  - Create project  
  - Create sandbox  
  - Install nginx from catalog  
  - Upgrade version  

---

# Cross-Cutting — UUID Normalization

## Deliverables
- Remove all `uid` fields from:
  - clusters  
  - tenants  
  - projects  
  - apps  
  - catalog_items  
  - policysets  
- Use UUID PKs consistently.
- Update foreign keys and DB constraints.
- Regenerate `internal/http/knapi`.
- Document migration instructions.

---

# Final Architecture Summary

## Control Plane Layer
- **Manager**  
  Source of truth (Apps, Tenants, Projects, Catalog)  
  Decides cluster placement for each App  

- **Agent (per cluster)**  
  Renders Vela Applications  
  Applies changes into app namespaces only  
  Ignores sandbox namespaces  

- **KubeVela**  
  Application engine responsible for workloads  
  Hands-off for sandbox namespaces  

- **Capsule + Capsule-Proxy (per cluster)**  
  Multi-tenant enforcement  
  Kubeconfig access control  
  Namespace isolation  

---

## Tenant Experience Layer
- Tenant kubeconfig through Capsule-Proxy:
  - **App namespaces:** read-only  
  - **Sandbox namespaces:** full-edit  
- Deploy, upgrade, rollback apps via Manager API.
- Inspect logs, pods, exec via kubectl.
- Develop freely in sandbox without risk to platform.

---

## Result
A clear, stable, production-grade PaaS:
- No drift  
- Clean multi-cluster separation  
- Predictable kubectl behavior  
- Safe tenancy  
- Unified application delivery workflow  
- Powered by Capsule + Vela  
