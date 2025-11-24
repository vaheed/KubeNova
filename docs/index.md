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

## 3) Register a cluster
```bash
curl -s -X POST "$KN_HOST/api/v1/clusters" \
  -H "Authorization: Bearer $KN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "dc-a",
    "datacenter": "eu-west",
    "labels": {"region":"eu-west"}
  }'
```

## 4) Create a tenant
```bash
CLUSTER_ID="<id-from-previous-step>"
curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants" \
  -H "Authorization: Bearer $KN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "acme",
    "owners": ["alice@example.com"],
    "plan": "baseline",
    "labels": {"tier":"gold"}
  }'
```

## 5) Create a project
```bash
TENANT_ID="<tenant-id>"
curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects" \
  -H "Authorization: Bearer $KN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "payments",
    "description": "Payments services"
  }'
```

## 6) Deploy an app (projects → apps → deploy)
```bash
PROJECT_ID="<project-id>"
curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps" \
  -H "Authorization: Bearer $KN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "web",
    "component": "webservice",
    "image": "ghcr.io/example/web:1.0.0",
    "spec": {"type":"webservice","port":8080}
  }'

curl -s -X POST "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/<app-id>:deploy" \
  -H "Authorization: Bearer $KN_TOKEN"
```

## 7) Inspect status & logs
```bash
APP_ID="<app-id>"
curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/status" \
  -H "Authorization: Bearer $KN_TOKEN"
curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/logs/main" \
  -H "Authorization: Bearer $KN_TOKEN"
```

## 8) Fetch kubeconfigs
```bash
curl -s -X POST "$KN_HOST/api/v1/tenants/$TENANT_ID/kubeconfig" \
  -H "Authorization: Bearer $KN_TOKEN"
curl -s "$KN_HOST/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/kubeconfig" \
  -H "Authorization: Bearer $KN_TOKEN"
```

---

## Where to go next

- Full OpenAPI contract and examples: `docs/openapi/openapi.yaml`.
- Endpoint matrix and overview: `docs/README.md`.
- Track future/experimental endpoints in the OpenAPI file; this quickstart only covers the stable v0.0.2 surface. 
