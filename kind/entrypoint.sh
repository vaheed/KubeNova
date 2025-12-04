#!/bin/sh
set -e

CLUSTER_NAME="${CLUSTER_NAME:-nova}"

echo "[kind] Using cluster name: $CLUSTER_NAME"

# Ensure docker socket works
if ! docker info >/dev/null 2>&1; then
  echo "[kind] ERROR: docker socket not working (check /var/run/docker.sock mount)"
  exit 1
fi

# ------------------------------------------------------------------------------
# Create cluster on our IPv4 network
# ------------------------------------------------------------------------------
if ! kind get clusters | grep -q "$CLUSTER_NAME"; then
  echo "[kind] Creating kind cluster on network kind-ipv4..."
  KIND_EXPERIMENTAL_DOCKER_NETWORK=kind-ipv4 \
    kind create cluster --name "$CLUSTER_NAME" --config /kind-config.yaml
else
  echo "[kind] Cluster $CLUSTER_NAME already exists, skipping create."
fi

# ------------------------------------------------------------------------------
# Fix kubeconfig to use control-plane IP instead of 127.0.0.1
# ------------------------------------------------------------------------------
echo "[kind] Patching kubeconfig to use control-plane IP..."
CONTROL_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' nova-control-plane)

if [ -z "$CONTROL_IP" ]; then
  echo "[kind] ERROR: could not find control-plane IP"
  exit 1
fi

mkdir -p /root/.kube
# kind already wrote /root/.kube/config, just rewrite server line
sed -i "s|server: https://127.0.0.1:[0-9]*|server: https://$CONTROL_IP:6443|" /root/.kube/config

# Export kubeconfig to shared volume for host
mkdir -p /kubeconfig
cp -f /root/.kube/config /kubeconfig/config
echo "[kind] Kubeconfig exported to /kubeconfig/config (server=$CONTROL_IP:6443)"

export KUBECONFIG=/root/.kube/config

# ------------------------------------------------------------------------------
# Wait for API to be ready
# ------------------------------------------------------------------------------
echo "[kind] Waiting for API server to be reachable..."
i=0
until kubectl get nodes >/dev/null 2>&1; do
  i=$((i+1))
  if [ "$i" -gt 150 ]; then   # 150 * 2s = 300s timeout
    echo "[kind] WARNING: API server did not become ready in time, continuing anyway"
    break
  fi
  sleep 2
done
echo "[kind] API wait loop finished."

# ------------------------------------------------------------------------------
# Install MetalLB (idempotent)
# ------------------------------------------------------------------------------
if ! kubectl get ns metallb-system >/dev/null 2>&1; then
  echo "[kind] Installing MetalLB..."
  kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.8/config/manifests/metallb-native.yaml
else
  echo "[kind] MetalLB already installed, skipping manifest apply."
fi

echo "[kind] Waiting for MetalLB deployments to be Available..."
kubectl -n metallb-system wait --for=condition=Available deployment --all --timeout=180s || \
  echo "[kind] WARNING: MetalLB deployments did not all report Available within timeout"

echo "[kind] Applying MetalLB IP pool config..."
kubectl apply -f /metallb-config.yaml

echo "[kind] Cluster + MetalLB ready. Keeping container alive..."
tail -f /dev/null
