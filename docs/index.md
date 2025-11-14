---
title: KubeNova API v1 — Interactive Guide
---

# KubeNova API v1 — Interactive Guide

Use these copy‑ready snippets to exercise every endpoint. Paths use UUIDv4 (lowercase). Click the copy icon on code blocks to paste commands quickly.

## 0) Quick setup

```bash
export BASE=${BASE:-http://localhost:8080}
# export KN_TOKEN="<jwt>"   # Optional; only when auth is enabled
AUTH=${KN_TOKEN:+-H "Authorization: Bearer $KN_TOKEN"}
```

::: tip Tip
All responses shown below are examples. Real values (UIDs, timestamps) will differ.
:::

## 1) Access & system

Issue a token
```bash
curl -sS -X POST "$BASE/api/v1/tokens" -H 'Content-Type: application/json' \
  -d '{"subject":"demo","roles":["admin"],"ttlSeconds":3600}'
```

Who am I?
```bash
curl -sS "$BASE/api/v1/me" $AUTH
```

Version & features
```bash
curl -sS "$BASE/api/v1/version" $AUTH
curl -sS "$BASE/api/v1/features" $AUTH
```

Health & readiness
```bash
curl -sS "$BASE/api/v1/healthz"
curl -sS "$BASE/api/v1/readyz"
```

## 2) Clusters

Register cluster
```bash
export CLUSTER_NAME=${CLUSTER_NAME:-dev}
export KUBE_B64=$(base64 < ~/.kube/config | tr -d '\n')
curl -sS -X POST "$BASE/api/v1/clusters" -H 'Content-Type: application/json' $AUTH \
  -d '{"name":"'$CLUSTER_NAME'","kubeconfig":"'$KUBE_B64'","labels":{"env":"dev"}}'
```

Resolve cluster UID
```bash
export CLUSTER_ID=$(curl -sS "$BASE/api/v1/clusters?limit=200" $AUTH \
  | jq -r '.[] | select(.name=="'$CLUSTER_NAME'") | .uid')
echo "$CLUSTER_ID"
```

List & get
```bash
curl -sS "$BASE/api/v1/clusters?limit=50&labelSelector=env%3Ddev" $AUTH
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID" $AUTH | jq .
```

::: details Example response — list clusters
```json
[
  {
    "uid": "5f1e4c8a-8f9a-4b1e-9d92-1b2c3d4e5f62",
    "name": "dev",
    "labels": { "env": "dev" },
    "createdAt": "2025-01-01T12:00:00Z"
  }
]
```
:::

Capabilities & bootstrap
```bash
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/capabilities" $AUTH | jq .
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/bootstrap/tenancy" $AUTH -i
```

## 3) Tenants

Create tenant and capture UID
```bash
export TENANT_NAME=${TENANT_NAME:-acme}
TENANT_JSON=$(curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants" \
  -H 'Content-Type: application/json' $AUTH \
  -d '{"name":"'$TENANT_NAME'","owners":["owner@example.com"],"labels":{"team":"platform"}}')
export TENANT_ID=$(echo "$TENANT_JSON" | jq -r .uid)
echo "$TENANT_ID"
```
::: tip Tenant → Cluster mapping
Tenants created via `/clusters/{c}/tenants` are bound to that cluster as their primary home. KubeNova records the cluster UID on the tenant (`labels.kubenova.cluster`), and usage/kubeconfig lookups use this mapping instead of guessing. If a tenant was created before this label existed, KubeNova falls back to the first registered cluster.
:::

List/get/owners/quotas/limits/netpols/summary
```bash
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants" $AUTH | jq .
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID" $AUTH | jq .
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/owners" \
  -H 'Content-Type: application/json' $AUTH -d '{"owners":["alice@example.com","ops@example.com"]}' -i
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/quotas" \
  -H 'Content-Type: application/json' $AUTH -d '{"cpu":"4","memory":"8Gi"}' -i
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/limits" \
  -H 'Content-Type: application/json' $AUTH -d '{"pods":"50"}' -i
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/network-policies" \
  -H 'Content-Type: application/json' $AUTH -d '{"defaultDeny":true}' -i
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/summary" $AUTH | jq .
```
::: tip Tenant limits
Quotas, limits, and network policies are applied at the Capsule `Tenant` level:
- `quotas` → `spec.resourceQuotas` (plus a `kubenova.io/quotas` annotation for reporting).
- `limits` → `spec.limitRanges`.
- `network-policies` → `spec.networkPolicies`.
`/summary` reports effective quotas, lists namespaces labeled for the Tenant, and includes a `usages` map derived from namespace `ResourceQuota` usage (cpu, memory, pods) when available.
:::

Filter tenants by labels and owner
```bash
# Label selector (k=v[,k=v])
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants?labelSelector=env%3Dprod,tier%3Dgold" $AUTH | jq .
# Owner e-mail/subject
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants?owner=alice@example.com" $AUTH | jq .
```

## 4) Projects

Create project and capture UID
```bash
export PROJECT_NAME=${PROJECT_NAME:-web}
PROJECT_JSON=$(curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects" \
  -H 'Content-Type: application/json' $AUTH -d '{"name":"'$PROJECT_NAME'"}')
export PROJECT_ID=$(echo "$PROJECT_JSON" | jq -r .uid)
echo "$PROJECT_ID"
```

List/get/update/access/kubeconfig
```bash
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects" $AUTH | jq .
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID" $AUTH | jq .
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID" \
  -H 'Content-Type: application/json' $AUTH -d '{"labels":{"tier":"gold"}}' -i
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/access" \
  -H 'Content-Type: application/json' $AUTH -d '{"members":[{"subject":"dev@example.com","role":"projectDev"}]}' -i
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/kubeconfig" $AUTH | jq .
```
::: tip Projects → Namespaces & access
Creating a project ensures a Kubernetes Namespace exists with labels:
- `kubenova.project=<project>`, `kubenova.tenant=<tenant>`, `capsule.clastix.io/tenant=<tenant>`.
`access` updates create per-project `Role` and `RoleBinding` objects in that namespace for each member, using the role to determine allowed verbs.
Project kubeconfigs are proxy-based: they point at `CAPSULE_PROXY_URL` and are intended to be used together with short-lived tokens issued by capsule-proxy.
:::

## 5) Apps

Create app and capture UID
```bash
export APP_NAME=${APP_NAME:-hello}
APP_JSON=$(curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps" \
  -H 'Content-Type: application/json' $AUTH -d '{"name":"'$APP_NAME'"}')
export APP_ID=$(echo "$APP_JSON" | jq -r .uid)
echo "$APP_ID"
```

List/get
```bash
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps" $AUTH | jq .
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID" $AUTH | jq .
```

Deploy/suspend/resume & status/revisions/diff/logs
```bash
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:deploy" $AUTH -i
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:suspend" $AUTH -i
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:resume" $AUTH -i
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/status" $AUTH | jq .
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/revisions" $AUTH | jq .
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/diff/1/2" $AUTH | jq .
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/logs/web" $AUTH | jq .
```

Traits/policies/image update & delete
```bash
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/traits" \
  -H 'Content-Type: application/json' $AUTH -d '[{"type":"scaler","properties":{"replicas":2}}]' -i
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/policies" \
  -H 'Content-Type: application/json' $AUTH -d '[{"type":"rollout","properties":{"maxUnavailable":1}}]' -i
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/image-update" \
  -H 'Content-Type: application/json' $AUTH -d '{"component":"web","image":"nginx","tag":"1.25.3"}' -i
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:delete" $AUTH -i
```

Note
- Traits/Policies return 200 OK with no response body.
- Image Update and Delete return 202 Accepted with no response body; operations are asynchronous and idempotent.

## 6) PolicySets

```bash
# List tenant policy sets
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/policysets" $AUTH | jq .

# Create a policy set
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/policysets" \
  -H 'Content-Type: application/json' $AUTH \
  -d '{"name":"baseline","description":"Base guardrails","rules":[]}' -i

# Get/update/delete a policy set
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/policysets/baseline" $AUTH | jq .
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/policysets/baseline" \
  -H 'Content-Type: application/json' $AUTH -d '{"name":"baseline","description":"Updated","rules":[]}' -i
curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/policysets/baseline" $AUTH -i

# Cluster curated PolicySet catalog
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/policysets/catalog" $AUTH | jq .
```

Tenant PolicySets are persisted in the KubeNova store (Postgres in production, in-memory in tests) and keyed by tenant UID and policy set name. The cluster-wide catalog served by `/policysets/catalog` is data-backed: by default it is loaded from `docs/catalog/policysets.json`, and can be extended by editing that file without changing the manager binary.

When a PolicySet includes `rules` of kind `vela.trait` or `vela.policy` and is attached to a tenant/project via `attachedTo`, KubeNova will materialize those rules into Vela traits and policies on `:deploy`:

- `kind: "vela.trait"` → the `spec` map is appended to the traits passed to Vela `SetTraits`.
- `kind: "vela.policy"` → the `spec` map is appended to the policies passed to Vela `SetPolicies`.

This lets you define reusable rollout/autoscaling/health PolicySets and attach them to many projects without changing app manifests.

Built-in catalog PolicySets (PaaS/CaaS oriented):

- `baseline` — health-check policy for apps (HTTP probe on `/healthz` every 10s).
- `gold-tier` — autoscaling + rollout defaults suitable for “gold” apps (min/max replicas and rollout batches).
- `baseline-security` — default network and image hygiene (default-deny style networking, allowed registries).
- `gold-observability` — logging + metrics traits turned on by default.
- `bluegreen-rollout` — blue/green rollout strategy for safer production releases.

Example: attach a `gold-tier` plan to a tenant/project

```bash
cat << 'EOF' > policyset-gold-tier.json
{
  "name": "gold-tier",
  "attachedTo": [
    { "tenant": "acme", "project": "web" }
  ],
  "rules": [
    {
      "kind": "vela.trait",
      "spec": {
        "type": "scaler",
        "properties": { "min": 3, "max": 10 }
      }
    },
    {
      "kind": "vela.policy",
      "spec": {
        "type": "rollout",
        "properties": { "batchPartition": 1, "maxUnavailable": 1 }
      }
    }
  ]
}
EOF

curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/policysets" \
  -H 'Content-Type: application/json' $AUTH \
  --data-binary @policyset-gold-tier.json
```

On the next deploy of an app in the `web` project under the `acme` tenant:

- The scaler trait and rollout policy from `gold-tier` will be sent to Vela via `SetTraits` / `SetPolicies`.
- Any additional PolicySets attached to the same tenant or project (e.g., `baseline-security`, `gold-observability`) will be applied in the same way, giving you PaaS/CaaS-style “plans” without changing application manifests.

### End-to-end example: baseline health checks

Assuming you already have `CLUSTER_ID`, `TENANT_ID`, `PROJECT_ID`, and `APP_ID`:

```bash
# Attach a baseline health-check PolicySet to tenant/project
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/policysets" \
  -H 'Content-Type: application/json' $AUTH \
  -d '{
    "name": "baseline",
    "attachedTo": [
      { "tenant": "acme", "project": "web" }
    ],
    "rules": [
      {
        "kind": "vela.policy",
        "spec": {
          "type": "health",
          "properties": { "probe": "http", "path": "/healthz", "intervalSeconds": 10 }
        }
      }
    ]
  }'

# Deploy the app (PolicySet traits/policies are applied first)
curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:deploy" \
  $AUTH -i

# Inspect Vela-backed app status
curl -sS \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/status" \
  $AUTH | jq .
```

::: details Example response — app status (baseline)
```json
{ "phase": "Running" }
```
:::

### End-to-end example: gold-tier rollout & autoscaling

```bash
# Attach gold-tier PolicySet (from catalog) to tenant/project
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/policysets" \
  -H 'Content-Type: application/json' $AUTH \
  --data-binary @policyset-gold-tier.json

# Trigger a new deploy for the app
curl -sS -X POST \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:deploy" \
  $AUTH -i

# Fetch app status after rollout starts
curl -sS \
  "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/status" \
  $AUTH | jq .
```

::: details Example response — app status (gold-tier)
```json
{ "phase": "Running" }
```
:::

### Plans (quotas + PolicySets)

For convenience, KubeNova can treat a “plan” as a bundle of:

- Tenant-level quotas (CPU, memory, pods).
- A set of PolicySets that should be attached to the tenant/projects.

The catalog of plans lives in `docs/catalog/plans.json` and currently includes:

- `baseline` plan
  - `tenantQuotas`: `cpu: 2`, `memory: 4Gi`, `pods: 50`.
  - `policysets`: `["baseline"]`.
- `gold` plan
  - `tenantQuotas`: `cpu: 6`, `memory: 10Gi`, `pods: 200`.
  - `policysets`: `["baseline","baseline-security","gold-tier","gold-observability","bluegreen-rollout"]`.

You can apply a plan to a tenant using existing APIs:

```bash
# Example: apply the gold plan to tenant acme/web

# 1) Set tenant quotas from the plan
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/quotas" \
  -H 'Content-Type: application/json' $AUTH \
  -d '{"cpu":"6","memory":"10Gi"}'

# 2) Optionally cap pods at the plan level
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/limits" \
  -H 'Content-Type: application/json' $AUTH \
  -d '{"pods":"200"}'

# 3) Attach the plan’s PolicySets to the tenant/project (see examples above)
#    e.g. create baseline, baseline-security, gold-tier, gold-observability, bluegreen-rollout
```

This gives you a simple “plans” abstraction for CaaS/PaaS:

- Pick a plan (`baseline` or `gold`) for each tenant.
- Apply quotas once, and manage behavior via the associated PolicySets that are applied automatically on deploy.

## 7) Catalog

```bash
curl -sS "$BASE/api/v1/catalog/components" $AUTH | jq .
curl -sS "$BASE/api/v1/catalog/traits" $AUTH | jq .
curl -sS "$BASE/api/v1/catalog/workflows" $AUTH | jq .
```

::: details Example response — components
```json
[{ "name": "web", "type": "component", "description": "Web service" }]
```
:::

::: details Example response — traits
```json
[{ "name": "scaler", "type": "trait", "description": "Scale deployments" }]
```
:::

::: details Example response — workflows
```json
[{ "name": "rollout", "type": "workflow", "description": "Rolling updates" }]
```
:::

## 8) Usage & kubeconfig

```bash
curl -sS "$BASE/api/v1/tenants/$TENANT_ID/usage?range=24h" $AUTH | jq .
curl -sS "$BASE/api/v1/projects/$PROJECT_ID/usage?range=24h" $AUTH | jq .
curl -sS -X POST "$BASE/api/v1/tenants/$TENANT_ID/kubeconfig" $AUTH | jq .
```

::: details Example response — tenant usage
```json
{ "window": "24h", "cpu": "2", "memory": "4Gi", "pods": 12 }
```
:::
::: tip Usage data
Tenant and project usage endpoints aggregate `cpu`, `memory`, and `pods` from Kubernetes `ResourceQuota` objects in the target cluster when a kubeconfig is registered. In development or when quotas are not available, they return example stub values to keep tests and scripts working.
:::

::: details Example response — tenant kubeconfig
```json
{
  "kubeconfig": "YXBpVmVyc2lvbjogdjEKY2x1c3RlcnM6IFtdCmNvbnRleHRzOiBbXQp1c2Vyczo gW10K",
  "expiresAt": "2025-01-01T13:00:00Z"
}
```
:::

## 9) Cleanup

```bash
curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_ID" $AUTH -i
```


## 10) Force-clear finalizers for any Terminating namespaces
```
# 0) Keep core namespaces (don’t try to delete kube-node-lease/default/kube-*)
CORE_NS='^(kube-system|kube-public|kube-node-lease|default|local-path-storage|metallb-system)$'

# 1) Remove broken webhooks (capsule / cert-manager / kyverno / vela)
for t in validatingwebhookconfigurations.admissionregistration.k8s.io \
         mutatingwebhookconfigurations.admissionregistration.k8s.io; do
  kubectl get "$t" -o name | grep -E 'capsule|projectcapsule|cert-manager|kyverno|vela' | xargs -r kubectl delete
done

# 2) Delete their CRDs (removes CR-level finalizers that can block NS deletion)
kubectl get crd -o name | grep -E 'capsule|projectcapsule|cert-manager|kyverno|vela|kubevela' | xargs -r kubectl delete

# 3) Delete non-core namespaces (don’t delete kube-* or default)
for ns in $(kubectl get ns --no-headers | awk '{print $1}' | grep -vE "$CORE_NS"); do
  echo "Deleting $ns"
  kubectl delete ns "$ns" --wait=false
done

# 4) Force-clear finalizers for any Terminating namespaces
for ns in $(kubectl get ns --no-headers | awk '$2=="Terminating"{print $1}'); do
  echo "Forcing finalize $ns"
  kubectl get ns "$ns" -o json \
  | jq '.spec.finalizers=[]' \
  | kubectl replace --raw "/api/v1/namespaces/$ns/finalize" -f -
done

# (If you don’t have jq)
# for ns in $(kubectl get ns --no-headers | awk '$2=="Terminating"{print $1}'); do
#   kubectl patch namespace "$ns" --type=json -p='[{"op":"remove","path":"/spec/finalizers"}]'
# done

```bash
for ns in $(kubectl get ns --no-headers | awk '{print $1}' | grep -vE '^(kube-system|kube-public|default|local-path-storage|metallb-system)$'); do   echo "Deleting namespace: $ns"; kubectl delete ns "$ns" --ignore-not-found; done
```

# 5) Sanity check
kubectl get ns
kubectl get pods -A
```
