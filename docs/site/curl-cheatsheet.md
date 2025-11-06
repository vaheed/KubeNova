```bash
# KubeNova API v1 — curl-only quickstart (clusters + tenants)
# Copy/paste friendly. Set BASE and optional token, then run the steps.

# Base URL (adjust if running remotely)
export BASE=${BASE:-http://localhost:8080}

# Optional: bearer token. If set, it will be used automatically.
# export KN_TOKEN="<your-jwt>"
AUTH=${KN_TOKEN:+-H "Authorization: Bearer $KN_TOKEN"}

# -----------------------------------------------------------------------------
# 1) Add Cluster
#   Requires a kubeconfig, base64-encoded without newlines.

export CLUSTER_NAME=dev
export KUBE_B64=$(base64 < ~/.kube/config | tr -d '\n')

curl -sS -X POST "$BASE/api/v1/clusters" \
  -H "Content-Type: application/json" $AUTH \
  -d '{
        "name": "'$CLUSTER_NAME'",
        "kubeconfig": "'$KUBE_B64'",
        "labels": {"env":"dev"}
      }'

# 2) List Clusters
curl -sS "$BASE/api/v1/clusters" $AUTH

# 3) Get Cluster (readiness, capabilities available via separate route)
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME" $AUTH

# 4) Delete Cluster
#   Warning: this removes the cluster registration from KubeNova.
curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_NAME" $AUTH -i

# -----------------------------------------------------------------------------
# 5) Add Tenant (scoped to a cluster)

export CLUSTER_NAME=dev
export TENANT_NAME=acme

curl -sS -X POST "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants" \
  -H "Content-Type: application/json" $AUTH \
  -d '{
        "name": "'$TENANT_NAME'",
        "owners": ["owner@example.com"],
        "labels": {"team":"platform"}
      }'

# 6) List Tenants (in a cluster)
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants" $AUTH

# 7) Get Tenant
curl -sS "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT_NAME" $AUTH

# 8) Delete Tenant
curl -sS -X DELETE "$BASE/api/v1/clusters/$CLUSTER_NAME/tenants/$TENANT_NAME" $AUTH -i
```

