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
- `GET /api/v1/features` tells you which high‑level capabilities are enabled (tenancy, proxy, app delivery).

If `healthz` or `readyz` returns a non‑200 status, check the manager logs before continuing.

---



## 3) Register a cluster

KubeNova models clusters centrally and stores their kubeconfigs. First, base64‑encode your kubeconfig and register it.

**Commands**

```bash
# 3.1) Choose a logical name for the cluster
export CLUSTER_NAME=${CLUSTER_NAME:-dev}

# 3.2) Base64‑encode your kubeconfig (single line)
export KUBE_B64=$(base64 < ~/.kube/config | tr -d '\n')

# 3.3) Register the cluster
curl -sS -X POST "$BASE/api/v1/clusters" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{
    "name": "'"$CLUSTER_NAME"'",
    "kubeconfig": "'"$KUBE_B64"'",
    "labels": { "env": "dev" }
  }' \
  | jq .
```

**What this does**

- `CLUSTER_NAME` is a human‑readable name (`dev`, `prod-a`, etc.).
- `KUBE_B64` holds your kubeconfig encoded as base64, as required by the `ClusterRegistration` schema.
- `POST /api/v1/clusters`:
  - persists the cluster in the store,
  - asynchronously starts installing the KubeNova agent into the target cluster (if `AGENT_IMAGE` and `MANAGER_URL_PUBLIC` are set),
  - returns a JSON `Cluster` with a stable `uid` and any labels you provided.

Next, capture the cluster UID for subsequent calls.

```bash
# 3.4) Resolve the cluster UID from its name
export CLUSTER_ID=$(curl -sS "$BASE/api/v1/clusters?limit=200" \
  -H "$AUTH_HEADER" \
  | jq -r '.[] | select(.name=="'"$CLUSTER_NAME"'") | .uid')

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
      "labels": { "team": "platform" }
    }'
)

echo "$TENANT_JSON" | jq .

# 5.2) Capture the tenant UID
export TENANT_ID=$(echo "$TENANT_JSON" | jq -r .uid)
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
- The response includes a stable `uid` used in many subsequent calls; you store it in `TENANT_ID`.
- `GET /api/v1/clusters/{c}/tenants` lists tenants; you can later add `labelSelector` or `owner` query parameters.
- `GET /api/v1/clusters/{c}/tenants/{t}` returns a single tenant by UID.
- At this stage the tenant exists in KubeNova’s store only; the Capsule `Tenant` CR and any namespaces are created lazily when you attach quotas/limits or a plan (step 6) or create a project (step 7).

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

# 6.4) Request a tenant‑scoped proxy kubeconfig
curl -sS -X POST "$BASE/api/v1/tenants/$TENANT_ID/kubeconfig" \
  -H "$AUTH_HEADER" \
  | jq .

# 6.4b) Tenant-scoped tenantOwner kubeconfig (1 hour TTL)
curl -sS -X POST "$BASE/api/v1/tenants/$TENANT_ID/kubeconfig" \
  -H "$AUTH_HEADER" \
  -H 'Content-Type: application/json' \
  -d '{"role":"tenantOwner","ttlSeconds":3600}' \
  | jq .

# 6.4c) Project-scoped projectDev kubeconfig (1 hour TTL)
curl -sS -X POST "$BASE/api/v1/tenants/$TENANT_ID/kubeconfig" \
  -H "$AUTH_HEADER" \
  -H 'Content-Type: application/json' \
  -d '{"project":"web","role":"projectDev","ttlSeconds":3600}' \
  | jq .
```

**Using tenant kubeconfigs with kubectl**

```bash
# Save a tenant-scoped kubeconfig to a file
TENANT_KCFG_B64=$(curl -sS -X POST "$BASE/api/v1/tenants/$TENANT_ID/kubeconfig" \
  -H "$AUTH_HEADER" | jq -r '.kubeconfig')
printf "%s" "$TENANT_KCFG_B64" | base64 -d > kn-tenant-kubeconfig.yaml

# Use it against the access proxy (capsule-proxy)
KUBECONFIG=kn-tenant-kubeconfig.yaml kubectl get ns
KUBECONFIG=kn-tenant-kubeconfig.yaml kubectl get pods -A
```

**What this does**

- `GET /api/v1/plans` returns the configured plan catalog loaded from `pkg/catalog/plans.json`.
- `PUT /api/v1/tenants/{t}/plan`:
  - validates the requested plan name,
  - applies quotas and PolicySets associated with that plan,
  - records the chosen plan on the tenant.
- `GET /api/v1/tenants/{t}/usage` aggregates `cpu`, `memory`, and `pods` for the tenant, using live ResourceQuota data when available, falling back to example values in dev/test.
- `POST /api/v1/tenants/{t}/kubeconfig` returns kubeconfigs targeting the configured access proxy (`CAPSULE_PROXY_URL`):
  - without a body, it issues a tenant-scoped read-only kubeconfig with unlimited TTL,
  - with `role` and `ttlSeconds`, it records a requested role and expiry metadata,
  - with `project`, it scopes the kubeconfig to that project’s namespace while still validating the tenant,
  - when `role` is `projectDev`, a `project` must be provided (otherwise the request is rejected with KN-422).

**kubectl checks – quotas and limits**

```bash
# Quotas and limits attached to the Capsule Tenant
kubectl get tenants.capsule.clastix.io "$TENANT_NAME" -o yaml

# After you create a project/namespace, inspect effective limits there
kubectl get resourcequota,limitrange -n "$PROJECT_NAME"
kubectl describe resourcequota -n "$PROJECT_NAME"
kubectl describe limitrange -n "$PROJECT_NAME"
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
export PROJECT_ID=$(echo "$PROJECT_JSON" | jq -r .uid)
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
- The response includes a project `uid`, stored in `PROJECT_ID` for later use.
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
export APP_ID=$(echo "$APP_JSON" | jq -r .uid)
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

---

## 10) Clean up resources

To remove resources created during this quickstart:

**Commands**

```bash
# 10.1) Delete the app (if still present)
curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:delete" \
  -H "$AUTH_HEADER" \
  -i

# 10.2) Delete the project
curl -sS -X DELETE \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID" \
  -H "$AUTH_HEADER" \
  -i

# 10.3) Delete the tenant
curl -sS -X DELETE \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID" \
  -H "$AUTH_HEADER" \
  -i

# 10.4) Delete the cluster
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
- For future and not‑yet‑implemented endpoints, track the roadmap in `docs/roadmap.md` and the OpenAPI file; they are intentionally omitted from this quickstart until fully implemented in `internal/http`.
