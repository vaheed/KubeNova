# KubeNova Architecture ADR Set

## ADR-001: Global Manager with Per-Datacenter Isolated Clusters
**Status:** Accepted  
**Date:** 2025-11-23  

### Context
KubeNova must support multiple datacenters, each with independent Kubernetes clusters, while providing a unified control plane for cluster registration, tenant/user lifecycle, application deployment, usage reporting, and billing.  
Hard isolation must be enforced across datacenters.

### Decision
Implement a **single global Manager** and **one fully isolated Kubernetes cluster per datacenter**.  
Manager stores metadata only; no workloads or shared networking.

### Consequences
**Pros:** strong isolation, cleaner compliance boundaries, scalable SaaS design.  
**Cons:** no cross-cluster workloads, more clusters to operate.

---

## ADR-002: Outbound-Only gRPC Operator Model
**Status:** Accepted  
**Date:** 2025-11-23  

### Context
Clusters may be behind firewalls/NAT, and Manager must not require inbound access.

### Decision
Every cluster runs a **KubeNova Operator** which establishes an **outbound** authenticated gRPC stream to the Manager. All instructions and status updates flow through this stream.

### Consequences
**Pros:** secure, NAT-friendly, minimal attack surface.  
**Cons:** requires resilient streaming and reconnection logic.

---

## ADR-003: Manager Must Not Talk Directly to Kubernetes APIs
**Status:** Accepted  
**Date:** 2025-11-23  

### Context
Direct API access requires privileged connectivity, complicates security, and reduces isolation.

### Decision
Manager is forbidden from calling cluster APIs.  
All cluster interactions must pass through the Operator.

### Consequences
**Pros:** clean separation of concerns; no high-privilege central component.  
**Cons:** debugging flows must involve the Operator.

---

## ADR-004: Use Capsule & Capsule Proxy for Multi-Tenancy
**Status:** Accepted  
**Date:** 2025-11-23  

### Context
We need strong logical isolation between tenants inside shared clusters.

### Decision
Use **Capsule** for tenant boundaries and **Capsule Proxy** for per-tenant LoadBalancer isolation.

### Consequences
**Pros:** hardened multi-tenancy, namespace ownership, strong policy model.  
**Cons:** ties architecture to Capsule CRDs and behavior.

---

## ADR-005: Use KubeVela as the Application Orchestrator
**Status:** Accepted  
**Date:** 2025-11-23  

### Context
We need a flexible application orchestration layer without building our own platform abstraction.

### Decision
Use **KubeVela** as the canonical workload engine.  
All user applications become **KubeVela Application CRDs**.

### Consequences
**Pros:** clean abstraction, OAM model, addons ecosystem.  
**Cons:** dependency on KubeVela lifecycle and resources.

---

## ADR-006: Per-Datacenter Single Shared Cluster
**Status:** Accepted  
**Date:** 2025-11-23  

### Context
Options included: per-tenant clusters, per-datacenter shared clusters, or hybrid.

### Decision
Each datacenter runs **one shared cluster** containing multiple tenants.  
Isolation is enforced via Capsule, namespace policies, and quotas.

### Consequences
**Pros:** simpler operations, efficient resource sharing.  
**Cons:** isolation is logical, not physical; noisy neighbor risks.

---

## ADR-007: Pull-From-DB Read-Only Application Status
**Status:** Accepted  
**Date:** 2025-11-23  

### Context
Manager must expose app status without direct Kubernetes access.

### Decision
Operator syncs KubeVela Application statuses to Manager.  
Manager serves read-only status from Postgres ("pull-from-db" model).

### Consequences
**Pros:** cluster independence, no direct API calls.  
**Cons:** eventual consistency.

---

## ADR-008: Hourly Usage Aggregation in Operator
**Status:** Accepted  
**Date:** 2025-11-23  

### Context
Billing requires tenant-level aggregated usage.

### Decision
Each Operator performs hourly usage aggregation (CPU/memory/PVC/LB/pods/apps) and sends results to Manager.

### Consequences
**Pros:** reduces cross-datacenter traffic; localizes heavy introspection.  
**Cons:** billing precision limited to hourly granularity.

---

## ADR-009: Two Namespaces + Two Kubeconfigs per Tenant
**Status:** Accepted  
**Date:** 2025-11-23  

### Context
Tenants need clean operational separation and different permission levels.

### Decision
Each tenant gets:  
- `<tenant>-owner` namespace  
- `<tenant>-apps` namespace  
- Owner SA (full access)  
- Readonly SA (get/list/watch-only)  
- Two kubeconfigs generated from those SAs

### Implementation Notes
- The operator creates both namespaces during `NovaTenant` reconciliation, plus Capsule Tenant CR.
- Owner/readonly ServiceAccounts, Roles, and RoleBindings are created in both namespaces; kubeconfigs are packaged into `kubenova-kubeconfigs` Secret under `<tenant>-owner`.
- Manager’s `/api/v1/tenants/{tenantId}/kubeconfig` returns the kubeconfig content from that Secret when present (keys: `owner`, `readonly`); if missing, it falls back to the Capsule Proxy base URL (`{proxyBase}`).

### Consequences
**Pros:** predictable structure; clean roles; reproducible model.  
**Cons:** rigid namespace structure for all tenants.

---

# End of ADR Set
