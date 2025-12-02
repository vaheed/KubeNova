---
title: API Lifecycle Walkthrough
---

# API Lifecycle Walkthrough

Hands-on `curl` flow that mirrors the `/api/v1` contract (see `docs/openapi/openapi.yaml`). It assumes the manager runs locally on `http://localhost:8080` with auth disabled; add a `Authorization: Bearer <token>` header if auth is enabled.

Set helper variables:
```bash
export KN_HOST=http://localhost:8080
export KN_ROLES="X-KN-Roles: admin"
```

## 1) System checks
```bash
curl -s "$KN_HOST/api/v1/healthz"
curl -s "$KN_HOST/api/v1/readyz"
curl -s "$KN_HOST/api/v1/version"
curl -s "$KN_HOST/api/v1/features"
```

## 2) Register a cluster
```bash
KUBE_B64=$(base64 -w0 kind/config) # any kubeconfig works
CAPSULE_PROXY_ENDPOINT="https://proxy.dev.example.com"
CLUSTER=$(curl -s -X POST "$KN_HOST/api/v1/clusters" \
  -H "$KN_ROLES" -H 'Content-Type: application/json' \
  -d "{
    \"name\": \"dev-cluster\",
    \"datacenter\": \"dc1\",
    \"labels\": {\"env\": \"dev\"},
    \"kubeconfig\": \"$KUBE_B64\",
    \"capsuleProxyEndpoint\": \"$CAPSULE_PROXY_ENDPOINT\"
  }")
CLUSTER_ID=$(echo "$CLUSTER" | jq -r '.id')
```
The manager will install the operator (local Helm chart) into the cluster asynchronously and update status to `connected` when successful.

## 3) Create a tenant
```bash
TENANT=$(curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants" \
  -H "$KN_ROLES" -H 'Content-Type: application/json' \
  -d '{
    "name": "acme",
    "owners": ["alice@example.com"],
    "plan": "gold",
    "labels": {"tier":"gold"},
    "quotas": {"cpu":"4"},
    "limits": {"pods":"20"}
  }')
TENANT_ID=$(echo "$TENANT" | jq -r '.id')
```

Fetch tenant kubeconfigs (owner/read-only) once the operator reconciles:
```bash
curl -s "$KN_HOST/api/v1/tenants/$TENANT_ID/kubeconfig" -H "$KN_ROLES"
```
If kubeconfigs aren’t ready yet, the response falls back to Capsule Proxy URLs.

## 4) Create a project
```bash
PROJECT=$(curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects" \
  -H "$KN_ROLES" -H 'Content-Type: application/json' \
  -d '{
    "name": "payments",
    "description": "Handles payment flows"
  }')
PROJECT_ID=$(echo "$PROJECT" | jq -r '.id')
```

## 5) Create and deploy an app
```bash
APP=$(curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps" \
  -H "$KN_ROLES" -H 'Content-Type: application/json' \
  -d '{
    "name": "api",
    "description": "API service",
    "component": "web",
    "image": "ghcr.io/vaheed/kubenova/kubenova-manager:v0.1.3",
    "spec": {
      "type":"webservice",
      "properties":{
        "image":"ghcr.io/vaheed/kubenova/kubenova-manager:v0.1.3",
        "port":8080
      }
    },
    "traits": [
      {
        "type":"scaler",
        "properties":{"min":1,"max":3}
      }
    ]
  }')
APP_ID=$(echo "$APP" | jq -r '.id')

curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:deploy" \
  -H "$KN_ROLES"
```

## 6) Inspect + update
```bash
curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/status" -H "$KN_ROLES"
curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/revisions" -H "$KN_ROLES"

curl -s -X PUT "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID" \
  -H "$KN_ROLES" -H 'Content-Type: application/json' \
  -d '{
    "description": "API service v2",
    "spec": {
      "type":"webservice",
      "properties":{
        "image":"ghcr.io/vaheed/kubenova/kubenova-manager:v0.1.3",
        "port":8080
      }
    }
  }'
```

## 7) Workflows & lifecycle actions
```bash
RUN=$(curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/workflow/run" \
  -H "$KN_ROLES" -H 'Content-Type: application/json' \
  -d '{"inputs":{"action":"smoke-test"}}')
RUN_ID=$(echo "$RUN" | jq -r '.id')
curl -s "$KN_HOST/api/v1/apps/runs/$RUN_ID" -H "$KN_ROLES"

curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:suspend" -H "$KN_ROLES"
curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:resume" -H "$KN_ROLES"
curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:delete" -H "$KN_ROLES"
```

## 8) Usage and summaries
```bash
curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/summary" -H "$KN_ROLES"
curl -s "$KN_HOST/api/v1/tenants/$TENANT_ID/usage" -H "$KN_ROLES"
curl -s "$KN_HOST/api/v1/projects/$PROJECT_ID/usage" -H "$KN_ROLES"
```
