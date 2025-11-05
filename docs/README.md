# KubeNova — Unified Control Plane

**Goal:** One external API that bootstraps clusters, enforces multi-tenancy, and delivers apps.

## Principles
- Single source of truth in KubeNova; clusters are projections.
- Adapters: KubeNova ↔ platform components only through adapters.
- Idempotent operations; eventual consistency via reconcilers.
- Least privilege; tenants access via a scoped access proxy, not kube-apiserver.
- Full observability: logs, metrics, traces, events.

## Bootstrap Runbook
1) **Tenancy controller** (install per your platform’s guidance)
2) **Access proxy** (expose proxy service for tenant-scoped kubeconfigs)
3) **App delivery core** (deploy core controllers and CRDs)
4) **Register cluster in KubeNova**
```http
POST /api/v1/clusters
{ "name":"prod-a", "kubeconfig":"<BASE64>", "labels":{"region":"eu-west"} }
```

## Unified API (canonical excerpts)
### Tenancy
```
POST   /api/v1/tenants
GET    /api/v1/tenants/{name}
POST   /api/v1/tenants/{name}/projects
```
### Delivery
```
POST   /api/v1/apps
GET    /api/v1/apps/{name}/rollout
POST   /api/v1/apps/{name}:rollback
```
### Access
```
POST   /api/v1/tenants/{name}:issue-kubeconfig
POST   /api/v1/projects/{name}:issue-kubeconfig
```
### Policies
```
POST /api/v1/policysets
```

## Adapters
- **TenancyAdapter:** Tenant, quotas, namespace options, RBAC, NetworkPolicy.
- **AppsAdapter:** Application, Workflow, WorkflowRun, Definitions. Deploy per project namespace. Support rollback via revisions.

## Reconcile Loop
Intent → Plan → Apply → Observe → Converge. Emit events with correlation-id.
Status phases: `Pending|Applying|Deployed|Drifted|Error`.

## Security
- JWT (HS256/RS256); roles: tenant-admin, tenant-dev, read-only.
- Kubeconfigs via **KubeconfigGrant**: TTL, verbs, namespaces; endpoint = access proxy.
- Envelope encryption for secrets; periodic key rotation.

## Observability
- Logs: structured JSON with `request_id`, `tenant`, `cluster`, `adapter`.
- Metrics: `kubenova_reconcile_seconds`, `kubenova_events_total`, `kubenova_adapter_errors_total`.
- Traces: OpenTelemetry.

## Failure Modes
- CRD mismatch → capability flags; return 422 for incompatible intents.
- Namespace drift → admission + repair.
- Proxy outage → tenant access blocked; operators still reconcile with scoped SA.

## Architecture (ASCII)
```
Client → KubeNova API → Orchestrator
                     ├─ TenancyAdapter → Tenancy + Access Proxy → Namespaces/RBAC/Policies
                     └─ AppsAdapter    → App Delivery Core → Applications/Workflows → Workloads
```
