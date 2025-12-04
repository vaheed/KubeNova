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
- Implement role model:
  - System Owner
  - Tenant Owner
  - App User
- Map role actions to Kubernetes + Capsule permissions.

### 2.3 Tenant Management
- CRUD for tenants
- Assign tenant-level quotas
- Store tenant metadata for Operator syncing

### 2.4 Project Management
- CRUD for projects
- Bind Capsule Projects to KubeVela Projects
- Provide project-level owner/app kubeconfigs

### 2.5 Kubeconfig Generator
- Generate Owner and App kubeconfigs
- Secure token creation
- Auto-expiry and rotation support
- Permission validation via Capsule Proxy

### 2.6 Capsule Proxy Integration
- Auto-create proxy rules per tenant
- Restrict App kubeconfig actions
- Map user → tenant → project → namespace

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