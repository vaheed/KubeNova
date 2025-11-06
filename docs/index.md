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
