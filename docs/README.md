# KubeNova — Unified Control Plane for Capsule + KubeVela

**Goal:** One external API that bootstraps clusters, enforces multi-tenancy (Capsule), and delivers apps (KubeVela).

## Principles
- Single source of truth in KubeNova; clusters are projections.
- Adapters: KubeNova ↔ Capsule/Vela only through adapters.
- Idempotent operations; eventual consistency via reconcilers.
- Least privilege; tenants access via capsule-proxy, not kube-apiserver.
- Full observability: logs, metrics, traces, events.

## Bootstrap Runbook
1) **Capsule**
```bash
helm repo add clastix https://clastix.github.io/charts && helm repo update
kubectl create ns capsule-system || true
helm upgrade --install capsule clastix/capsule -n capsule-system --set manager.leaderElection=true
kubectl -n capsule-system rollout status deploy/capsule-controller-manager
```
2) **capsule-proxy**
```bash
helm upgrade --install capsule-proxy clastix/capsule-proxy \
  -n capsule-system --set service.enabled=true \
  --set options.allowedUserGroups='{tenant-admins,tenant-maintainers}'
kubectl -n capsule-system rollout status deploy/capsule-proxy
```
3) **KubeVela Core**
```bash
helm repo add kubevela https://kubevela.github.io/charts && helm repo update
kubectl create ns vela-system || true
helm upgrade --install vela-core kubevela/vela-core -n vela-system --set admissionWebhooks.enabled=true
kubectl -n vela-system rollout status deploy/vela-core
```
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
- **CapsuleAdapter:** Tenant, quotas, namespace options, RBAC, NetworkPolicy. Stamp `capsule.clastix.io/tenant=<tenant>` on namespaces.
- **VelaAdapter:** Application, Workflow, WorkflowRun, *Definitions. Deploy per project namespace. Support rollback via Application revisions.

## Reconcile Loop
Intent → Plan → Apply → Observe → Converge. Emit events with correlation-id.
Status phases: `Pending|Applying|Deployed|Drifted|Error`.

## Security
- JWT (HS256/RS256); roles: tenant-admin, tenant-dev, read-only.
- Kubeconfigs via **KubeconfigGrant**: TTL, verbs, namespaces; endpoint = capsule-proxy.
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
                     ├─ CapsuleAdapter → Capsule + capsule-proxy → Namespaces/RBAC/Policies
                     └─ VelaAdapter    → KubeVela Core → Applications/Workflows → Workloads
```
