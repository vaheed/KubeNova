#!/usr/bin/env bash
set -euo pipefail

# Helper to spin up a kind cluster with MetalLB and export kubeconfig to kind/config.

CLUSTER_NAME="${CLUSTER_NAME:-nova}"
KIND_CONFIG="${KIND_CONFIG:-kind/kind-config.yaml}"
METALLB_CONFIG="${METALLB_CONFIG:-kind/metallb-config.yaml}"
NETWORK_NAME="${NETWORK_NAME:-kind-ipv4}"

command -v kind >/dev/null 2>&1 || { echo "[e2e] kind is required"; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo "[e2e] kubectl is required"; exit 1; }

if ! docker network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
  echo "[e2e] creating docker network $NETWORK_NAME ..."
  docker network create --subnet 10.250.0.0/16 "$NETWORK_NAME" >/dev/null
fi

if ! kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
  echo "[e2e] creating kind cluster $CLUSTER_NAME ..."
  KIND_EXPERIMENTAL_DOCKER_NETWORK="$NETWORK_NAME" kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG"
else
  echo "[e2e] cluster $CLUSTER_NAME already exists; skipping create"
fi

echo "[e2e] installing MetalLB..."
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.8/config/manifests/metallb-native.yaml >/dev/null
kubectl apply -f "$METALLB_CONFIG" >/dev/null
kubectl -n metallb-system wait --for=condition=Available deployment --all --timeout=180s || \
  echo "[e2e] warning: MetalLB deployments did not all report Available within timeout"

echo "[e2e] exporting kubeconfig to kind/config"
mkdir -p kind
kind get kubeconfig --name "$CLUSTER_NAME" > kind/config

if [[ "${LOAD_IMAGES:-0}" == "1" ]]; then
  echo "[e2e] loading manager/operator images into kind (tags: ${IMAGE_TAG:-v0.1.2})"
  TAG="${IMAGE_TAG:-v0.1.2}"
  kind load docker-image "ghcr.io/vaheed/kubenova/kubenova-manager:${TAG}" --name "$CLUSTER_NAME" || true
  kind load docker-image "ghcr.io/vaheed/kubenova/kubenova-operator:${TAG}" --name "$CLUSTER_NAME" || true
fi

if [[ "${REGISTER_WITH_MANAGER:-0}" == "1" ]]; then
  BASE_URL="${MANAGER_URL:-http://localhost:8080}"
  echo "[e2e] registering cluster with manager at ${BASE_URL}"
  KUBE_B64=$(base64 < kind/config | tr -d '\n')
  RESPONSE=$(curl -s -X POST "${BASE_URL}/api/v1/clusters" \
    -H "X-KN-Roles: admin" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"${CLUSTER_NAME}\",\"datacenter\":\"dev\",\"kubeconfig\":\"${KUBE_B64}\",\"labels\":{\"env\":\"dev\"}}")
  if command -v jq >/dev/null 2>&1; then
    echo "$RESPONSE" | jq .
  else
    echo "$RESPONSE"
  fi
fi

echo "[e2e] kind cluster ready. kubeconfig: kind/config"
