---
title: KubeNova API v1 – cURL Quickstart
---

# KubeNova API v1 – cURL Quickstart

This page walks through a complete end‑to‑end flow using `curl`, from getting an access token to deploying a simple app. Every step shows:

- the exact command to run, and
- a short explanation of what it does.

All examples use the v1 HTTP API defined in `docs/openapi/openapi.yaml` (v0.0.2) and implemented by the manager. Only implemented endpoints are shown here; for the full contract (including future additions) see `docs/openapi/openapi.yaml` and `docs/README.md`.

> You can copy‑paste these snippets into any POSIX shell (bash/zsh). On Windows, run them from WSL or adapt the syntax to PowerShell.

---

## 0) Prerequisites & base URL

- Base URL: `http://localhost:8080`
- Env vars used below:
  ```bash
  export KN_HOST=http://localhost:8080
  export KN_TOKEN="" # fill after step 1
  ```
- If auth is enabled, set `KUBENOVA_REQUIRE_AUTH=true` and `JWT_SIGNING_KEY` on the manager.

---

## 1) Get a token

```bash
curl -s -X POST "$KN_HOST/api/v1/tokens" \
  -H 'Content-Type: application/json' \
  -d '{"subject":"admin@example.com","roles":["admin"],"ttlMinutes":60}' \
  | tee /tmp/kn-token.json
export KN_TOKEN=$(jq -r '.token' /tmp/kn-token.json)
```

## 2) Health & version
```bash
curl -s "$KN_HOST/api/v1/healthz"
curl -s "$KN_HOST/api/v1/readyz"
curl -s "$KN_HOST/api/v1/version"
curl -s "$KN_HOST/api/v1/features"
```

## 3) Register a cluster (base64 kubeconfig)
```bash
CLUSTER_NAME="dev-cluster"
KUBECONFIG_FILE="kind/config"   # any kubeconfig with server / cert data
KUBECONFIG_B64=$(base64 -w0 "$KUBECONFIG_FILE")

curl -s -X POST "$KN_HOST/api/v1/clusters" \
  -H "Authorization: Bearer $KN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{
    \"name\": \"$CLUSTER_NAME\",
    \"datacenter\": \"dc1\",
    \"labels\": {\"env\": \"dev\"},
    \"kubeconfig\": \"$KUBECONFIG_B64\"
  }"
# capture the .id from the response into CLUSTER_ID
```
The kubeconfig must stay base64-encoded so the payload preserves the embedded
`clusters[].cluster.server`, certificates, and tokens. After the manager
decodes the kubeconfig it:

1. Provisions the `ghcr.io/vaheed/kubenova-operator` deployment into the
   registered cluster.
2. The operator immediately runs the bootstrap job that installs
   cert-manager, Capsule, Capsule Proxy, and KubeVela (using Helm),
   verifies they become Ready, and keeps the cluster status updated.

No additional manual install steps are required for those dependencies.

## 4) Create a tenant (owners, plan, quotas)
```bash
CLUSTER_ID="<cluster-id>"
curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants" \
  -H "Authorization: Bearer $KN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "acme",
    "owners": ["alice@example.com"],
    "plan": "gold",
    "labels": {"tier":"gold"},
    "quotas": {"cpu":"4"},
    "limits": {"pods":"20"}
  }'
# capture TENANT_ID from the response
```

## 5) Create a project
```bash
TENANT_ID="<tenant-id>"
curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects" \
  -H "Authorization: Bearer $KN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "payments",
    "description": "Handles payment flows"
  }'
# capture PROJECT_ID
```

## 6) Create and deploy an app
```bash
PROJECT_ID="<project-id>"
curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps" \
  -H "Authorization: Bearer $KN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "api",
    "description": "API service",
    "component": "web",
    "image": "ghcr.io/vaheed/kubenova-manager:latest",
    "spec": {
      "type":"webservice",
      "properties":{
        "image":"ghcr.io/vaheed/kubenova-manager:latest",
        "port":8080
      }
    },
    "traits": [
      {
        "type":"scaler",
        "properties":{"min":1,"max":3}
      }
    ]
  }'
# capture APP_ID

curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:deploy" \
  -H "Authorization: Bearer $KN_TOKEN"
```

## 7) Inspect + update the running app
```bash
curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/status" \
  -H "Authorization: Bearer $KN_TOKEN"

curl -s -X PUT "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID" \
  -H "Authorization: Bearer $KN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "description": "API service v2",
    "spec": {
      "type":"webservice",
      "properties":{
        "image":"ghcr.io/vaheed/api:v2",
        "port":8080
      }
    }
  }'

curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/revisions" \
  -H "Authorization: Bearer $KN_TOKEN"
```

## 8) Tenant summary and usage
```bash
curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/summary" \
  -H "Authorization: Bearer $KN_TOKEN"

curl -s "$KN_HOST/api/v1/tenants/$TENANT_ID/usage" \
  -H "Authorization: Bearer $KN_TOKEN"

curl -s "$KN_HOST/api/v1/projects/$PROJECT_ID/usage" \
  -H "Authorization: Bearer $KN_TOKEN"
```

## 9) Workflows and runs
```bash
RUN=$(curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/workflow/run" \
  -H "Authorization: Bearer $KN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"inputs":{"action":"smoke-test"}}')
RUN_ID=$(echo "$RUN" | jq -r '.id')

curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/workflow/runs" \
  -H "Authorization: Bearer $KN_TOKEN"

curl -s "$KN_HOST/api/v1/apps/runs/$RUN_ID" \
  -H "Authorization: Bearer $KN_TOKEN"
```

## 10) Suspend, resume, and delete the app
```bash
curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:suspend" \
  -H "Authorization: Bearer $KN_TOKEN"

curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/status" \
  -H "Authorization: Bearer $KN_TOKEN"

curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:resume" \
  -H "Authorization: Bearer $KN_TOKEN"

curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:delete" \
  -H "Authorization: Bearer $KN_TOKEN"

curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps" \
  -H "Authorization: Bearer $KN_TOKEN"
```

## 11) Clean up
```bash
curl -s -X DELETE "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID" \
  -H "Authorization: Bearer $KN_TOKEN"
```

---

## Where to go next

- Full OpenAPI contract (with request/response examples): `docs/openapi/openapi.yaml`.
- Endpoint matrix and background: `docs/README.md`.
- Docs site snippets in this file mirror the `internal/manager/e2e_test.go` scenario; keep both in sync when adding routes. 
