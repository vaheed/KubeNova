# KubeNova ↔ Capsule ↔ KubeVela  
### Bootstrap Guide & API Mapping Reference

---

## Overview

KubeNova acts as a **central control plane** for multi-tenant Kubernetes environments.  
It coordinates **Capsule** for multi-tenancy and **KubeVela** for application delivery. 

Quick Start
- Deploy Manager via Helm (see flags below), then register a cluster via POST /api/v1/clusters.

What happens
- Manager stores the Cluster and installs the in-cluster Agent Deployment (replicas=2) and HPA.
- Agent starts with leader election, installs/validates add-ons in order: Capsule → capsule-proxy → KubeVela.
- Manager exposes status at GET /api/v1/clusters/{id} with Conditions: AgentReady, AddonsReady.
- Both components expose /healthz and /readyz and publish Prometheus metrics and OpenTelemetry traces.

Configuration
- env.example documents DATABASE_URL, JWT, and deployment image versions.
- AGENT_IMAGE controls the image used for remote install.

Tests
- `make test-unit` – unit tests and integration stubs.

Docs
- VitePress site at `docs/site`. Build with `make docs-build` and serve with `make docs-serve`.

New features
- Tenant listing supports `labelSelector` and `owner` filters.
- App operations wired to KubeVela: traits, policies, image-update, delete action.

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
- Charts are published to GHCR under a separate namespace:
  - ghcr.io/vaheed/kubenova-charts/manager
  - ghcr.io/vaheed/kubenova-charts/agent
- Branch/tag mapping
  - develop: chart version is suffixed with -dev (e.g., 0.9.3-dev). A lightweight OCI tag alias dev also points to the same artifact.
  - main: chart version is the normal semver (e.g., 0.9.3). A lightweight OCI tag alias latest also points to the same artifact.
  - release tags (vX.Y.Z): an additional OCI tag alias vX.Y.Z is applied to the published artifact.
- Examples (OCI)
```
helm registry login ghcr.io -u <user> -p <token>
# Pull latest main (alias)
helm pull oci://ghcr.io/vaheed/kubenova-charts/manager --version latest
# Pull a specific version
helm pull oci://ghcr.io/vaheed/kubenova-charts/manager --version 0.9.3
# Pull develop stream
helm pull oci://ghcr.io/vaheed/kubenova-charts/manager --version dev     # requires Helm that supports tag aliases
helm pull oci://ghcr.io/vaheed/kubenova-charts/manager --version 0.9.3-dev
# Pull a release tag alias
helm pull oci://ghcr.io/vaheed/kubenova-charts/manager --version v0.9.3   # alias to the same 0.9.3 artifact
```
```

### Helm install flags (quick reference)

Manager chart common flags
```
helm upgrade --install manager kubenova/manager \
  -n kubenova-system --create-namespace \
  --set image.tag=latest \
  --set env.KUBENOVA_REQUIRE_AUTH=true \
  --set env.MANAGER_URL_PUBLIC=http://kubenova-manager.kubenova-system.svc.cluster.local:8080 \
  --set env.CAPSULE_PROXY_URL=http://capsule-proxy.capsule-system.svc.cluster.local:9001 \
  --set env.AGENT_IMAGE=ghcr.io/vaheed/kubenova/agent:latest
```

Agent chart common flags
```
helm upgrade --install agent kubenova/agent \
  -n kubenova-system \
  --set image.tag=latest \
  --set manager.url=http://kubenova-manager.kubenova-system.svc.cluster.local:8080 \
  --set redis.enabled=true \
  --set bootstrap.capsuleVersion=0.10.6 \
  --set bootstrap.capsuleProxyVersion=0.9.13
```

Note: The Manager chart supports JWT secret injection via `jwt.existingSecret` or inline `jwt.value` for development.

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
helm repo add projectcapsule https://projectcapsule.github.io/charts
helm repo update

kubectl create namespace capsule-system || true

helm upgrade --install capsule projectcapsule/capsule \
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
helm upgrade --install capsule-proxy projectcapsule/capsule-proxy \
  --namespace capsule-system \
  --set service.enabled=true \
  --set options.allowedUserGroups='{tenant-admins,tenant-maintainers}'

kubectl -n capsule-system rollout status deploy/capsule-proxy
kubectl -n capsule-system get svc capsule-proxy
```

---

## 3. KubeNova ↔ Capsule — Top-50 API Map

Cluster-scoped KubeNova routes map to Capsule Tenant operations and related k8s resources. The key CRD is `capsule.clastix.io/v1beta2, Tenant`.

| # | KubeNova Route | Capsule/Cluster Mapping | Verb | OpenAPI |
|---|----------------|-------------------------|------|---------|
| 1 | `POST /api/v1/clusters/{c}/tenants` | Tenant (create/update) | create | [spec](docs/openapi/openapi.yaml#L445) |
| 2 | `GET /api/v1/clusters/{c}/tenants/{t}` | Tenant (get) | get | [spec](docs/openapi/openapi.yaml#L486) |
| 3 | `GET /api/v1/clusters/{c}/tenants` | Tenant (list with labels/owners) | list | [spec](docs/openapi/openapi.yaml#L445) |
| 4 | `DELETE /api/v1/clusters/{c}/tenants/{t}` | Tenant (delete) | delete | [spec](docs/openapi/openapi.yaml#L486) |
| 5 | `PUT /api/v1/clusters/{c}/tenants/{t}/owners` | Tenant.spec.owners | update | [spec](docs/openapi/openapi.yaml#L519) |
| 6 | `PUT /api/v1/clusters/{c}/tenants/{t}/quotas` | Tenant.spec.resourceQuotas.hard | update | [spec](docs/openapi/openapi.yaml#L549) |
| 7 | `PUT /api/v1/clusters/{c}/tenants/{t}/limits` | Tenant.spec.limitRanges.limits | update | [spec](docs/openapi/openapi.yaml#L572) |
| 8 | `PUT /api/v1/clusters/{c}/tenants/{t}/network-policies` | Tenant.spec.networkPolicies | update | [spec](docs/openapi/openapi.yaml#L595) |
| 9 | `GET /api/v1/clusters/{c}/tenants/{t}/summary` | Aggregate Tenant + cluster objects | get | [spec](docs/openapi/openapi.yaml#L618) |
|10 | `GET /api/v1/clusters/{c}/capabilities` | Detect Capsule/proxy presence | get | [spec](docs/openapi/openapi.yaml#L430) |
|11 | `POST /api/v1/clusters/{c}/bootstrap/tenancy` | Install/verify Capsule | action | [spec](docs/openapi/openapi.yaml#L411) |
|12 | `POST /api/v1/clusters/{c}/bootstrap/proxy` | Install/verify capsule-proxy | action | [spec](docs/openapi/openapi.yaml#L411) |
|13 | `GET /api/v1/clusters/{c}/tenants/{t}/projects` | Namespaces permitted by Tenant | list | [spec](docs/openapi/openapi.yaml#L633) |
|14 | `POST /api/v1/clusters/{c}/tenants/{t}/projects` | Create Namespace for project | create | [spec](docs/openapi/openapi.yaml#L633) |
|15 | `PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}` | Update Namespace labels/annots | update | [spec](docs/openapi/openapi.yaml#L673) |
|16 | `DELETE /api/v1/clusters/{c}/tenants/{t}/projects/{p}` | Delete Namespace | delete | [spec](docs/openapi/openapi.yaml#L673) |
|17 | `PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/access` | Namespace RBAC (Role/RoleBinding) | update | [spec](docs/openapi/openapi.yaml#L701) |
|18 | `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/kubeconfig` | capsule-proxy issued kubeconfig | get | [spec](docs/openapi/openapi.yaml#L707) |
|19 | `POST /api/v1/tenants/{t}/kubeconfig` | capsule-proxy tenant-scoped kubeconfig | create | [spec](docs/openapi/openapi.yaml#L1267) |
|20 | `GET /api/v1/tenants/{t}/usage` | Metrics aggregator (tenant scope) | get | [spec](docs/openapi/openapi.yaml#L1225) |
|21 | `GET /api/v1/projects/{p}/usage` | Metrics aggregator (namespace scope) | get | [spec](docs/openapi/openapi.yaml#L1245) |
|22 | `GET /api/v1/clusters/{c}/tenants?labelSelector=...` | Tenant list with label filter | list | [spec](docs/openapi/openapi.yaml#L445) |
|23 | `GET /api/v1/clusters?labelSelector=...` | Cluster list w/ labels (store) | list | [spec](docs/openapi/openapi.yaml#L334) |
|24 | `GET /api/v1/clusters/{c}` | Cluster object (store) | get | [spec](docs/openapi/openapi.yaml#L381) |
|25 | `DELETE /api/v1/clusters/{c}` | Remove cluster registration | delete | [spec](docs/openapi/openapi.yaml#L381) |
|26 | `POST /api/v1/tokens` | JWT for API auth (manager) | create | [spec](docs/openapi/openapi.yaml#L297) |
|27 | `GET /api/v1/me` | Introspect roles/subject | get | [spec](docs/openapi/openapi.yaml#L320) |
|28 | `GET /api/v1/healthz` | Liveness | get | [spec](docs/openapi/openapi.yaml#L261) |
|29 | `GET /api/v1/readyz` | Readiness (store reachable) | get | [spec](docs/openapi/openapi.yaml#L266) |
|30 | `GET /api/v1/version` | Version info | get | [spec](docs/openapi/openapi.yaml#L271) |

Notes
- Quotas/limits/network-policies write into Tenant.spec as per Capsule reference.
- Projects are represented as Namespaces within a Tenant; access updates result in Role/RoleBinding adjustments.
- Kubeconfigs are expected to be served by capsule-proxy; in dev/test they are stubbed.

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

KubeNova app endpoints map to KubeVela CRDs under `core.oam.dev/v1beta1`.

| # | KubeNova Route | KubeVela Mapping | Verb | OpenAPI |
|---|----------------|------------------|------|---------|
| 1 | `POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps` | Application (create) | create | [spec](docs/openapi/openapi.yaml#L810) |
| 2 | `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps` | Application (list) | list | [spec](docs/openapi/openapi.yaml#L810) |
| 3 | `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}` | Application (get) | get | [spec](docs/openapi/openapi.yaml#L851) |
| 4 | `PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}` | Application (update) | update | [spec](docs/openapi/openapi.yaml#L851) |
| 5 | `POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:deploy` | Application annotate (redeploy) | action | [spec](docs/openapi/openapi.yaml#L906) |
| 6 | `POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:suspend` | Application.spec.suspend=true | action | [spec](docs/openapi/openapi.yaml#L920) |
| 7 | `POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:resume` | Application.spec.suspend=false | action | [spec](docs/openapi/openapi.yaml#L934) |
| 8 | `POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:rollback` | Annotate with target revision | action | [spec](docs/openapi/openapi.yaml#L948) |
| 9 | `POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:delete` | Delete Application | delete | [spec](docs/openapi/openapi.yaml#L965) |
|10 | `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/status` | Application.status | get | [spec](docs/openapi/openapi.yaml#L971) |
|11 | `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/revisions` | ApplicationRevision (list by label) | list | [spec](docs/openapi/openapi.yaml#L992) |
|12 | `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/diff/{revA}/{revB}` | Compare two ApplicationRevisions | get | [spec](docs/openapi/openapi.yaml#L1013) |
|13 | `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/logs/{component}` | Pod logs by app/component labels | get | [spec](docs/openapi/openapi.yaml#L1046) |
|14 | `PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/traits` | Application.spec.traits | update | [spec](docs/openapi/openapi.yaml#L1084) |
|15 | `PUT /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/policies` | Application.spec.policies | update | [spec](docs/openapi/openapi.yaml#L1099) |
|16 | `POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/image-update` | Update component image/tag | action | [spec](docs/openapi/openapi.yaml#L1114) |
|17 | `POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/run` | WorkflowRun (create) | create | [spec](docs/openapi/openapi.yaml#L1169) |
|18 | `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/workflow/runs` | WorkflowRun (list) | list | [spec](docs/openapi/openapi.yaml#L1190) |
|19 | `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/runs/{id}` | WorkflowRun (get by id) | get | [spec](docs/openapi/openapi.yaml#L1203) |
|20 | `GET /api/v1/catalog/components` | ComponentDefinition (curated list) | list | [spec](docs/openapi/openapi.yaml#L1131) |
|21 | `GET /api/v1/catalog/traits` | TraitDefinition (curated list) | list | [spec](docs/openapi/openapi.yaml#L1143) |
|22 | `GET /api/v1/catalog/workflows` | WorkflowStepDefinition (curated list) | list | [spec](docs/openapi/openapi.yaml#L1155) |

Notes
- The adapter uses dynamic client for `Application` and `ApplicationRevision` in `core.oam.dev/v1beta1`.
- Logs are aggregated from Pods labeled with `app.oam.dev/name` and optional `app.oam.dev/component`.
- Catalog endpoints are curated views of KubeVela definitions; they don’t expose raw objects.
- SetTraits/SetPolicies update `spec.traits`/`spec.policies`; ImageUpdate updates the matching component `properties.image` (creates the component if missing).

---

## 6. Coverage — Remaining KubeNova API Endpoints

These endpoints are served by the Manager control plane or helper surfaces and don’t directly map to Capsule/KubeVela objects:

- Clusters
  - `POST /api/v1/clusters` — register cluster (stores kubeconfig, labels)
  - `GET /api/v1/clusters` — list registered clusters (label selectors supported)
  - `GET /api/v1/clusters/{c}` — get cluster
  - `DELETE /api/v1/clusters/{c}` — delete registration
  - `POST /api/v1/clusters/{c}/bootstrap/{component}` — install/verify components (tenancy, proxy, app-delivery)
- PolicySets
  - `GET /api/v1/clusters/{c}/policysets/catalog` — curated catalog
  - `GET/POST/PUT/DELETE /api/v1/clusters/{c}/tenants/{t}/policysets[/{}]` — tenant-scoped policy sets (control-plane stored)
- Access & System
  - `POST /api/v1/tokens`, `GET /api/v1/me`
  - `GET /api/v1/version`, `GET /api/v1/features`, `GET /api/v1/healthz`, `GET /api/v1/readyz`

All remaining endpoints in docs/openapi/openapi.yaml are implemented and tested; see reports/api_coverage.md for status.

---

## 6. Architecture (Plain Text)

```
KubeNova API → CapsuleAdapter → Capsule(+proxy)
           └→ VelaAdapter → KubeVela Core → Workloads
```
