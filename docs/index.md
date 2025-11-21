---
title: KubeNova API v1 – cURL Quickstart
---

# KubeNova API v1 – cURL Quickstart

This page walks through a complete end‑to‑end flow using `curl`, from getting an access token to deploying a simple app. Every step shows:

- the exact command to run, and
- a short explanation of what it does.

All examples use the v1 HTTP API defined in `docs/openapi/openapi.yaml` and implemented by the manager in `internal/http`. Only implemented endpoints are shown here; for the full contract (including future additions) see `docs/openapi/openapi.yaml` and `docs/README.md`.

> You can copy‑paste these snippets into any POSIX shell (bash/zsh). On Windows, run them from WSL or adapt the syntax to PowerShell.

---

## 0) Prerequisites & base URL

**Command**

```bash
# Where the KubeNova API is listening
export BASE=${BASE:-http://localhost:8080}
```

**What this does**

- Defines `BASE` as the root URL for all API calls.
- Change the value (for example to `https://kubenova.example.com`) if your manager is not running on `localhost:8080`.

You will reuse `$BASE` in every subsequent `curl` command.

---

## 1) Get a JWT for API calls

When `KUBENOVA_REQUIRE_AUTH=true`, most endpoints require a Bearer JWT signed with `JWT_SIGNING_KEY`. You can issue a short‑lived token through the `/api/v1/tokens` endpoint.

**Command**

```bash
# Issue an admin token valid for 1 hour and store it in KN_TOKEN
export KN_TOKEN=$(
  curl -sS -X POST "$BASE/api/v1/tokens" \
    -H 'Content-Type: application/json' \
    -d '{"subject":"demo-admin","roles":["admin"],"ttlSeconds":3600}' \
  | jq -r '.token'
)

echo "$KN_TOKEN"
```

**What this does**

- Sends a `POST /api/v1/tokens` request with:
  - `subject`: an identifier for the caller (`demo-admin` here).
  - `roles`: a list of roles for the token. Valid roles are `admin`, `ops`, `tenantOwner`, `projectDev`, and `readOnly`.
  - `ttlSeconds`: token time‑to‑live in seconds (between 60 and 2 592 000).
- Extracts the `token` field from the JSON response and stores it in `KN_TOKEN`.
- Prints the raw JWT so you can verify it or copy it elsewhere if needed.

If `KUBENOVA_REQUIRE_AUTH=false`, you can skip this step and omit the Authorization header in later calls.

For convenience, define a small helper for the header:

```bash
# Convenience variable used in later commands
export AUTH_HEADER="Authorization: Bearer $KN_TOKEN"
```

You will now add `-H "$AUTH_HEADER"` to authenticated `curl` calls.

---

## 2) Verify connectivity and identity

Before touching any clusters or tenants, confirm that the API is reachable and that your token is recognized.

**Commands**

```bash
# Who am I? (requires auth when enabled)
curl -sS "$BASE/api/v1/me" \
  -H "$AUTH_HEADER" \
  | jq .

# Basic system information (no auth required)
curl -sS "$BASE/api/v1/healthz"
curl -sS "$BASE/api/v1/readyz"

curl -sS "$BASE/api/v1/version" | jq .
curl -sS "$BASE/api/v1/features" | jq .
```

**What this does**

- `GET /api/v1/me` returns the `subject` and `roles` derived from your JWT or from the `X-KN-Roles` header in tests/dev.
- `GET /api/v1/healthz` and `GET /api/v1/readyz` report basic liveness and readiness.
- `GET /api/v1/version` returns version, commit, and build date for the manager.
- `GET /api/v1/features` tells you which high‑level capabilities are enabled (tenancy, proxy, app delivery) and, when plans are configured, surfaces the default tenant plan and the list of available plans.

If `healthz` or `readyz` returns a non‑200 status, check the manager logs before continuing.

---



## 3) Register a cluster

KubeNova models clusters centrally and stores their kubeconfigs. First, base64‑encode your kubeconfig and register it together with the Capsule proxy URL for that cluster.

**Commands**

```bash
# 3.1) Choose a logical name for the cluster
export CLUSTER_NAME=${CLUSTER_NAME:-dev}

# 3.2) Base64‑encode your kubeconfig (single line)
export KUBE_B64=$(base64 < ~/.kube/config | tr -d '\n')

# 3.2b) Discover capsule-proxy URL for this cluster
# Example using a LoadBalancer Service; adjust to your environment.
export CAPSULE_PROXY_URL="https://capsule-proxy.example.com:9001"

# 3.3) Register the cluster
curl -sS -X POST "$BASE/api/v1/clusters" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{
    "name": "'"$CLUSTER_NAME"'",
    "kubeconfig": "'"$KUBE_B64"'",
    "capsuleProxyUrl": "'"$CAPSULE_PROXY_URL"'",
    "labels": { "env": "dev" }
  }' \
  | jq .
```

**What this does**

- `CLUSTER_NAME` is a human‑readable name (`dev`, `prod-a`, etc.).
- `KUBE_B64` holds your kubeconfig encoded as base64, as required by the `ClusterRegistration` schema.
- `POST /api/v1/clusters`:
  - persists the cluster in the store,
  - records `capsuleProxyUrl` on the cluster so every kubeconfig issued for this cluster targets capsule‑proxy, never the raw kube‑apiserver,
  - asynchronously starts installing the KubeNova agent into the target cluster (if `AGENT_IMAGE` and `MANAGER_URL_PUBLIC` are set),
  - returns a JSON `Cluster` with a stable `id` and any labels you provided.

Next, capture the cluster UID for subsequent calls.

```bash
# 3.4) Resolve the cluster UID from its name
export CLUSTER_ID=$(curl -sS "$BASE/api/v1/clusters?limit=200" \
  -H "$AUTH_HEADER" \
  | jq -r '.[] | select(.name=="'"$CLUSTER_NAME"'") | .id')

echo "$CLUSTER_ID"
```

If `CLUSTER_ID` is empty, the cluster registration did not succeed; re‑run step 3.3 and check the manager logs.

**kubectl checks – cluster and agent**

```bash
# All namespaces (sanity check)
kubectl get ns

# KubeNova agent deployment and HPA in the target cluster
kubectl get deploy -n kubenova-system
kubectl get hpa -n kubenova-system

# Agent logs (helpful if registration or heartbeats fail)
kubectl logs deploy/agent -n kubenova-system --tail=10

# bootstrap logs (helpful if registration or heartbeats fail)
kubectl logs job/kubenova-bootstrap -n kubenova-system

```

## 3.1) Basic Kubernetes checks with kubectl

Once your cluster is reachable by your own `kubectl` (outside of KubeNova), these checks help you understand cluster health and what KubeNova is acting on. Run them against the same kubeconfig you will use in step 3.

**Commands**

```bash
# Cluster and node health
kubectl cluster-info
kubectl get nodes -o wide

# Namespaces and workloads
kubectl get ns
kubectl get pods -A
kubectl get pods -A -o wide

# Events (recent cluster issues)
kubectl get events -A --sort-by=.lastTimestamp

# API resources and CRDs (make sure platform components are installed)
kubectl api-resources
kubectl get crds | head

# Storage and networking
kubectl get storageclass
kubectl get svc -A
kubectl get ingress -A

# Tenancy/app-delivery components (if you use them)
kubectl get pods -n capsule-system
kubectl get pods -n vela-system
kubectl get pods -n cert-manager

# Quick pod-level debug
kubectl describe pod -n <namespace> <pod-name>
kubectl logs -n <namespace> <pod-name> --tail=100
```

**What this does**

- `cluster-info` and `get nodes` confirm the Kubernetes API and worker nodes are healthy.
- `get ns` and `get pods -A` show what is actually running and where.
- `get events` surfaces recent failures (image pull errors, scheduling issues, etc.).
- `api-resources` and `get crds` confirm that tenancy/app-delivery CRDs are installed.
- The namespace-specific commands (`capsule-system`, `vela-system`, `cert-manager`) help you verify supporting controllers are up before you rely on KubeNova to interact with them.

These `kubectl` checks are optional from KubeNova’s perspective, but they are useful to confirm that the underlying cluster is ready for the API flows described in the next sections.

## 3.2) Confirm the Agent labels every Vela Application

The Agent projects apps into KubeVela and tags every Application with `kubenova.io/app-id`, `kubenova.io/tenant-id`, `kubenova.io/project-id`, and `kubenova.io/source-kind`. These labels keep `kubectl` queries safe and let the Manager detect drift.

```bash
# list Applications and show the KubeNova labels
kubectl get applications.core.oam.dev -n tn-<tenant>-app-<project> \
  -o jsonpath='{range .items[*]}{.metadata.name} {.metadata.labels.kubenova.io/app-id} {.metadata.labels.kubenova.io/tenant-id} {.metadata.labels.kubenova.io/project-id} {.metadata.labels.kubenova.io/source-kind}{"\n"}{end}'

# ask the Manager for orphaned Applications (admin/ops token required)
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/apps/orphans" \
  -H "$AUTH_HEADER" \
  | jq .
```

Legitimate apps are filtered out of the `/apps/orphans` response; only Applications that omit the KubeNova label or reference a missing App row are returned, which makes this a handy drift detection shortcut.

## 3.3) Provide credentials via SecretRefs

Private registries, Git repos, and Helm catalogs must be backed by Kubernetes secrets so the Agent can inject them into Vela workloads without storing raw credentials in the database.

1. **Docker registry secrets** (for `containerImage` sources / imagePullSecrets):

```bash
kubectl create secret docker-registry registry-creds \
  --docker-server=registry.example.com \
  --docker-username=platform-user \
  --docker-password=$REGISTRY_PASSWORD \
  --docker-email=ops@example.com \
  -n tn-<tenant>-app-<project>
```

2. **Git SSH secrets** (for `gitRepo` sources):

```bash
kubectl create secret generic git-ssh \
  --from-file=ssh-privatekey=./id_rsa \
  --from-file=known_hosts=./known_hosts \
  -n tn-<tenant>-app-<project>
```

3. **Helm repo credentials** (for `helmHttp` / `helmOci` sources):

```bash
kubectl create secret generic helm-creds \
  --type=kubernetes.io/basic-auth \
  --from-literal=username=helm-user \
  --from-literal=password="$HELM_PASSWORD" \
  -n tn-<tenant>-app-<project>
```

Reference these secrets via the `credentialsSecretRef` block in your App source:

```json
"source": {
  "kind": "containerImage",
  "containerImage": {
    "image": "registry.example.com/nginx:1.0.0",
    "credentialsSecretRef": {
      "name": "registry-creds",
      "namespace": "tn-acme-app-shop"
    }
  }
}
```

The Agent reads the `SecretRef` object (name + namespace) and injects it into `imagePullSecrets`, Helm repo auth, or Git repo auth for the rendered Vela Application, while the Manager never persists raw credentials in the database. Sandboxes may host their own secrets, but App deployments only use secrets in the app namespace to keep the platform secure.

---

## 4) Inspect cluster capabilities and bootstrap components

With the cluster registered, you can check what features are available and kick off bootstrap tasks such as installing tenancy or app‑delivery components.

**Commands**

```bash
# 4.1) Check cluster capabilities
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/capabilities" \
  -H "$AUTH_HEADER" \
  | jq .

# 4.2) Bootstrap tenancy (namespaces, quotas, RBAC, etc.)
curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/bootstrap/tenancy" \
  -H "$AUTH_HEADER" \
  -i

# 4.3) Optionally bootstrap access proxy and app delivery
curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/bootstrap/proxy" \
  -H "$AUTH_HEADER" \
  -i

curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/bootstrap/app-delivery" \
  -H "$AUTH_HEADER" \
  -i
```

**What this does**

- `GET /api/v1/clusters/{c}/capabilities` returns booleans for `tenancy`, `vela`, and `proxy`.
- `POST /api/v1/clusters/{c}/bootstrap/{component}` starts a bootstrap task for the chosen component and returns `202 Accepted`.
- Supported `component` values are:
  - `tenancy`
  - `proxy`
  - `app-delivery`

Bootstrap is asynchronous; watch your platform controllers and cluster state to see progress.

**kubectl checks – bootstrap components**

```bash
# Bootstrap job created by the Agent
kubectl get job -n kubenova-system kubenova-bootstrap
kubectl get pods -n kubenova-system

# Core add-ons installed by bootstrap
kubectl get pods -n cert-manager
kubectl get pods -n capsule-system
kubectl get pods -n vela-system

# Capsule Tenant CRD present
kubectl get crd tenants.capsule.clastix.io

# capsule-proxy service and external IP (used as capsuleProxyUrl per cluster)
kubectl get svc -n capsule-system capsule-proxy
```

---

## 5) Create and inspect a tenant

Tenants are the main multi‑tenant boundary. They own namespaces, quotas, and policies.

**Commands**

```bash
# 5.1) Create a tenant
export TENANT_NAME=${TENANT_NAME:-acme}

TENANT_JSON=$(
  curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants" \
    -H 'Content-Type: application/json' \
    -H "$AUTH_HEADER" \
    -d '{
      "name": "'"$TENANT_NAME"'",
      "owners": ["owner@example.com"],
      "labels": { "team": "platform" },
      "plan": "baseline"
    }'
)

echo "$TENANT_JSON" | jq .

# 5.2) Capture the tenant UID
export TENANT_ID=$(echo "$TENANT_JSON" | jq -r .id)
echo "$TENANT_ID"

# 5.3) List all tenants on the cluster
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants" \
  -H "$AUTH_HEADER" \
  | jq .

# 5.4) Fetch a single tenant by UID
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID" \
  -H "$AUTH_HEADER" \
  | jq .
```

**What this does**

- `POST /api/v1/clusters/{c}/tenants` creates a new tenant:
  - `name` is the logical name, unique per cluster.
  - `owners` is an array of e‑mail addresses or subjects that identify tenant owners.
  - `labels` lets you tag tenants for filtering (for example, `team=platform`).
  - `plan` is optional; when provided (for example `baseline` or `gold`), that plan is applied immediately. When omitted, the manager best‑effort applies the default plan configured via `KUBENOVA_DEFAULT_TENANT_PLAN` (default `baseline`) when available.
- The response includes a stable `id` used in many subsequent calls; you store it in `TENANT_ID`.
- `GET /api/v1/clusters/{c}/tenants` lists tenants; you can later add `labelSelector` or `owner` query parameters.
- `GET /api/v1/clusters/{c}/tenants/{t}` returns a single tenant by UID.
- Namespaces are still created lazily when you create projects (step 7); prior to that, quotas/limits are attached to the Capsule `Tenant` itself.

You can also update ownership and limits:

```bash
# 5.5) Update tenant owners
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/owners" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{"owners":["alice@example.com","ops@example.com"]}' \
  -i

# 5.6) View a summarized picture of the tenant
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/summary" \
  -H "$AUTH_HEADER" \
  | jq .
```

The `summary` endpoint aggregates namespaces, effective quotas, usage and (when plans are used) the applied plan.

**kubectl checks – tenant and namespaces**

```bash
# Capsule Tenant object (visible after quotas/plan or a project has been created)
kubectl get tenants.capsule.clastix.io
kubectl get tenants.capsule.clastix.io "$TENANT_NAME" -o yaml

# Namespaces that belong to this tenant (after you create a project)
kubectl get ns -l "capsule.clastix.io/tenant=$TENANT_NAME"
```

---

## 6) Plans, quotas, and usage

KubeNova can apply pre‑defined tenant plans and expose usage metrics at tenant and project scope.

**Commands**

```bash
# 6.1) List available plans
curl -sS "$BASE/api/v1/plans" \
  -H "$AUTH_HEADER" \
  | jq .

# 6.2) Apply a plan to the tenant (optional)
curl -sS -X PUT "$BASE/api/v1/tenants/$TENANT_ID/plan" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{"name":"baseline"}' \
  | jq .

# 6.3) Inspect tenant usage over the last 24 hours
curl -sS "$BASE/api/v1/tenants/$TENANT_ID/usage?range=24h" \
  -H "$AUTH_HEADER" \
  | jq .

# 6.4) Request a tenant/project-scoped proxy kubeconfig
curl -sS -X POST "$BASE/api/v1/tenants/$TENANT_ID/kubeconfig" \
  -H "$AUTH_HEADER" \
  -H 'Content-Type: application/json' \
  -d '{"project":"web","role":"projectDev","ttlSeconds":3600}' \
  | jq .

```

**Using tenant kubeconfigs with kubectl**

```bash
# Save a project-scoped kubeconfig to a file
TENANT_KCFG_B64=$(curl -sS -X POST "$BASE/api/v1/tenants/$TENANT_ID/kubeconfig" \
  -H "$AUTH_HEADER" \
  -H 'Content-Type: application/json' \
  -d '{"project":"web","role":"projectDev","ttlSeconds":3600}' \
  | jq -r '.kubeconfig')
printf "%s" "$TENANT_KCFG_B64" | base64 -d > kn-tenant-kubeconfig.yaml

# Use it against the access proxy (capsule-proxy) for that project
KUBECONFIG=kn-tenant-kubeconfig.yaml kubectl get ns
KUBECONFIG=kn-tenant-kubeconfig.yaml kubectl get pods -n web
```

If these commands fail:

- with `no route to host`, verify that the `capsuleProxyUrl` you configured for the cluster points to a reachable capsule‑proxy URL (including the correct port) and that the `capsule-proxy` Service has an accessible `EXTERNAL-IP`.
- with `the server doesn't have a resource type "pods"/"ns"`, make sure `capsuleProxyUrl` is the capsule‑proxy endpoint, not the Manager’s `/api/v1` URL.

**What this does**

- `GET /api/v1/plans` returns the configured plan catalog loaded from `pkg/catalog/plans.json`.
- `PUT /api/v1/tenants/{t}/plan`:
  - validates the requested plan name,
  - applies quotas and PolicySets associated with that plan,
  - records the chosen plan on the tenant.
- `GET /api/v1/tenants/{t}/usage` aggregates `cpu`, `memory`, and `pods` for the tenant, using live ResourceQuota data when available, falling back to example values in dev/test.
- `POST /api/v1/tenants/{t}/kubeconfig` returns kubeconfigs targeting the per-cluster access proxy configured via `capsuleProxyUrl` on the associated cluster:
  - with `project`, it issues a project-scoped kubeconfig for that tenant/project,
  - `role` and `ttlSeconds` control the logical role and optional TTL; roles map to Kubernetes RBAC via Capsule.

**Customizing plans**

- Plans and PolicySets are defined in the embedded catalog under `pkg/catalog/plans.json` and `pkg/catalog/policysets.json`.
- To customize them for your environment, edit those JSON files in your fork and rebuild/redeploy the Manager image; the `/api/v1/plans` and plan application behavior will then reflect your changes.
- The default plan applied when `plan` is omitted on tenant creation is controlled by the `KUBENOVA_DEFAULT_TENANT_PLAN` env var (default `baseline`).

**Example – switch default plan to gold**

```bash
# In the Manager environment (.env, Helm values, or deployment)
export KUBENOVA_DEFAULT_TENANT_PLAN=gold

# After restarting the Manager:
curl -sS "$BASE/api/v1/features" | jq .
# ...
# "defaultTenantPlan": "gold",
# "availablePlans": ["baseline","gold"]
```

Now a call to:

```bash
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{"name":"acme"}'
```

will best‑effort apply the `gold` plan to `acme` (as long as a `gold` plan exists in the catalog).

## Namespace model

KubeNova enforces deterministic namespace names per tenant/project or tenant sandbox so every cluster can host large numbers of tenants without conflicting names:

- App namespaces follow `tn-<tenant>-app-<project>` and are created by the Manager when a project is declared. Agents, Capsule, and KubeVela treat those namespaces as read-only for tenants.
- Sandbox namespaces use `tn-<tenant>-sandbox-<name>` and are owned by tenant groups (`<tenant>-devs`). They are never reconciled by the Agent and allow tenant-level mutations.
- Namespaces carry labels (`kubenova.tenant`, `kubenova.project`, `kubenova.namespace-type`, `kubenova.io/sandbox=true`) so telemetry and controllers can filter app vs sandbox contexts.

List all app namespaces via `kubectl get ns -l kubenova.namespace-type=app` and sandboxes via `kubectl get ns -l kubenova.io/sandbox=true`.

## Tenant → Project → App hierarchy

The Manager API mirrors real-world workflows:

1. Register a cluster → the Manager stores its kubeconfig, installs an Agent, and configures Capsule + capsule-proxy.
2. Create a tenant (`/clusters/{c}/tenants`) → defines the tenant identity, owners, and RBAC labels.
3. Create a project (`/clusters/{c}/tenants/{t}/projects`) → results in the Capsule Tenant namespace `tn-<tenant>-app-<project>`.
4. Create an App (`/clusters/{c}/tenants/{t}/projects/{p}/apps`) → the App is persisted with canonical `kubenova.io/app-id` metadata, and the Agent projects it into a KubeVela Application inside the project namespace.

# App.source examples

Use the JSON payloads under `docs/examples/` to bootstrap apps:

- [`catalog-item.json`](docs/examples/catalog-item.json) – defines a catalog entry (slug, source, version, scope). POST it to `/api/v1/catalog`.
- [`app-create.json`](docs/examples/app-create.json) – describes a container-image App, traits, and a `SecretRef`. POST it to `/api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps`.
- [`app-upgrade.json`](docs/examples/app-upgrade.json) – shows how to bump the catalog version and overrides when installing the same slug twice.
- [`secret-ref.json`](docs/examples/secret-ref.json) – highlights the minimal `SecretRef` object for registry, Git, or Helm credentials.
- [`app-helm.json`](docs/examples/app-helm.json) – example Helm (grafana) install referencing `grafana.github.io/helm-charts`.
- [`app-oci.json`](docs/examples/app-oci.json) – OCI chart example from `registry-1.docker.io/grafana`.

Include these snippets directly in your automation (curl/HTTP/CI) to stay in sync with the OpenAPI contract.

## Sandbox usage guide

Sandboxes are tenant-owned namespaces created via `POST /api/v1/tenants/{t}/sandbox`. A typical sandbox workflow:

1. Create a namespace (`{"name":"dev"}`) and receive a sandbox-scoped kubeconfig.
2. Use the kubeconfig via capsule-proxy; tenants have full edit access but the Manager/Agent never touch the namespace.
3. Sandboxes can host secrets and experiments independently of app namespaces; the Agent skips reconciliation when it sees `kubenova.io/sandbox=true`.

## Quickstart checklist

1. Register the cluster with `capsuleProxyUrl` pointing to capsule-proxy and capture `CLUSTER_ID`.
2. Create a tenant (`TENANT_ID`) and project (`PROJECT_ID`); the Manager creates the app namespace automatically.
3. Create a sandbox (`POST /api/v1/tenants/{TENANT_ID}/sandbox`) and test `kubectl` against its kubeconfig.
4. Install nginx from the catalog using `docs/examples/catalog-item.json` via `POST /api/v1/clusters/{CLUSTER_ID}/tenants/{TENANT_ID}/projects/{PROJECT_ID}/catalog/install`.
5. Upgrade nginx with `docs/examples/app-upgrade.json` by calling the same install endpoint with a higher version and overrides.

Each step is tied to the JSON examples under `docs/examples/`.

**kubectl checks – quotas and limits**

```bash
# Quotas and limits attached to the Capsule Tenant (after plan apply)
kubectl get tenants.capsule.clastix.io "$TENANT_NAME" -o yaml
```

---

## 7) Create a project within the tenant

Projects group workloads and namespaces within a tenant. Each project maps to a Kubernetes namespace and has its own access controls.

**Commands**

```bash
# 7.1) Create a project
export PROJECT_NAME=${PROJECT_NAME:-web}

PROJECT_JSON=$(
  curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects" \
    -H 'Content-Type: application/json' \
    -H "$AUTH_HEADER" \
    -d '{"name":"'"$PROJECT_NAME"'"}'
)

echo "$PROJECT_JSON" | jq .

# 7.2) Capture the project UID
export PROJECT_ID=$(echo "$PROJECT_JSON" | jq -r .id)
echo "$PROJECT_ID"

# 7.3) List projects in the tenant
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects" \
  -H "$AUTH_HEADER" \
  | jq .

# 7.4) Get a single project
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID" \
  -H "$AUTH_HEADER" \
  | jq .
```

**What this does**

- `POST /api/v1/clusters/{c}/tenants/{t}/projects` creates a project and ensures a Kubernetes namespace exists with appropriate labels (including Capsule tenant labels).
- The response includes a project `id`, stored in `PROJECT_ID` for later use.
- `GET /api/v1/clusters/{c}/tenants/{t}/projects` and `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}` let you browse and inspect projects.

You can grant access to specific users and request a project‑scoped kubeconfig:

```bash
# 7.5) Grant project access to a developer
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/access" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{
    "members": [
      { "subject": "dev@example.com", "role": "projectDev" }
    ]
  }' \
  -i

# 7.6) Request a project‑scoped proxy kubeconfig
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/kubeconfig" \
  -H "$AUTH_HEADER" \
  | jq .

# 7.7) Check project usage
curl -sS "$BASE/api/v1/projects/$PROJECT_ID/usage?range=24h" \
  -H "$AUTH_HEADER" \
  | jq .
```

**kubectl checks – project namespace, limits, and access**

```bash
# Namespace created for the project and its labels
kubectl get ns "$PROJECT_NAME" --show-labels
kubectl get ns -l "kubenova.tenant=$TENANT_NAME,kubenova.project=$PROJECT_NAME"

# ResourceQuota and LimitRange applied in the project namespace
kubectl get resourcequota -n "$PROJECT_NAME"
kubectl get limitrange -n "$PROJECT_NAME"
kubectl describe resourcequota -n "$PROJECT_NAME" kubenova-default-quota
kubectl describe limitrange -n "$PROJECT_NAME" kubenova-default-limits

# RBAC objects created for project members
kubectl get role,rolebinding -n "$PROJECT_NAME" | grep "kubenova:$TENANT_NAME:$PROJECT_NAME" || true
```

---

## 8) Deploy and manage an app

Apps describe workloads delivered via the app‑delivery backend (for example, KubeVela). You create an app under a project, then deploy and observe it.

**Commands**

```bash
# 8.1) Create an app
export APP_NAME=${APP_NAME:-hello}

APP_JSON=$(
  curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps" \
    -H 'Content-Type: application/json' \
    -H "$AUTH_HEADER" \
    -d '{"name":"'"$APP_NAME"'"}'
)

echo "$APP_JSON" | jq .

# 8.2) Capture the app UID
export APP_ID=$(echo "$APP_JSON" | jq -r .id)
echo "$APP_ID"

# 8.3) List apps in the project
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps" \
  -H "$AUTH_HEADER" \
  | jq .

# 8.4) Deploy the app
curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:deploy" \
  -H "$AUTH_HEADER" \
  -i

# 8.5) Check app status
curl -sS \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/status" \
  -H "$AUTH_HEADER" \
  | jq .

# 8.6) Fetch recent logs for a component
curl -sS \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/logs/web" \
  -H "$AUTH_HEADER" \
  | jq .
```

**What this does**

- `POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps` creates a new app resource with the given `name`.
- `POST /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}:deploy` triggers an app deployment.
- `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/status` returns a high‑level `phase`, conditions, and timestamps.
- `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps/{a}/logs/{component}` streams recent log lines grouped by component.

You can also:

- Suspend/resume an app:

  ```bash
  curl -sS -X POST \
    "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:suspend" \
    -H "$AUTH_HEADER" \
    -i

  curl -sS -X POST \
    "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:resume" \
    -H "$AUTH_HEADER" \
    -i
  ```

- Inspect revisions and diffs:

  ```bash
  curl -sS \
    "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/revisions" \
    -H "$AUTH_HEADER" \
    | jq .

  curl -sS \
    "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/diff/1/2" \
    -H "$AUTH_HEADER" \
    | jq .
  ```

- Tune traits and policies, or update images:

  ```bash
  curl -sS -X PUT \
    "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/traits" \
    -H 'Content-Type: application/json' \
    -H "$AUTH_HEADER" \
    -d '[{"type":"scaler","properties":{"replicas":2}}]' \
    -i

  curl -sS -X PUT \
    "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/policies" \
    -H 'Content-Type: application/json' \
    -H "$AUTH_HEADER" \
    -d '[{"type":"rollout","properties":{"maxUnavailable":1}}]' \
    -i

  curl -sS -X POST \
    "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/image-update" \
    -H 'Content-Type: application/json' \
    -H "$AUTH_HEADER" \
    -d '{"component":"web","image":"nginx","tag":"1.25.3"}' \
    -i
  ```

When you are done experimenting with an app, you can delete it:

```bash
curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:delete" \
  -H "$AUTH_HEADER" \
  -i
```

**kubectl checks – app, pods, and revisions**

```bash
# Vela Application and revisions created for this app
kubectl get applications.core.oam.dev -n "$PROJECT_NAME"
kubectl get applicationrevisions.core.oam.dev -n "$PROJECT_NAME"

# Application details and status
kubectl describe application.core.oam.dev "$APP_NAME" -n "$PROJECT_NAME"

# Workload pods owned by this app
kubectl get pods -n "$PROJECT_NAME" -l "app.oam.dev/name=$APP_NAME"
kubectl logs -n "$PROJECT_NAME" -l "app.oam.dev/name=$APP_NAME" --tail=100
```

---

## 9) Browse the catalog

The catalog endpoints expose the available components, traits, and workflows that the app‑delivery backend supports.

**Commands**

```bash
curl -sS "$BASE/api/v1/catalog/components" \
  -H "$AUTH_HEADER" \
  | jq .

curl -sS "$BASE/api/v1/catalog/traits" \
  -H "$AUTH_HEADER" \
  | jq .

curl -sS "$BASE/api/v1/catalog/workflows" \
  -H "$AUTH_HEADER" \
  | jq .
```

**What this does**

- `GET /api/v1/catalog/components` lists available workload component types.
- `GET /api/v1/catalog/traits` lists traits (for example, scalers or traffic policies).
- `GET /api/v1/catalog/workflows` lists workflows (for example, rollout strategies).

These endpoints return simple JSON arrays and are safe to call frequently.

### 9.1) Catalog items & install

```bash
# List catalog entries
curl -sS "$BASE/api/v1/catalog?scope=global" \
  -H "$AUTH_HEADER" \
  | jq .

# Inspect a single catalog item
curl -sS "$BASE/api/v1/catalog/nginx" \
  -H "$AUTH_HEADER" \
  | jq .

# Install from the catalog
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/catalog/install" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{
    "slug":"nginx",
    "source": {
      "containerImage": { "tag": "1.22.0" }
    }
  }' \
  | jq .
```

- `GET /api/v1/catalog` returns the catalog entries for the requested `scope` (global by default) and, when `scope=tenant`, the `tenantId` that you pass.
- `GET /api/v1/catalog/{slug}` surfaces the stored source definition so you can preview the Helm chart, Git repo, container image, or KubeManifest that underpins the template.
- `POST /clusters/{cluster}/tenants/{tenant}/projects/{project}/catalog/install` merges overrides into the catalog source, persists an App with a `catalogRef`, and mirrors the metadata into the project ConfigMap so the Agent can project the App into Vela.
- The install endpoint returns `{ "status": "accepted", "appSlug": "<slug>" }` because delivery happens asynchronously through the Agent.
- Re-running the install call with the same `slug` but a newer catalog version acts as an upgrade; the request records `catalogItemId`, `catalogVersion`, and `catalogOverrides` so the Agent and the dashboard know which template boundaries were applied.

---

## 10) Sandbox namespaces & kubeconfigs

Sandbox namespaces give tenants a writable playground that KubeNova never manages through Vela.

```bash
# 10.1) Create a sandbox namespace with a proxy kubeconfig
curl -sS -X POST "$BASE/api/v1/tenants/$TENANT_ID/sandbox" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{
    "name":"playground",
    "ttlSeconds":3600
  }' \
  | jq .
```

The controller (`internal/http/server.go`) decodes the tenant, checks the `capsuleProxyUrl` recorded on the tenant’s primary cluster, calls `clusterpkg.EnsureSandboxNamespace` (which uses `internal/cluster/projects.go`, `cluster/namespaces.go`, `cluster/rbac.go`, and the Capsule tenant helpers) to create `tn-<tenant>-sandbox-<name>` with `kubenova.io/sandbox=true`, and issues a tenantOwner kubeconfig via `clusterpkg.IssueSandboxKubeconfig`. The response includes `namespace`, `kubeconfig`, `expiresAt`, and other metadata and is covered by `internal/http/server_sandbox_test.go`.

```bash
# 10.2) Use the sandbox kubeconfig
SANDBOX_KCFG_B64=$(curl -sS -X POST "$BASE/api/v1/tenants/$TENANT_ID/sandbox" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{"name":"playground"}' \
  | jq -r '.kubeconfig')
printf "%s" "$SANDBOX_KCFG_B64" | base64 -d > sandbox-kubeconfig.yaml
KUBECONFIG=sandbox-kubeconfig.yaml kubectl get ns
KUBECONFIG=sandbox-kubeconfig.yaml kubectl get pods -n "tn-$TENANT_NAME-sandbox-playground"
```

- Sandbox namespaces always carry `kubenova.io/sandbox=true`, `kubenova.tenant`, and `kubenova.project` labels (set by `EnsureSandboxNamespace`), the Capsule `capsule.clastix.io/tenant` label, and the `kubenova.namespace-type=sandbox` label so the Agent (`internal/reconcile/project.go`) ignores them.
- `clusterpkg.ensureSandboxClusterRole` binds the `<tenant>-devs` group to the `kubenova-sandbox-editor` ClusterRole defined in `internal/cluster/rbac.go`, giving tenants full edit rights inside sandboxes while app namespaces stayed read-only.
- The Agent’s AppReconciler (`internal/reconcile/app.go`) only watches ConfigMaps in non-sandbox namespaces, so Vela never mutates sandbox workloads; tenants can create their own objects freely with the sandbox kubeconfig.

```
kubectl get ns -l "kubenova.io/sandbox=true"
kubectl describe ns tn-$TENANT_NAME-sandbox-playground
kubectl get rolebindings -n tn-$TENANT_NAME-sandbox-playground
```

---

## 11) One-shot PaaS bootstrap (optional)

Once you are comfortable with the individual steps above, you can use a single
endpoint to create a default tenant, project, and project-scoped kubeconfig on
an existing cluster. This is driven by `KUBENOVA_BOOTSTRAP_*` environment
variables (see `env.example`).

**Command**

```bash
curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/bootstrap/paas" \
  -H "$AUTH_HEADER" \
  | jq .
```

Example response:

```json
{
  "cluster": "03d95dfd-a551-4dfa-a48f-2f49390704c1",
  "tenant": "acme",
  "tenantId": "3a7f5d62-2a0b-4b3e-bc39-3b3f1f33b111",
  "project": "web",
  "projectId": "4f1e4c8a-8f9a-4b1e-9d92-1b2c3d4e5f61",
  "kubeconfig": "YXBpVmVyc2lvbjogdjEK...",
  "expiresAt": "2025-01-01T01:00:00Z"
}
```

**Using the returned kubeconfig**

```bash
PAAST_KCFG_B64=$(curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/bootstrap/paas" \
  -H "$AUTH_HEADER" | jq -r '.kubeconfig')
printf "%s" "$PAAST_KCFG_B64" | base64 -d > paas-kubeconfig.yaml

KUBECONFIG=paas-kubeconfig.yaml kubectl get ns
KUBECONFIG=paas-kubeconfig.yaml kubectl get pods -n web
```

**Optional – create a tenantOwner kubeconfig for the same tenant/project**

```bash
# Extract tenant and project IDs from the PaaS bootstrap response
BOOTSTRAP_JSON=$(
  curl -sS -X POST \
    "$BASE/api/v1/clusters/$CLUSTER_ID/bootstrap/paas" \
    -H "$AUTH_HEADER"
)

export TENANT_ID=$(echo "$BOOTSTRAP_JSON" | jq -r '.tenantId')
export PROJECT_ID=$(echo "$BOOTSTRAP_JSON" | jq -r '.projectId')

# Set a logical owner subject for the tenant and mark it as owner in Capsule
curl -sS -X PUT \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/owners" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{"owners":["owner@example.com"]}' \
  -i

# Issue a tenantOwner kubeconfig scoped to the bootstrap project
TENANT_OWNER_KCFG_B64=$(curl -sS -X POST \
  "$BASE/api/v1/tenants/$TENANT_ID/kubeconfig" \
  -H "$AUTH_HEADER" \
  -H 'Content-Type: application/json' \
  -d '{"project":"web","role":"tenantOwner","ttlSeconds":3600}' \
  | jq -r '.kubeconfig')
printf "%s" "$TENANT_OWNER_KCFG_B64" | base64 -d > paas-tenant-owner-kubeconfig.yaml

KUBECONFIG=paas-tenant-owner-kubeconfig.yaml kubectl get pods -n web
```

**What this does**

- Creates (or reuses) a tenant named `KUBENOVA_BOOTSTRAP_TENANT_NAME` (default `acme`) on the target cluster.
- Optionally applies the plan configured via `KUBENOVA_BOOTSTRAP_TENANT_PLAN` (default `baseline`).
- Creates (or reuses) a project named `KUBENOVA_BOOTSTRAP_PROJECT_NAME` (default `web`) in that tenant and ensures its namespace exists.
- Issues a project-scoped kubeconfig (role `projectDev`) for that tenant/project with TTL controlled by `KUBENOVA_BOOTSTRAP_KUBECONFIG_TTL` (default `3600` seconds).
  - It is **namespaced** to the bootstrap project; cluster-scoped operations like `kubectl get ns` are expected to be forbidden by RBAC.

You can now use `paas-kubeconfig.yaml` directly to deploy apps into the
bootstrap project (for example, `kubectl get pods -n web`) using `kubectl` or higher-level tools.

---

## 12) Clean up resources

To remove resources created during this quickstart:

**Commands**

```bash
# 12.1) Delete the app (if still present)
curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:delete" \
  -H "$AUTH_HEADER" \
  -i

# 12.2) Delete the project
curl -sS -X DELETE \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID" \
  -H "$AUTH_HEADER" \
  -i

# 12.3) Delete the tenant
curl -sS -X DELETE \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID" \
  -H "$AUTH_HEADER" \
  -i

# 12.4) Delete the cluster
curl -sS -X DELETE \
  "$BASE/api/v1/clusters/$CLUSTER_ID" \
  -H "$AUTH_HEADER" \
  -i
```

**What this does**

- Deletes the app, project, tenant, and cluster in reverse order.
- Leaves your underlying Kubernetes cluster intact; only KubeNova‑managed records and installed agents are cleaned up according to the delete handlers.

If you need to perform low‑level cleanup inside the Kubernetes cluster itself (for example, force‑removing namespaces), use your platform’s runbooks or `kubectl`‑based scripts outside of the KubeNova API.

---

## Where to go next

- For the full OpenAPI contract and example payloads, see `docs/openapi/openapi.yaml`.
- For a high‑level overview and endpoint matrix, see `docs/README.md`.
- For future and not‑yet‑implemented endpoints, track the roadmap via GitHub issues and milestones and the OpenAPI file; they are intentionally omitted from this quickstart until fully implemented in `internal/http`.
