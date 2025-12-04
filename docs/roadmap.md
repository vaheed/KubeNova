---
title: Roadmap
---
# Kubenova Roadmap

A complete engineering roadmap for building the Kubenova multi-tenant application platform on top of Kubernetes, Capsule, and KubeVela.

---

## 1. Architecture Foundations

### 1.1 Core Concepts Definition
- Finalize terminology for: Tenant, Capsule Project, KubeVela Project, KubeVela App, Owner Kubeconfig, App Kubeconfig  
- Create architecture diagrams:
  - Cluster → Tenant → Capsule Project → KubeVela Project → KubeVela App  
- Define integration contracts between Manager ↔ Operator ↔ Capsule ↔ KubeVela.

### 1.2 Repository Structure
```
/cmd
/pkg
  /manager
  /operator
  /api
  /proxy
  /kubeconfig
  /capsule
  /kubevela
/config
/docs
```

---

## 2. Manager Development (API + Backend)

### 2.1 API Definition
- Define REST/gRPC interfaces for:
  - Tenant management
  - Project management
  - Kubeconfig generation
  - Syncing Capsule & KubeVela projects
  - Application lifecycle operations
- Add OpenAPI or protobuf definitions.

### 2.2 RBAC Layer
- Implement role model (aligned to Capsule/KubeVela + Manager API):
  - **System Owner** — manages clusters/tenants/projects; corresponds to manager roles `admin|ops`; cluster-scoped `capsule-proxy` access; can bootstrap components.
  - **Tenant Owner** — manages their tenant/projects/apps; allowed to read usage; has owner SA kubeconfig (full access in `<tenant>-owner`/`<tenant>-apps` namespaces).
  - **App User** — deploys/updates apps inside a project; maps to `projectDev`; inherits readonly SA kubeconfig and scoped Vela project access.
- Map role actions to Kubernetes + Capsule permissions:
  - Manager API routes → required roles (`admin|ops|tenantOwner|projectDev|readOnly`) documented in `internal/manager/server.go`.
  - Capsule: Tenant Owner bound to `kubenova-owner` Role (all verbs) in both tenant namespaces; App User bound to readonly Role (get/list/watch).
  - KubeVela: Vela Project per tenant; App User allowed to create/update Applications within that project; Tenant Owner allowed to manage project settings.
  - Proxy: ensure Capsule Proxy enforces namespace scoping for owner/readonly kubeconfigs; keep proxy base URL stored on the cluster.

### 2.3 Tenant Management
- CRUD for tenants (Manager API + store) including labels/owners/plan/network policies/limits.
- Quotas and limits persisted per tenant, validated in API, and projected into Capsule Tenant spec.
- Capsule/KubeVela sync:
  - Manager writes proxy endpoint on tenant (cluster-level default plus override).
  - Operator reconciles namespaces, SA + RBAC, kubeconfigs Secret, Capsule Tenant, and publishes Capsule Proxy endpoint.
- Usage reporting path defined (Operator → Manager) with API surface for `GET /tenants/{id}/usage`.

### 2.4 Project Management
- CRUD for projects (per-tenant); enforce unique names per tenant and label support.
- Bind Capsule Projects to KubeVela Projects:
  - Manager ensures NovaProject -> Capsule tenant namespaces -> Vela Project creation.
  - Operator reconciles NovaProject into Vela Project with access lists.
- Provide project-level owner/app kubeconfigs (manager endpoint `/projects/{id}/kubeconfig`); enforce role-gated access (`admin|ops|tenantOwner|projectDev|readOnly`).

### 2.5 Kubeconfig Generator
- Generate Owner and Readonly kubeconfigs from ServiceAccounts created per tenant; store in `kubenova-kubeconfigs` Secret.
- Secure token creation: prefer `TokenRequest` API; fall back to SA token Secrets; redact kubeconfig content in API responses except explicit kubeconfig endpoints.
- Auto-expiry and rotation support: background refresh (operator) and on-demand regeneration via Manager endpoint; ensure Secrets are updated when tokens change.
- Permission validation via Capsule Proxy: kubeconfigs point to the proxy base; proxy enforces namespace scoping; Manager falls back to proxy URL when Secret missing.

### 2.6 Capsule Proxy Integration
- Auto-publish proxy endpoint per tenant (Manager stores cluster proxy base; Operator publishes via PROXY_API_URL or ConfigMap).
- Ensure kubeconfigs use proxy base URL; Capsule Proxy enforces namespace scoping for owner/readonly SAs.
- Restrict readonly kubeconfig to get/list/watch; owner kubeconfig full verbs within tenant namespaces; validate via Capsule RBAC + proxy.
- Map user → tenant → project → namespace for project/app operations; document proxy behavior and failure modes in docs.

---

## 3. Operator Development (Controllers + Reconcilers)

### 3.1 CRDs
Create Kubenova CRDs:
- Tenant
- Project
- ProjectSync
- KubeConfigRequest
- CapsuleSync
- VelaSync

### 3.2 Tenant Reconciler
- Create Capsule tenant
- Create default namespaces
- Initialize Capsule Proxy
- Trigger kubeconfig creation

### 3.3 Project Reconciler
- Create Capsule Project namespaces
- Create matching KubeVela Project
- Apply ResourceQuotas and limits
- Initialize app workspace structures

### 3.4 KubeVela Integration Controller
- Sync Capsule Project ↔ KubeVela Project
- Watch Vela Application CRDs
- Report app status to Manager

### 3.5 Drift Detection
- Detect out-of-sync Capsule Projects
- Detect missing KubeVela Projects
- Auto-heal discrepancies

---

## 4. Capsule Integration Layer

### 4.1 Tenant Management
- Auto-generate Capsule Tenant
- Configure quotas and annotations
- Enforce tenant policies

### 4.2 Capsule Project Lifecycle
- Create namespaces
- Apply NetworkPolicies
- Enforce isolation
- Store metadata

### 4.3 Capsule Proxy Controller
- Build proxy config per project
- Restrict App access
- Enforce RBAC & security boundaries

---

## 5. KubeVela Integration Layer

### 5.1 Vela Project Lifecycle
- Create Vela Projects tied to Capsule Projects
- Map permissions (owner vs app user)
- Apply delivery pipeline policies

### 5.2 Vela App Lifecycle
- Watch Vela Applications
- Track revision + health
- Sync state + logs back to Manager

---

## 6. Kubeconfig Architecture

### 6.1 Owner Kubeconfig
- Full tenant access
- All project namespaces included
- Admin-level permissions

### 6.2 App Kubeconfig
- Namespace-scoped
- App-level permissions only
- Enforced via Capsule Proxy

### 6.3 Storage Model
- Encrypted token storage
- Expiry + rotation controller

---

## 7. CLI & UI (Optional but Recommended)

### 7.1 Kubenova CLI
Commands for tenants, projects, apps, kubeconfigs.

### 7.2 Web UI
- Dashboard for tenants, projects, apps
- Kubeconfig download
- Metrics + logs
- Vela App states

---

## 8. Security & Isolation

### 8.1 Tenant Isolation
- NetworkPolicies
- ResourceQuotas
- PodSecurity
- Capsule Proxy boundaries

### 8.2 API Security
- JWT/OIDC auth
- Rate limiting
- Audit logs
- Namespace validation

### 8.3 Data Security
- Encrypted kubeconfig tokens
- Multi-tenant audit trails

---

## 9. Deployment Model

### 9.1 Installation
- Helm chart for Manager
- Helm chart for Operator
- CRDs versioning

### 9.2 Monitoring
- Prometheus metrics
- Grafana dashboards

### 9.3 Logging
- Structured logging
- Kubernetes Events
- Tracing (OpenTelemetry)

---

## 10. Release Phases

### Phase 1 — Core Framework
- Manager API skeleton
- Tenant CRUD
- Project CRUD
- Capsule + Vela sync (basic)
- Kubeconfig generator

### Phase 2 — Operator Stability
- Full reconcilers
- Vela App watcher
- Capsule Proxy enforcement
- Auto-heal + drift detection

### Phase 3 — App Management
- App lifecycle endpoints
- Vela revision tracking
- Project observability

### Phase 4 — UI & CLI
- CLI release
- Web UI release
- Alerts + metrics

### Phase 5 — GA
- Performance tests
- Scalability tests
- Multi-cluster (optional)
- Documentation improvements

---
