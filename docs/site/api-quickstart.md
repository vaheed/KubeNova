# API Quick Start (curl)

This page shows copy‑pasteable curl snippets to exercise KubeNova and the Capsule proxy from a clean environment.

## Prerequisites

- API base URL (Manager): `API=http://localhost:8080`
- `curl`, `jq`, and access token if `KUBENOVA_REQUIRE_AUTH=true` (export `KUBENOVA_TOKEN`).

```bash
export API=${API:-http://localhost:8080}
# Optional, when auth is enabled:
HDR_AUTH=()
if [[ -n "${KUBENOVA_TOKEN:-}" ]]; then HDR_AUTH=(-H "Authorization: Bearer $KUBENOVA_TOKEN"); fi
```

## 1) Register a cluster

```bash
KUBECONFIG_B64=$(base64 -w0 ~/.kube/config 2>/dev/null || base64 ~/.kube/config)
curl -sS -XPOST "$API/api/v1/clusters" \
  -H 'Content-Type: application/json' "${HDR_AUTH[@]}" \
  -d '{"name":"kind","kubeconfig":"'"$KUBECONFIG_B64"'"}' | jq -r .id | tee /tmp/cluster_id
CID=$(cat /tmp/cluster_id)
echo "Cluster ID: $CID"

# Prefer cluster UID if returned (more portable). Example jq:
CLUSTER_UID=$(curl -sS "$API/api/v1/clusters/$CID" "${HDR_AUTH[@]}" | jq -r .uid // empty)
if [[ -n "$CLUSTER_UID" && "$CLUSTER_UID" != "null" ]]; then echo "Cluster UID: $CLUSTER_UID"; fi
```

Wait a minute for the Agent to deploy and bootstrap add‑ons. Check conditions:

```bash
curl -sS "$API/api/v1/clusters/$CID" "${HDR_AUTH[@]}" | jq .conditions
```

## 2) Create a Capsule Tenant on the cluster

```bash
TENANT=acme
cat > /tmp/tenant.json <<JSON
{
  "apiVersion":"capsule.clastix.io/v1beta2",
  "kind":"Tenant",
  "metadata":{"name":"$TENANT"},
  "spec":{"owners": []}
}
JSON

curl -sS -XPOST "$API/api/v1/tenants?cluster_id=$CID" \
  -H 'Content-Type: application/json' "${HDR_AUTH[@]}" \
  -d @/tmp/tenant.json | jq -r .metadata.name

curl -sS "$API/api/v1/tenants?cluster_id=$CID" "${HDR_AUTH[@]}" | jq '.items[].metadata.name'

# Or use cluster_uid if present:
if [[ -n "$CLUSTER_UID" ]]; then
  curl -sS "$API/api/v1/tenants?cluster_uid=$CLUSTER_UID" "${HDR_AUTH[@]}" | jq '.items[].metadata.name'
fi

## (Alternative) Single-call bootstrap

curl -sS -XPOST "$API/api/v1/bootstrap-user" \
  -H 'Content-Type: application/json' "${HDR_AUTH[@]}" \
  -d '{"cluster_uid":"'$CLUSTER_UID'","tenant":"acme","owners":["user1@example.com"],"project":"web","role":"tenant-admin"}' | jq .
```

## 3) Issue a kubeconfig for the tenant (capsule‑proxy)

```bash
curl -sS -XPOST "$API/api/v1/kubeconfig-grants" \
  -H 'Content-Type: application/json' "${HDR_AUTH[@]}" \
  -d '{"tenant":"'$TENANT'","role":"tenant-admin"}' | jq -r .kubeconfig > ${TENANT}.kubeconfig
chmod 600 ${TENANT}.kubeconfig
echo "Kubeconfig saved: ${TENANT}.kubeconfig"
```

## 4) Capsule CRUD via KubeNova (cluster_id required)

Tenant Resource Quotas:
```bash
cat > /tmp/trq.json <<JSON
{
  "apiVersion":"capsule.clastix.io/v1beta2",
  "kind":"TenantResourceQuota",
  "metadata":{"name":"default"},
  "spec":{"hard": {"pods": "100"}}
}
JSON

curl -sS -XPOST "$API/api/v1/tenant-quotas?cluster_id=$CID" \
  -H 'Content-Type: application/json' "${HDR_AUTH[@]}" -d @/tmp/trq.json | jq .
curl -sS "$API/api/v1/tenant-quotas?cluster_id=$CID" "${HDR_AUTH[@]}" | jq .
```

Namespace Options:
```bash
cat > /tmp/nsopts.json <<JSON
{
  "apiVersion":"capsule.clastix.io/v1beta2",
  "kind":"NamespaceOptions",
  "metadata":{"name":"defaults"},
  "spec":{"allowUserDefinedLabels": true}
}
JSON

curl -sS -XPOST "$API/api/v1/namespace-options?cluster_id=$CID" \
  -H 'Content-Type: application/json' "${HDR_AUTH[@]}" -d @/tmp/nsopts.json | jq .
curl -sS "$API/api/v1/namespace-options?cluster_id=$CID" "${HDR_AUTH[@]}" | jq .
```

Capsule Configurations:
```bash
cat > /tmp/cfg.json <<JSON
{
  "apiVersion":"capsule.clastix.io/v1beta2",
  "kind":"CapsuleConfiguration",
  "metadata":{"name":"default"},
  "spec":{"userGroups": ["tenant-admins","tenant-maintainers"]}
}
JSON

curl -sS -XPOST "$API/api/v1/configurations?cluster_id=$CID" \
  -H 'Content-Type: application/json' "${HDR_AUTH[@]}" -d @/tmp/cfg.json | jq .
curl -sS "$API/api/v1/configurations?cluster_id=$CID" "${HDR_AUTH[@]}" | jq .
```

## 5) Manager‑scoped objects (optional)

Projects and apps are stored in KubeNova (not Capsule):
```bash
curl -sS -XPOST "$API/api/v1/projects" -H 'Content-Type: application/json' "${HDR_AUTH[@]}" \
  -d '{"tenant":"acme","name":"web","labels":{"env":"prod"}}' | jq .

curl -sS -XPOST "$API/api/v1/apps" -H 'Content-Type: application/json' "${HDR_AUTH[@]}" \
  -d '{"tenant":"acme","project":"web","name":"api","image":"ghcr.io/acme/api:1.0.0"}' | jq .
```

Tips
- If `KUBENOVA_REQUIRE_AUTH` is true, export `KUBENOVA_TOKEN` and the snippets add a Bearer header.
- The kubeconfig Manager issues uses `CAPSULE_PROXY_URL` so tenants only see your provider domain.
- Discovery RBAC omits Capsule API groups from tenant users by default.
