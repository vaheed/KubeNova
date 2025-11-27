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
- **AppsAdapter:** Application, Workflow, WorkflowRun, Definitions. Deploy per project namespace. Support rollback via revisions. In-cluster, the Operator runs an `AppReconciler` (`internal/reconcile/app.go`) that watches ConfigMaps labeled with the KubeNova app identity and uses `internal/backends/vela` to project them into KubeVela `Application` resources and keep traits/policies in sync.

## Plans & PolicySets catalog

- The tenant plan and PolicySet catalog is embedded from `pkg/catalog/plans.json` and `pkg/catalog/policysets.json`.
- Plans define tenant‑level quotas and a list of PolicySets to attach; PolicySets describe higher‑level traits/policies that are turned into Vela traits/policies at deploy time.
- The Manager exposes:
  - `GET /api/v1/plans` and `GET /api/v1/plans/{name}` to inspect available plans.
  - `PUT /api/v1/tenants/{t}/plan` to apply a plan to an existing tenant.
- On tenant creation:
  - If the request includes a `plan` field (for example `baseline` or `gold`), that plan is applied immediately.
  - If `plan` is omitted and `KUBENOVA_DEFAULT_TENANT_PLAN` is set (default `baseline`) and exists in the catalog, the Manager best‑effort applies that plan as the default. If the configured default plan is missing or fails to apply, defaulting is effectively disabled and only the stored tenant record is created.
- To customize plans/policysets, edit the JSON files in `pkg/catalog` and rebuild the Manager image; changes are read at process start and reflected in the `/plans` and `/policysets` APIs.

## Reconcile Loop
Intent → Plan → Apply → Observe → Converge. Emit events with correlation-id.
Status phases: `Pending|Applying|Deployed|Drifted|Error`.

## Security
- JWT (HS256); KubeNova roles: `tenantOwner`, `projectDev`, `readOnly`.
  - Roles are used for Manager API RBAC and are also mapped to Kubernetes groups for capsule-proxy:
    - `tenantOwner` (and `admin`/`ops`) → group `tenant-admins`.
    - `projectDev` → group `tenant-maintainers`.
    - `readOnly` → group `tenant-viewers` (operators may bind this group for read-only access).
  - Issued tokens include both `roles` and `groups` claims so capsule-proxy and Kubernetes RBAC can enforce the same semantics.
- Kubeconfigs via **KubeconfigGrant**: TTL, verbs, namespaces; endpoint = access proxy. Tenant and project kubeconfig endpoints embed these JWTs.
- Envelope encryption utilities are available in `internal/security` for future secret-at-rest encryption and key rotation; the current release does not yet encrypt stored data by default.

New features
- Tenant listing supports `labelSelector` and `owner` filters.
- App operations wired to KubeVela: `traits`, `policies`, `image-update`, `:delete` action.
- See interactive examples in `docs/index.md` (Section 5 and 6).

## Observability
- Logs: structured JSON with `request_id`, `tenant`, `cluster`, `adapter`, and `trace_id` when OTLP is enabled.
- Metrics: `kubenova_reconcile_seconds`, `kubenova_events_total`, `kubenova_adapter_errors_total`.
- Traces: OpenTelemetry/OTLP; set `OTEL_EXPORTER_OTLP_ENDPOINT` (+ `OTEL_EXPORTER_OTLP_INSECURE`) to stream spans to SigNoz or any collector. See `docs/examples/signoz.md` for SigNoz setup.

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
