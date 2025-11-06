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
The API surface is cluster-scoped and defined in `docs/openapi/openapi.yaml`.

### System & Access
```
GET    /api/v1/healthz
GET    /api/v1/readyz
GET    /api/v1/version
GET    /api/v1/features
POST   /api/v1/tokens
GET    /api/v1/me
```

### Clusters
```
POST   /api/v1/clusters
GET    /api/v1/clusters
GET    /api/v1/clusters/{c}
DELETE /api/v1/clusters/{c}
GET    /api/v1/clusters/{c}/capabilities
POST   /api/v1/clusters/{c}/bootstrap/{component}
```

### Tenants
```
GET    /api/v1/clusters/{c}/tenants
POST   /api/v1/clusters/{c}/tenants
GET    /api/v1/clusters/{c}/tenants/{t}
DELETE /api/v1/clusters/{c}/tenants/{t}
PUT    /api/v1/clusters/{c}/tenants/{t}/owners
PUT    /api/v1/clusters/{c}/tenants/{t}/quotas
PUT    /api/v1/clusters/{c}/tenants/{t}/limits
PUT    /api/v1/clusters/{c}/tenants/{t}/network-policies
GET    /api/v1/clusters/{c}/tenants/{t}/summary
POST   /api/v1/tenants/{t}/kubeconfig
GET    /api/v1/tenants/{t}/usage
```

### Projects
```
GET    /api/v1/clusters/{c}/tenants/{t}/projects
POST   /api/v1/clusters/{c}/tenants/{t}/projects
GET    /api/v1/clusters/{c}/tenants/{t}/projects/{p}
PUT    /api/v1/clusters/{c}/tenants/{t}/projects/{p}
DELETE /api/v1/clusters/{c}/tenants/{t}/projects/{p}
PUT    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/access
GET    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/kubeconfig
GET    /api/v1/projects/{p}/usage
```

### Apps
```
GET    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps
POST   /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps
GET    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}
PUT    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}
POST   /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:deploy
POST   /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:suspend
POST   /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:resume
POST   /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:rollback
POST   /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:delete
GET    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/status
GET    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/revisions
GET    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/diff/{revA}/{revB}
GET    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/logs/{component}
PUT    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/traits
PUT    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/policies
POST   /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/run
GET    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/runs
GET    /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/runs/{id}
```

### PolicySets & Catalog
```
GET    /api/v1/clusters/{c}/tenants/{t}/policysets
POST   /api/v1/clusters/{c}/tenants/{t}/policysets
GET    /api/v1/clusters/{c}/tenants/{t}/policysets/{name}
PUT    /api/v1/clusters/{c}/tenants/{t}/policysets/{name}
DELETE /api/v1/clusters/{c}/tenants/{t}/policysets/{name}
GET    /api/v1/clusters/{c}/policysets/catalog
GET    /api/v1/catalog/components
GET    /api/v1/catalog/traits
GET    /api/v1/catalog/workflows
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

New features
- Tenant listing supports `labelSelector` and `owner` filters.
- App operations wired to KubeVela: `traits`, `policies`, `image-update`, `:delete` action.
- See interactive examples in `docs/index.md` (Section 5 and 6).

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
