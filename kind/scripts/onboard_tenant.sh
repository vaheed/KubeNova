#!/usr/bin/env bash
set -euo pipefail

# Tenant onboarding helper:
# - Creates a Capsule Tenant via KubeNova Manager (proxied to target cluster by cluster_id)
# - Issues a kubeconfig bound to CAPSULE_PROXY_URL
#
# Requirements: curl, jq
#
# Usage:
#   kind/scripts/onboard_tenant.sh \
#     -a http://localhost:8080 \
#     -c 1 \
#     -t acme \
#     [-r tenant-admin] \
#     [-o user1@example.com,user2@example.com] \
#     [-k ./acme.kubeconfig] \
#     [--token <jwt_token>]
#
# If --token (or KUBENOVA_TOKEN env) is provided and the manager requires auth,
# it is sent as a Bearer token.

API_URL=${API_URL:-http://localhost:8080}
CLUSTER_ID=""
TENANT=""
ROLE="tenant-admin"
OWNERS=""
KCONF_OUT=""
TOKEN="${KUBENOVA_TOKEN:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -a|--api)
      API_URL="$2"; shift 2;;
    -c|--cluster-id)
      CLUSTER_ID="$2"; shift 2;;
    -t|--tenant)
      TENANT="$2"; shift 2;;
    -r|--role)
      ROLE="$2"; shift 2;;
    -o|--owners)
      OWNERS="$2"; shift 2;;
    -k|--kubeconfig)
      KCONF_OUT="$2"; shift 2;;
    --token)
      TOKEN="$2"; shift 2;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0;;
    *) echo "unknown arg: $1"; exit 1;;
  esac
done

if [[ -z "$CLUSTER_ID" || -z "$TENANT" ]]; then
  echo "error: --cluster-id and --tenant are required" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then echo "curl missing" >&2; exit 1; fi
if ! command -v jq >/dev/null 2>&1; then echo "jq missing" >&2; exit 1; fi

HDRS=(-H 'Content-Type: application/json')
if [[ -n "$TOKEN" ]]; then HDRS+=(-H "Authorization: Bearer $TOKEN"); fi

# Build Capsule Tenant object
OWNERS_JSON="[]"
if [[ -n "$OWNERS" ]]; then
  IFS=',' read -r -a arr <<< "$OWNERS"
  tmp="["; sep=""
  for o in "${arr[@]}"; do tmp+="$sep{\"kind\":\"User\",\"name\":\"$o\"}"; sep=","; done
  tmp+="]"; OWNERS_JSON="$tmp"
fi

TENANT_BODY=$(cat <<JSON
{
  "apiVersion":"capsule.clastix.io/v1beta2",
  "kind":"Tenant",
  "metadata":{"name":"$TENANT"},
  "spec":{"owners": $OWNERS_JSON}
}
JSON
)

echo "[onboard] Creating Tenant '$TENANT' on cluster_id=$CLUSTER_ID via $API_URL"
curl -sS -f -XPOST "$API_URL/api/v1/tenants?cluster_id=$CLUSTER_ID" "${HDRS[@]}" -d "$TENANT_BODY" >/dev/null || true

echo "[onboard] Verifying Tenant exists"
LIST=$(curl -sS -f "$API_URL/api/v1/tenants?cluster_id=$CLUSTER_ID" "${HDRS[@]}")
echo "$LIST" | jq -e --arg n "$TENANT" '.items | any(.metadata.name == $n)' >/dev/null || {
  echo "error: tenant '$TENANT' not found after create" >&2; exit 1; }

echo "[onboard] Issuing kubeconfig for tenant '$TENANT' (role=$ROLE)"
KCONF_JSON=$(curl -sS -f -XPOST "$API_URL/api/v1/kubeconfig-grants" "${HDRS[@]}" \
  -d "{\"tenant\":\"$TENANT\",\"role\":\"$ROLE\"}")
KCONF=$(echo "$KCONF_JSON" | jq -r .kubeconfig)
if [[ -z "$KCONF_OUT" ]]; then KCONF_OUT="${TENANT}.kubeconfig"; fi
echo "$KCONF" > "$KCONF_OUT"
chmod 600 "$KCONF_OUT"

echo
echo "[onboard] Success. Next steps for the tenant user:"
echo "  export KUBECONFIG=$(pwd)/$KCONF_OUT"
echo "  kubectl get ns"
echo "  # Use your issued kubeconfig against the provider domain (CAPSULE_PROXY_URL)"

