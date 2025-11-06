```bash
# KubeNova API v1 — Step-by-Step (curl only)
#
# How to use this guide
# - Copy the whole block, paste into a terminal, and adjust variables at top
# - Every section shows the endpoint, required/optional params, and common options
# - IDs in path params are UUIDv4 (lowercase). Names are only used in bodies and filters.
# - All commands are idempotent where possible; re-running is safe

export BASE=${BASE:-http://localhost:8080}
# export KN_TOKEN="<jwt>"
AUTH=${KN_TOKEN:+-H "Authorization: Bearer $KN_TOKEN"}

# ----------------------------------------------------------------------------
# Access & System

# Issue a token (admin role for demo)
curl -sS -X POST "$BASE/api/v1/tokens" -H 'Content-Type: application/json' \
  -d '{"subject":"demo","roles":["admin"],"ttlSeconds":3600}'
# Example output:
# { "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...", "expiresAt": "2025-01-01T12:00:00Z" }

# Who am I?
curl -sS "$BASE/api/v1/me" $AUTH
# Example output:
# { "subject": "demo", "roles": ["admin"] }

# Version and Features
curl -sS "$BASE/api/v1/version" $AUTH
curl -sS "$BASE/api/v1/features" $AUTH

# Health
curl -sS "$BASE/healthz"
curl -sS "$BASE/readyz"

# ----------------------------------------------------------------------------
# Clusters

export CLUSTER_NAME=${CLUSTER_NAME:-dev}
export KUBE_B64=$(base64 < ~/.kube/config | tr -d '\n')

# Register cluster
curl -sS -X POST "$BASE/api/v1/clusters" -H 'Content-Type: application/json' $AUTH \
  -d '{"name":"'$CLUSTER_NAME'","kubeconfig":"'$KUBE_B64'","labels":{"env":"dev"}}'
# Example output:
# { "name": "dev", "labels": {"env":"dev"}, "createdAt": "2025-01-01T00:00:00Z" }

# Resolve the cluster UUID by listing and filtering by name
CLUSTER_ID=$(curl -sS "$BASE/api/v1/clusters?limit=200" $AUTH | \
  jq -r '.[] | select(.name=="'$CLUSTER_NAME'") | .uid')
echo "CLUSTER_ID=$CLUSTER_ID"

# List clusters (with label filter)
curl -sS "$BASE/api/v1/clusters?limit=50&labelSelector=env%3Ddev" $AUTH -i
# Example output (body):
# [ { "uid": "2f1e4c8a-8f9a-4b1e-9d92-1b2c3d4e5f60", "name": "dev", "labels": {"env":"dev"}, "createdAt": "..." } ]

# Get cluster (path {c} is UUID)
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID" $AUTH | jq .
# Example output includes conditions when available
# { "name": "dev", "conditions": [ {"type":"AgentReady","status":"True"} ] }

# Capabilities
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/capabilities" $AUTH | jq .
# Example output: { "tenancy": true, "vela": true, "proxy": true }

# Bootstrap components
# - POST /clusters/{c}/bootstrap/{component}
# - {component} ∈ {tenancy, proxy, app-delivery}
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/bootstrap/tenancy" $AUTH -i

# ----------------------------------------------------------------------------
# Tenants (paths use tenant UID)

export TENANT_NAME=${TENANT_NAME:-acme}

# Create tenant
TENANT_JSON=$(curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants" \
  -H 'Content-Type: application/json' $AUTH \
  -d '{"name":"'$TENANT_NAME'","owners":["owner@example.com"],"labels":{"team":"platform"}}')
echo "$TENANT_JSON" | jq .
TENANT_ID=$(echo "$TENANT_JSON" | jq -r .uid)
echo "TENANT_ID=$TENANT_ID"
# Example output: { "uid":"3a7f5d62-...", "name":"acme", ... }

# List tenants
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants" $AUTH | jq .

# Get tenant by UID
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID" $AUTH | jq .

# Replace owners
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/owners" \
  -H 'Content-Type: application/json' $AUTH -d '{"owners":["alice@example.com","ops@example.com"]}' -i

# Set quotas
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/quotas" \
  -H 'Content-Type: application/json' $AUTH -d '{"cpu":"4","memory":"8Gi"}' -i

# Set limits
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/limits" \
  -H 'Content-Type: application/json' $AUTH -d '{"pods":"50"}' -i

# Set default network policies
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/network-policies" \
  -H 'Content-Type: application/json' $AUTH -d '{"defaultDeny":true}' -i

# Tenant summary (example)
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/summary" $AUTH | jq .

# ----------------------------------------------------------------------------
# Projects (paths use project UID)

export PROJECT_NAME=${PROJECT_NAME:-web}

# Create project
PROJECT_JSON=$(curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects" \
  -H 'Content-Type: application/json' $AUTH -d '{"name":"'$PROJECT_NAME'"}')
echo "$PROJECT_JSON" | jq .
PROJECT_ID=$(echo "$PROJECT_JSON" | jq -r .uid)
echo "PROJECT_ID=$PROJECT_ID"

# List projects
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects" $AUTH | jq .

# Get project
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID" $AUTH | jq .

# Update project labels/annotations
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID" \
  -H 'Content-Type: application/json' $AUTH -d '{"labels":{"tier":"gold"}}' -i

# Set project access
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/access" \
  -H 'Content-Type: application/json' $AUTH -d '{"members":[{"subject":"dev@example.com","role":"projectDev"}]}' -i

# (Optional) Scoped kubeconfig (if enabled server-side)
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/kubeconfig" $AUTH | jq .

# ----------------------------------------------------------------------------
# Apps (paths use app UID)

export APP_NAME=${APP_NAME:-hello}

# List apps
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps" $AUTH | jq .

# Create app
APP_JSON=$(curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps" \
  -H 'Content-Type: application/json' $AUTH -d '{"name":"'$APP_NAME'","components":[{"name":"web","image":"nginx:1.25"}]}')
echo "$APP_JSON" | jq .
APP_ID=$(echo "$APP_JSON" | jq -r .uid)
echo "APP_ID=$APP_ID"

# Get app
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID" $AUTH | jq .

# Deploy/Suspend/Resume
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:deploy" $AUTH -i
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:suspend" $AUTH -i
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID:resume" $AUTH -i

# Status/Revisions/Diff/Logs
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/status" $AUTH | jq .
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/revisions" $AUTH | jq .
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/diff/1/2" $AUTH | jq .
curl -sS "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/logs/web" $AUTH | jq .

# Traits/Policies/Image update
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/traits" \
  -H 'Content-Type: application/json' $AUTH -d '[{"type":"scaler","properties":{"replicas":2}}]' -i
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/policies" \
  -H 'Content-Type: application/json' $AUTH -d '[{"type":"rollout","properties":{"maxUnavailable":1}}]' -i
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID/image-update" \
  -H 'Content-Type: application/json' $AUTH -d '{"component":"web","image":"nginx","tag":"1.25.3"}' -i

# Delete app (Accepted)
curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_ID/tenants/$TENANT_ID/projects/$PROJECT_ID/apps/$APP_ID" $AUTH -i

# ----------------------------------------------------------------------------
# Catalog (read-only)

curl -sS "$BASE/api/v1/catalog/components" $AUTH | jq .
curl -sS "$BASE/api/v1/catalog/traits" $AUTH | jq .
curl -sS "$BASE/api/v1/catalog/workflows" $AUTH | jq .

# ----------------------------------------------------------------------------
# Usage & Kubeconfig

# Usage reports (if enabled)
curl -sS "$BASE/api/v1/tenants/$TENANT_ID/usage?range=24h" $AUTH | jq .

# Tenant-scoped kubeconfig (proxy)
curl -sS -X POST "$BASE/api/v1/tenants/$TENANT_ID/kubeconfig" $AUTH | jq .

# ----------------------------------------------------------------------------
# Cleanup

# Delete cluster (Requires UUID)
curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_ID" $AUTH -i
```

