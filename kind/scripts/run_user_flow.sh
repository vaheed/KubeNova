#!/usr/bin/env bash
set -euo pipefail

KIND_CLUSTER=${KIND_CLUSTER:-kubenova-e2e}
API_URL=${API_URL:-http://localhost:8080}
AGENT_IMAGE=${AGENT_IMAGE:-ghcr.io/vaheed/kubenova/agent:dev}
NS=kubenova-system

echo "[kind] Ensure kind cluster: $KIND_CLUSTER"
if ! kind get clusters | grep -q "^${KIND_CLUSTER}$"; then
  kind create cluster --name "$KIND_CLUSTER" --config kind/kind-config.yaml
fi

echo "[kind] Build agent image and load into kind"
docker build -t "$AGENT_IMAGE" -f build/Dockerfile.agent .
kind load docker-image "$AGENT_IMAGE" --name "$KIND_CLUSTER"

echo "[compose] Start Manager + Postgres"
export MANAGER_URL_PUBLIC=${MANAGER_URL_PUBLIC:-$API_URL}
export KUBENOVA_REQUIRE_AUTH=${KUBENOVA_REQUIRE_AUTH:-false}
export AGENT_IMAGE
docker compose -f docker-compose.dev.yml up -d --build

echo "[manager] Wait for /healthz at $API_URL"
for i in {1..60}; do curl -fsS "$API_URL/healthz" && break || sleep 2; done

echo "[manager] Register the Kind cluster"
KCFG=$(kind get kubeconfig --name "$KIND_CLUSTER")
KCFG_B64=$(printf "%s" "$KCFG" | base64 -w0 2>/dev/null || printf "%s" "$KCFG" | base64)
RESP=$(curl -sS -XPOST "$API_URL/api/v1/clusters" -H 'Content-Type: application/json' \
  -d '{"name":"'"$KIND_CLUSTER"'","kubeconfig":"'"$KCFG_B64"'"}')
echo "$RESP"
CID=$(echo "$RESP" | jq -r .id)

echo "[agent] Wait for Agent 2/2 Ready and HPA"
kubectl -n "$NS" rollout status deploy/kubenova-agent --timeout=5m
kubectl -n "$NS" get hpa kubenova-agent

echo "[addons] Wait for Capsule/capsule-proxy/KubeVela (best-effort)"
kubectl -n capsule-system rollout status deploy/capsule-controller-manager --timeout=10m || true
kubectl -n capsule-system rollout status deploy/capsule-proxy --timeout=5m || true
kubectl -n vela-system rollout status deploy/vela-core --timeout=10m || true

echo "[conditions] Validate cluster conditions via Manager"
curl -sS "$API_URL/api/v1/clusters/${CID}" | tee /tmp/cluster.json
AGENT_READY=$(jq -r '.conditions[] | select(.type=="AgentReady").status' /tmp/cluster.json)
ADDONS_READY=$(jq -r '.conditions[] | select(.type=="AddonsReady").status' /tmp/cluster.json)
test "$AGENT_READY" = "True" && test "$ADDONS_READY" = "True"

echo "[done] KubeNova end-to-end flow completed successfully."
