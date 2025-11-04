# KubeNova ↔ Capsule ↔ KubeVela  
### Bootstrap Guide & API Mapping Reference

---

## Overview

KubeNova acts as a **central control plane** for multi-tenant Kubernetes environments.  
It coordinates **Capsule** for multi-tenancy and **KubeVela** for application delivery. 

Quick Start (Kind)
- make kind-up
- make deploy-manager
 - Port-forward Manager and register the cluster:
  - kubectl -n kubenova-system port-forward svc/kubenova-manager 8080:8080 &
  - curl -XPOST localhost:8080/api/v1/clusters -H 'Content-Type: application/json' \
    -d '{"name":"kind","kubeconfig":"'"$(base64 -w0 ~/.kube/config 2>/dev/null || base64 ~/.kube/config)"'"}'

What happens
- Manager stores the Cluster and installs the in-cluster Agent Deployment (replicas=2) and HPA.
- Agent starts with leader election, installs/validates add-ons in order: Capsule → capsule-proxy → KubeVela.
- Manager exposes status at GET /api/v1/clusters/{id} with Conditions: AgentReady, AddonsReady.
- Both components expose /healthz and /readyz and publish Prometheus metrics and OpenTelemetry traces.

Configuration
- env.example documents DATABASE_URL, JWT, and deployment image versions.
- AGENT_IMAGE controls the image used for remote install.

Tests
- `make test-unit` – unit tests and integration stubs (the Go E2E suite is disabled by default).
- `E2E_RUN=1 E2E_BUILD_IMAGES=true make test-e2e` – Kind-based end-to-end suite that builds local Manager/Agent images, registers a cluster, and verifies Capsule/capsule-proxy/KubeVela health. When `E2E_RUN=1` is omitted, the suite is skipped.
  - Use `E2E_WAIT_TIMEOUT` to extend the suite's wait budget and HTTP timeouts when agent installation or add-on bootstrapping needs longer than the default 20 minutes.

Docs
- VitePress site at `docs/site`. Build with `make docs-build` and serve with `make docs-serve`.

Helm charts
- CI publishes packaged charts to GitHub Pages:
  - develop → https://vaheed.github.io/kubenova/charts/dev
  - main → https://vaheed.github.io/kubenova/charts/stable
- Add repo and install:
```
helm repo add kubenova-dev https://vaheed.github.io/kubenova/charts/dev
helm repo add kubenova https://vaheed.github.io/kubenova/charts/stable
helm repo update
helm install manager kubenova/manager --namespace kubenova-system --create-namespace

OCI charts in GitHub Packages (GHCR)
- CI pushes Helm charts as OCI artifacts to a separate repo namespace to avoid container tag collisions:
  - ghcr.io/vaheed/kubenova-charts/manager
  - ghcr.io/vaheed/kubenova-charts/kubenova-agent
- Tags:
  - develop: semantic version with -dev suffix, plus alias dev
  - main: semantic version, plus alias latest
- Example (OCI):
```
helm registry login ghcr.io -u <user> -p <token>
helm pull oci://ghcr.io/vaheed/kubenova-charts/manager --version latest
```
```

This document includes:
- Capsule & capsule-proxy bootstrap instructions  
- Top-50 Capsule API → KubeNova route mapping  
- KubeVela Core bootstrap instructions  
- Top-50 KubeVela API → KubeNova route mapping  
- Plain-text architecture diagram for clarity  

---

## Official References

| Component | Documentation |
|------------|----------------|
| Capsule | https://projectcapsule.dev/docs/reference/#capsuleclastixiov1beta2 |
| Capsule Proxy | https://projectcapsule.dev/docs/proxy/reference/#capsuleclastixiov1beta1 |
| KubeVela API | https://github.com/kubevela/velaux/blob/main/docs/apidoc/swagger.json |

---

## 1. Bootstrap Capsule

**Capsule** provides tenant isolation using namespaces, network policies, and admission controllers.  

```bash
helm repo add clastix https://clastix.github.io/charts
helm repo update

kubectl create namespace capsule-system || true

helm upgrade --install capsule clastix/capsule \
  --namespace capsule-system \
  --set manager.leaderElection=true

kubectl -n capsule-system rollout status deploy/capsule-controller-manager
kubectl api-resources | grep capsule.clastix.io
```
CRDs installed:
- `tenants.capsule.clastix.io`
- `tenantresourcequotas.capsule.clastix.io`
- `namespaceoptions.capsule.clastix.io`
- `capsuleconfigurations.capsule.clastix.io`

---

## 2. Bootstrap Capsule Proxy

**capsule-proxy** enforces tenant boundaries at API request level.

```bash
helm upgrade --install capsule-proxy clastix/capsule-proxy \
  --namespace capsule-system \
  --set service.enabled=true \
  --set options.allowedUserGroups='{tenant-admins,tenant-maintainers}'

kubectl -n capsule-system rollout status deploy/capsule-proxy
kubectl -n capsule-system get svc capsule-proxy
```

---

## 3. KubeNova ↔ Capsule — Top-50 API Map

Each object supports CRUD verbs (`create|get|list|update|delete`).

| # | KubeNova Route | Capsule Resource | Verbs |
|---|----------------|------------------|-------|
| 1 | `POST /api/v1/tenants` | `capsule.clastix.io/v1beta2, Tenant` | create |
| 2 | `GET /api/v1/tenants/{name}` | Tenant | get |
| 3 | `GET /api/v1/tenants` | Tenant | list |
| 4 | `PUT /api/v1/tenants/{name}` | Tenant | update |
| 5 | `DELETE /api/v1/tenants/{name}` | Tenant | delete |
| 6 | `POST /api/v1/tenant-quotas` | TenantResourceQuota | create |
| 7 | `GET /api/v1/tenant-quotas/{name}` | TenantResourceQuota | get |
| 8 | `GET /api/v1/tenant-quotas` | TenantResourceQuota | list |
| 9 | `PUT /api/v1/tenant-quotas/{name}` | TenantResourceQuota | update |
|10 | `DELETE /api/v1/tenant-quotas/{name}` | TenantResourceQuota | delete |
|11 | `POST /api/v1/namespace-options` | NamespaceOptions | create |
|12 | `GET /api/v1/namespace-options/{name}` | NamespaceOptions | get |
|13 | `GET /api/v1/namespace-options` | NamespaceOptions | list |
|14 | `PUT /api/v1/namespace-options/{name}` | NamespaceOptions | update |
|15 | `DELETE /api/v1/namespace-options/{name}` | NamespaceOptions | delete |
|16 | `POST /api/v1/configurations` | CapsuleConfiguration | create |
|17 | `GET /api/v1/configurations/{name}` | CapsuleConfiguration | get |
|18 | `GET /api/v1/configurations` | CapsuleConfiguration | list |
|19 | `PUT /api/v1/configurations/{name}` | CapsuleConfiguration | update |
|20 | `DELETE /api/v1/configurations/{name}` | CapsuleConfiguration | delete |
|21 | `POST /api/v1/networkpolicies` | NetworkPolicy (tenant) | create |
|22 | `GET /api/v1/networkpolicies/{name}` | NetworkPolicy | get |
|23 | `GET /api/v1/networkpolicies` | NetworkPolicy | list |
|24 | `PUT /api/v1/networkpolicies/{name}` | NetworkPolicy | update |
|25 | `DELETE /api/v1/networkpolicies/{name}` | NetworkPolicy | delete |
|26 | `POST /api/v1/namespaces` | Namespace (tenant) | create |
|27 | `GET /api/v1/namespaces/{name}` | Namespace | get |
|28 | `GET /api/v1/namespaces` | Namespace | list |
|29 | `PUT /api/v1/namespaces/{name}` | Namespace | update |
|30 | `DELETE /api/v1/namespaces/{name}` | Namespace | delete |
|31 | `POST /api/v1/roles` | Role | create |
|32 | `GET /api/v1/roles/{name}` | Role | get |
|33 | `GET /api/v1/roles` | Role | list |
|34 | `PUT /api/v1/roles/{name}` | Role | update |
|35 | `DELETE /api/v1/roles/{name}` | Role | delete |
|36 | `POST /api/v1/rolebindings` | RoleBinding | create |
|37 | `GET /api/v1/rolebindings/{name}` | RoleBinding | get |
|38 | `GET /api/v1/rolebindings` | RoleBinding | list |
|39 | `PUT /api/v1/rolebindings/{name}` | RoleBinding | update |
|40 | `DELETE /api/v1/rolebindings/{name}` | RoleBinding | delete |
|41 | `POST /api/v1/quotas` | ResourceQuota | create |
|42 | `GET /api/v1/quotas/{name}` | ResourceQuota | get |
|43 | `GET /api/v1/quotas` | ResourceQuota | list |
|44 | `PUT /api/v1/quotas/{name}` | ResourceQuota | update |
|45 | `DELETE /api/v1/quotas/{name}` | ResourceQuota | delete |
|46 | `GET /api/v1/tenant-status/{name}` | Tenant.status | get |
|47 | `GET /api/v1/tenant-events/{name}` | Events | list |
|48 | `GET /api/v1/tenant-policies/{name}` | PolicyRef | list |
|49 | `POST /api/v1/tenant-sync` | Capsule Sync | create |
|50 | `DELETE /api/v1/tenant-sync/{name}` | Capsule Sync | delete |

---

## 4. Bootstrap KubeVela Core

```bash
helm repo add kubevela https://kubevela.github.io/charts
helm repo update
kubectl create ns vela-system || true

helm upgrade --install vela-core kubevela/vela-core \
  --namespace vela-system \
  --set admissionWebhooks.enabled=true

kubectl -n vela-system rollout status deploy/vela-core
kubectl api-resources | grep vela
```

---

## 5. KubeNova ↔ KubeVela — Top-50 API Map

| # | KubeNova Route | KubeVela Object | Verbs |
|---|----------------|----------------|-------|
| 1 | `POST /api/v1/apps` | Application | create |
| 2 | `GET /api/v1/apps/{name}` | Application | get |
| 3 | `GET /api/v1/apps` | Application | list |
| 4 | `PUT /api/v1/apps/{name}` | Application | update |
| 5 | `DELETE /api/v1/apps/{name}` | Application | delete |
| 6 | `POST /api/v1/app-revisions` | ApplicationRevision | create |
| 7 | `GET /api/v1/app-revisions/{name}` | ApplicationRevision | get |
| 8 | `GET /api/v1/app-revisions` | ApplicationRevision | list |
| 9 | `PUT /api/v1/app-revisions/{name}` | ApplicationRevision | update |
|10 | `DELETE /api/v1/app-revisions/{name}` | ApplicationRevision | delete |
|11 | `POST /api/v1/workflows` | Workflow | create |
|12 | `GET /api/v1/workflows/{name}` | Workflow | get |
|13 | `GET /api/v1/workflows` | Workflow | list |
|14 | `PUT /api/v1/workflows/{name}` | Workflow | update |
|15 | `DELETE /api/v1/workflows/{name}` | Workflow | delete |
|16 | `POST /api/v1/workflowruns` | WorkflowRun | create |
|17 | `GET /api/v1/workflowruns/{name}` | WorkflowRun | get |
|18 | `GET /api/v1/workflowruns` | WorkflowRun | list |
|19 | `PUT /api/v1/workflowruns/{name}` | WorkflowRun | update |
|20 | `DELETE /api/v1/workflowruns/{name}` | WorkflowRun | delete |
|21 | `POST /api/v1/components` | ComponentDefinition | create |
|22 | `GET /api/v1/components/{name}` | ComponentDefinition | get |
|23 | `GET /api/v1/components` | ComponentDefinition | list |
|24 | `PUT /api/v1/components/{name}` | ComponentDefinition | update |
|25 | `DELETE /api/v1/components/{name}` | ComponentDefinition | delete |
|26 | `POST /api/v1/traits` | TraitDefinition | create |
|27 | `GET /api/v1/traits/{name}` | TraitDefinition | get |
|28 | `GET /api/v1/traits` | TraitDefinition | list |
|29 | `PUT /api/v1/traits/{name}` | TraitDefinition | update |
|30 | `DELETE /api/v1/traits/{name}` | TraitDefinition | delete |
|31 | `POST /api/v1/policies` | PolicyDefinition | create |
|32 | `GET /api/v1/policies/{name}` | PolicyDefinition | get |
|33 | `GET /api/v1/policies` | PolicyDefinition | list |
|34 | `PUT /api/v1/policies/{name}` | PolicyDefinition | update |
|35 | `DELETE /api/v1/policies/{name}` | PolicyDefinition | delete |
|36 | `POST /api/v1/projects` | VelaUX /projects | create |
|37 | `GET /api/v1/projects/{name}` | VelaUX /projects/{name} | get |
|38 | `GET /api/v1/projects` | VelaUX /projects | list |
|39 | `PUT /api/v1/projects/{name}` | VelaUX /projects/{name} | update |
|40 | `DELETE /api/v1/projects/{name}` | VelaUX /projects/{name} | delete |
|41 | `POST /api/v1/envs` | VelaUX /envs | create |
|42 | `GET /api/v1/envs/{name}` | VelaUX /envs/{name} | get |
|43 | `GET /api/v1/envs` | VelaUX /envs | list |
|44 | `PUT /api/v1/envs/{name}` | VelaUX /envs/{name} | update |
|45 | `DELETE /api/v1/envs/{name}` | VelaUX /envs/{name} | delete |
|46 | `POST /api/v1/targets` | VelaUX /targets | create |
|47 | `GET /api/v1/targets/{name}` | VelaUX /targets/{name} | get |
|48 | `GET /api/v1/targets` | VelaUX /targets | list |
|49 | `PUT /api/v1/targets/{name}` | VelaUX /targets/{name} | update |
|50 | `DELETE /api/v1/targets/{name}` | VelaUX /targets/{name} | delete |

---

## 6. Architecture (Plain Text)

```
KubeNova API → CapsuleAdapter → Capsule(+proxy)
           └→ VelaAdapter → KubeVela Core → Workloads
```
