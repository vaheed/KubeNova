# KubeNova  
### Unified Multi-Datacenter CaaS/PaaS Platform  
### Manager Global — Clusters Sovereign — Tenants Isolated

---

## 🚀 Overview

KubeNova is a federated multi-datacenter platform providing secure CaaS/PaaS capabilities on top of Kubernetes.  
Each datacenter runs a **completely isolated Kubernetes cluster**, while a **single global Manager** coordinates metadata, tenant lifecycle, billing, and application orchestration — without ever directly accessing the clusters.

All cluster operations are handled by the **KubeNova Operator**, which runs inside each cluster and communicates outbound-only with the Manager via gRPC.

KubeNova integrates the following:

- **KubeNova Operator** — cluster bootstrap, tenant management, Vela integration  
- **Capsule** — multi-tenancy and namespace isolation  
- **Capsule Proxy** — per-tenant LoadBalancer isolation  
- **KubeVela** — application orchestration for users  

---

## ⚖️ Core Principles

- **Clusters are sovereign** — no cross-datacenter sharing.  
- **Zero inbound connectivity** — Operators initiate outbound gRPC to Manager.  
- **Manager never talks to Kubernetes APIs directly.**  
- **Tenants are strictly isolated** using Capsule and namespace scoping.  
- **KubeVela Applications are the only workload entrypoint.**  

---

## 🏛️ System Architecture

### Global System Diagram

```mermaid
flowchart LR

subgraph Manager["Global Manager (Control Plane)"]
    API[REST API]
    GRPC[gRPC Manager]
    DB[(Postgres)]
    UI[Dashboard]
end

subgraph DC1["Datacenter A"]
    subgraph CL1["Kubernetes Cluster"]
        OP1[KubeNova Operator]
        CAPS1[Capsule]
        PROXY1[Capsule Proxy]
        VELA1[KubeVela]
    end
end

subgraph DC2["Datacenter B"]
    subgraph CL2["Kubernetes Cluster"]
        OP2[KubeNova Operator]
        CAPS2[Capsule]
        PROXY2[Capsule Proxy]
        VELA2[KubeVela]
    end
end

OP1 -->|Outbound gRPC| GRPC
OP2 -->|Outbound gRPC| GRPC
API --> DB
GRPC --> DB
```

---

## 🔄 Full Lifecycle Architecture

### End-to-End Workflow

```mermaid
sequenceDiagram
    participant User
    participant Manager
    participant Operator
    participant Cluster
    participant Vela

    User->>Manager: Add Cluster (POST /clusters)
    Manager->>Cluster: Deploy Operator
    Operator-->>Manager: gRPC Connect

    Manager->>Operator: Send NovaClusterConfig
    Operator->>Cluster: Install Capsule/Proxy/KubeVela

    User->>Manager: Create Tenant & User
    Manager->>Operator: Apply NovaTenant CRD
    Operator->>Cluster: Create Namespaces, SA, RBAC

    User->>Manager: Deploy Application
    Manager->>Operator: APPLY_YAML (KubeVela Application)
    Operator->>Vela: Create Application CRD
    Vela->>Cluster: Deploy Workloads

    Operator-->>Manager: Hourly Usage Reports
    Operator-->>Manager: Application Status Updates
```

---

## 🧱 Multi-Tenancy Model

KubeNova uses **Capsule** for multi-tenancy and **Capsule Proxy** for LoadBalancer isolation.

Each tenant receives:

- A Capsule Tenant  
- Two namespaces:  
  - `<tenant>-owner`  
  - `<tenant>-apps`  
- Two ServiceAccounts: owner + readonly  
- Two automatically generated kubeconfigs  
- One KubeVela Project  
- Unlimited KubeVela Applications  

### Tenant Bootstrap Diagram

```mermaid
flowchart TD
    NT[NovaTenant CRD]
    CAPS[Capsule Tenant]
    NS1["Namespace: <tenant>-owner"]
    NS2["Namespace: <tenant>-apps"]
    SA1[Owner ServiceAccount]
    SA2[Readonly ServiceAccount]
    KCFG1[Owner Kubeconfig]
    KCFG2[Readonly Kubeconfig]

    NT --> CAPS
    CAPS --> NS1
    CAPS --> NS2
    NS1 --> SA1 --> KCFG1
    NS2 --> SA2 --> KCFG2
```

---

## 🚀 Application Deployment (via KubeVela)

All user applications are defined as **KubeVela Application CRDs**.

### Deployment Flow

```mermaid
sequenceDiagram
    participant Dev
    participant Manager
    participant Operator
    participant Vela

    Dev->>Manager: POST /users/:tenant/apps
    Manager->>Operator: APPLY_YAML
    Operator->>Vela: Create Application CRD
    Vela->>Cluster: Deploy workloads
```

---

## 📊 Usage Reporting

Every hour the Operator aggregates per-tenant metrics:

- CPU & Memory Requests  
- PVC Storage Usage  
- LoadBalancer Count  
- Pod Count  
- Namespace Count  
- KubeVela Application Count  
- Quota Violations  

Usage is streamed to the Manager via gRPC.

---

## 🔐 Security Model

- **No inbound ports exposed**  
- **Outbound mTLS gRPC only**  
- **Encrypted kubeconfigs stored only for bootstrap**  
- **Capsule enforces strict boundaries**  
- **Capsule Proxy provides tenant LB isolation**  
- **Manager has no kubeadmin rights**  

---

## 🗂 Suggested Repository Layout

```
kubenova/
├── cmd/
│   ├── manager/
│   └── operator/
├── pkg/
│   ├── api/
│   ├── controllers/
│   ├── grpc/
│   ├── tenants/
│   └── kube/
├── config/
│   └── crd/
├── docs/
│   ├── rfc/
│   ├── adr/
│   ├── diagrams/
│   └── examples/
└── README.md
```

---

## 📚 Included Documentation

This repository includes:

- **Architecture RFC**  
- **ADR Set**  
- **Operator Controller Design**  
- **Manager API & Workflow**  
- **Multi-Tenant Policy & Structure**  
- **Diagrams in Mermaid format**  

---

## 🧩 Next Steps (Optional)

I can generate:

- `/docs/` folder with RFC, ADR, diagrams  
- GitHub Pages site  
- VitePress documentation  
- CRD YAML files  
- gRPC protobuf definitions  
- OpenAPI spec for Manager REST APIs  

Just let me know.

