```bash
# KubeNova API v1 — Step-by-Step (curl only)
#
# How to use this guide
# - Copy the whole block, paste into a terminal, and adjust variables at top
# - Every section shows the endpoint, required/optional params, and common options
# - All commands are idempotent where possible; re-running is safe
#
# Auth
# - If auth is enabled on the server, set KN_TOKEN (JWT with roles)
# - Roles: admin, ops, tenantOwner, projectDev, readOnly
# - You can also pass X-KN-Roles for dev/testing

export BASE=${BASE:-http://localhost:8080}
# export KN_TOKEN="<jwt>"
AUTH=${KN_TOKEN:+-H "Authorization: Bearer $KN_TOKEN"}

# ----------------------------------------------------------------------------
# Access & System

# Issue a token (admin role for demo)
curl -sS -X POST "$BASE/api/v1/tokens" -H 'Content-Type: application/json' \
  -d '{"subject":"demo","roles":["admin"],"ttlSeconds":3600}'

# Who am I?
curl -sS "$BASE/api/v1/me" $AUTH

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

# List clusters
# - Query args:
#   - limit: 1..200 (default 50)
#   - cursor: opaque pagination cursor from X-Next-Cursor header
#   - labelSelector: simple k=v[,k2=v2]
curl -sS "$BASE/api/v1/clusters?limit=50&labelSelector=env%3Ddev" $AUTH -i

# Get cluster
# - Path param {c} is a UUID (lowercase)
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME" $AUTH -i

# Capabilities
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/capabilities" $AUTH

# Bootstrap components
# - POST /clusters/{c}/bootstrap/{component}
# - {component} ∈ {tenancy, proxy, app-delivery}
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/bootstrap/tenancy" $AUTH -i

# Delete cluster
# - Removes KubeNova registration and uninstalls agent resources from the target cluster
# - Requires the cluster UUID
curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_NAME" $AUTH -i

# ----------------------------------------------------------------------------
# Tenants

export TENANT=${TENANT:-acme}

# Create tenant
# - POST /clusters/{c}/tenants with body {name, owners?, labels?, annotations?}
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants" -H 'Content-Type: application/json' $AUTH \
  -d '{"name":"'$TENANT'","owners":["owner@example.com"],"labels":{"team":"platform"}}'

# List tenants (optionally filter by owner or labels in future revisions)
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants" $AUTH -i

# Get tenant
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT" $AUTH

# Replace owners
# - PUT /clusters/{c}/tenants/{t}/owners with body {owners:[..]}
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/owners" \
  -H 'Content-Type: application/json' $AUTH -d '{"owners":["alice@example.com","ops@example.com"]}' -i

# Set quotas
# - PUT /clusters/{c}/tenants/{t}/quotas with body { key: quantity }
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/quotas" \
  -H 'Content-Type: application/json' $AUTH -d '{"cpu":"4","memory":"8Gi"}' -i

# Set limits
# - PUT /clusters/{c}/tenants/{t}/limits with body { key: quantity }
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/limits" \
  -H 'Content-Type: application/json' $AUTH -d '{"pods":"50"}' -i

# Set default network policies
# - PUT /clusters/{c}/tenants/{t}/network-policies
# - Body shape is neutral; e.g., {"defaultDeny":true}
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/network-policies" \
  -H 'Content-Type: application/json' $AUTH -d '{"defaultDeny":true}' -i

# Tenant summary
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/summary" $AUTH

# ----------------------------------------------------------------------------
# Projects

export PROJECT=${PROJECT:-web}

# Create project
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects" \
  -H 'Content-Type: application/json' $AUTH -d '{"name":"'$PROJECT'"}'

# List projects
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects" $AUTH

# Get project
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT" $AUTH

# Update project labels/annotations
# - PUT /clusters/{c}/tenants/{t}/projects/{p}
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT" \
  -H 'Content-Type: application/json' $AUTH -d '{"labels":{"tier":"gold"}}'

# Set project access (members + roles)
# - PUT /clusters/{c}/tenants/{t}/projects/{p}/access
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/access" \
  -H 'Content-Type: application/json' $AUTH -d '{"members":[{"subject":"dev@example.com","role":"projectDev"}]}' -i

# (Optional) Scoped kubeconfig (if enabled server-side)
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/kubeconfig" $AUTH

# ----------------------------------------------------------------------------
# PolicySets

export POLICY=${POLICY:-baseline}

# Catalog
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/policysets/catalog" $AUTH

# List tenant PolicySets
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/policysets" $AUTH

# Create PolicySet
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/policysets" \
  -H 'Content-Type: application/json' $AUTH -d '{"name":"'$POLICY'","rules":[]}'

# Get/Update/Delete PolicySet
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/policysets/$POLICY" $AUTH
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/policysets/$POLICY" \
  -H 'Content-Type: application/json' $AUTH -d '{"rules":[]}' -i
curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/policysets/$POLICY" $AUTH -i

# ----------------------------------------------------------------------------
# Apps

export APP=${APP:-hello}

# List apps
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps" $AUTH

# Create app (neutral app model)
# - POST /clusters/{c}/tenants/{t}/projects/{p}/apps
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps" \
  -H 'Content-Type: application/json' $AUTH -d '{"name":"'$APP'","components":[{"name":"web","image":"nginx:1.25"}]}'

# Get app
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP" $AUTH

# Deploy/Suspend/Resume
# - POST .../apps/{a}:(deploy|suspend|resume)
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP:deploy" $AUTH -i
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP:suspend" $AUTH -i
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP:resume" $AUTH -i

# Status/Revisions/Diff/Logs
# - GET .../status
# - GET .../revisions
# - GET .../diff/{revA}/{revB}
# - GET .../logs/{component}?tail={n}&sinceSeconds={s}
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP/status" $AUTH
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP/revisions" $AUTH
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP/diff/1/2" $AUTH
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP/logs/web?tail=100&sinceSeconds=600" $AUTH

# Traits/Policies/Image update
# - PUT .../traits
# - PUT .../policies
# - POST .../image-update {component,image,tag}
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP/traits" \
  -H 'Content-Type: application/json' $AUTH -d '[{"type":"scaler","properties":{"replicas":2}}]' -i
curl -sS -X PUT "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP/policies" \
  -H 'Content-Type: application/json' $AUTH -d '[{"type":"rollout","properties":{"maxUnavailable":1}}]' -i
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP/image-update" \
  -H 'Content-Type: application/json' $AUTH -d '{"component":"web","image":"nginx","tag":"1.25.3"}' -i

# Workflow run/list
curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP/workflow/run" \
  -H 'Content-Type: application/json' $AUTH -d '{"steps":[]}' -i
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP/workflow/runs" $AUTH

# Delete app
curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT/apps/$APP" $AUTH -i

# ----------------------------------------------------------------------------
# Catalog (read-only)

curl -sS "$BASE/api/v1/catalog/components" $AUTH
curl -sS "$BASE/api/v1/catalog/traits" $AUTH
curl -sS "$BASE/api/v1/catalog/workflows" $AUTH

# ----------------------------------------------------------------------------
# Usage & Kubeconfig

# Usage reports
curl -sS "$BASE/api/v1/tenants/$TENANT/usage?range=7d" $AUTH
curl -sS "$BASE/api/v1/projects/$PROJECT/usage?range=7d" $AUTH

# Tenant-scoped kubeconfig (if enabled)
curl -sS -X POST "$BASE/api/v1/tenants/$TENANT/kubeconfig" $AUTH

# ----------------------------------------------------------------------------
# Cleanup (delete project, tenant)

curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT/projects/$PROJECT" $AUTH -i
curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT" $AUTH -i
```
