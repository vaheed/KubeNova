# RFC: Unified Multi-Datacenter Architecture for KubeNova
**Version:** 1.0  
**Status:** Draft  
**Author:** —  
**Reviewers:** —  
**Created:** —  
**Updated:** —  

## 1. Overview

This document defines the architecture, lifecycle model, and system boundaries for **KubeNova**, a federated multi-datacenter platform providing secure CaaS/PaaS functionality on top of Kubernetes. KubeNova delivers a global control plane (**Manager**) and fully isolated per-datacenter compute environments powered by:

- KubeNova Operator (BYOI)  
- Capsule multi-tenancy  
- Capsule Proxy (isolated LoadBalancer per tenant)  
- KubeVela application orchestration  

The system enforces **strict cluster and datacenter independence**. The Manager never reaches into clusters directly and relies exclusively on outbound gRPC streams from Operators.

This RFC formalizes the entire design, including data models, APIs, bootstrap flows, multi-tenant lifecycle, usage reporting, and read-only status propagation.

---

## 2. High-Level Requirements

### 2.1 Goals

KubeNova must support:

- Multiple independent datacenters  
- Multiple independent Kubernetes clusters (one per datacenter)  
- A single global Manager for business logic, orchestration, metadata, billing, and dashboards  
- Per-datacenter CaaS/PaaS functionality driven by Operators, Capsule, Proxy, and KubeVela  
- Secure, outbound-only connectivity from clusters to Manager  
- Strong tenant isolation across namespaces and workloads  

### 2.2 Non-Goals

KubeNova must not:

- Allow the Manager to access clusters directly  
- Share workloads, secrets, or networking across datacenters  
- Provide cross-cluster workload scheduling  
- Allow tenants to communicate across datacenters  

---

## 3. Global System Architecture

### 3.1 Components

#### Manager (Global Control Plane)

- REST API  
- gRPC Manager (connect-go)  
- Postgres (clusters, users, tenants, usage, app records)  
- Dashboard UI  
- YAML assembler + dispatcher  
- Usage aggregator  
- Read-only status mirror  

#### Datacenter Cluster

Each datacenter hosts a single shared Kubernetes cluster running:

- KubeNova Operator  
- Capsule multi-tenancy  
- Capsule Proxy  
- KubeVela core + addons  
- Namespaces per tenant  
- Resource quotas + usage collectors  

### 3.2 Global Architecture Diagram

```
(Global Manager)
 ├─ REST API
 ├─ gRPC Manager
 └─ Postgres

(DataCenter A)   (DataCenter B)
 └─ Cluster        └─ Cluster
     ├─ Operator       ├─ Operator
     ├─ Capsule        ├─ Capsule
     ├─ Capsule Proxy  ├─ Capsule Proxy
     └─ KubeVela       └─ KubeVela

Operators → outbound gRPC streams → Manager
```

---

## 4. System Boundary Model

### 4.1 Manager Responsibilities

- Cluster registry & bootstrap  
- Tenant metadata  
- User lifecycle  
- YAML generation & transmission  
- Billing aggregation  
- Read-only status APIs  

Manager **never**:

- Calls Kubernetes APIs directly  
- Manages workloads  
- Stores tenant secrets beyond kubeconfig templates  

### 4.2 Operator Responsibilities

- Full local bootstrap (Capsule, Proxy, KubeVela)  
- Tenant provisioning  
- Namespace creation + RBAC  
- ServiceAccount & kubeconfig generation  
- Quota enforcement  
- KubeVela resource application  
- Hourly usage reporting  
- Real-time status event streaming  

### 4.3 Isolation Model

- No shared networking across datacenters  
- No shared workloads  
- No cross-datacenter resources  
- Manager never requires inbound connectivity  

---

## 5. Lifecycle Architecture

### 5.1 Phase 1 — Cluster Registration & Bootstrap

#### 5.1.1 Data Model

```
Cluster {
    UUID id
    string name
    string datacenter
    string kubeconfigEncrypted
    string novaClusterID
    enum status [pending_bootstrap, bootstrapping, connected, error]
    time createdAt
    time updatedAt
}
```

#### 5.1.2 API: POST /clusters

- Stores encrypted kubeconfig  
- Generates novaClusterID  
- Marks cluster as `pending_bootstrap`  

#### 5.1.3 Bootstrap Process

Manager:

1. Creates namespace `kubenova-system`  
2. Applies RBAC  
3. Deploys Operator with env vars:  
   - `NOVA_MANAGER_URL`  
   - `NOVA_CLUSTER_ID`  
   - `TARGET_STORAGE_CLASS`  
   - `TARGET_LB_ANNOTATIONS`  
   - `NOVA_CAPS_PROX_URL`  

#### 5.1.4 Sequence Diagram

```
User → Manager: POST /clusters
Manager → DB: save cluster
Manager → Cluster: deploy Operator
Operator → Manager: gRPC Connect
Manager → DB: status=connected
Manager → Operator: send NovaClusterConfig
Operator → Cluster: install Capsule, Proxy, KubeVela
```

---

## 5.2 Phase 2 — Operator Bootstrap

### NovaClusterConfig CRD

```
installCapsule: bool
installCapsuleProxy: bool
installKubeVela: bool
kubeVelaProfile: string
phase: [Pending, Installing, Ready, Error]
conditions: [...]
```

### Bootstrap Steps

- Install Capsule  
- Install Capsule Proxy  
- Install KubeVela  
- Mark CRD Ready  

---

## 5.3 Phase 3 — Tenant/User Provisioning

Per-tenant resources:

- Capsule Tenant  
- Namespaces: `<tenant>-owner`, `<tenant>-apps`  
- ServiceAccounts: owner + readonly  
- RBAC permissions  
- Secrets for kubeconfig tokens  

Manager composes kubeconfig files.

---

## 5.4 Phase 4 — KubeVela Project Creation

```
apiVersion: core.oam.dev/v1beta1
kind: Project
metadata:
  name: project-<tenant>
spec:
  destinations:
  - name: local
    namespace: <tenant-apps>
    cluster: local
```

---

## 5.5 Phase 5 — Application Deployment

Flow:

1. User → Manager: deployment request  
2. Manager → Operator: APPLY_YAML  
3. Operator → Cluster: creates Vela Application  
4. KubeVela deploys workloads  
5. Operator streams status  

---

## 5.6 Phase 6 — Hourly Usage Reporting

Metrics per tenant:

- CPU/memory requests  
- PVC capacity  
- LB Services count  
- Namespace count  
- Pod count  
- KubeVela Application count  

Operator → Manager via gRPC JSON payload.

---

## 5.7 Phase 7 — Read-Only Application Status API

`GET /users/:id/apps/:appID/status`

- Returns last-synced KubeVela app status from Manager DB  
- Manager never contacts cluster directly  

---

## 6. Security Model

- Zero inbound connectivity to clusters  
- Operators authenticate via gRPC  
- Secrets remain in datacenters  
- Capsule ensures namespace-level isolation  
- Capsule Proxy isolates LoadBalancers  

---

## 7. Failure & Recovery

### Operator Failure
- Workloads continue  
- Manager marks cluster stale  
- Resync on reconnect  

### Manager Failure
- Operators buffer events  
- No new deployments  

### Network Partition
- Retry logic on operator side  

---

## 8. Future Extensions

- Advanced billing  
- Per-datacenter KubeVela profiles  
- Optional pipelines  
- Per-tenant network policies  

---

## 9. Conclusion

This defines the full KubeNova multi-datacenter architecture.  
The system enforces:

- Strong separation of concerns  
- Cluster sovereignty  
- Tenant isolation  
- Declarative global control  

This RFC forms the foundation for **KubeNova 3.0**.

