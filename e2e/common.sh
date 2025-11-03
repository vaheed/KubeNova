#!/usr/bin/env bash
set -euo pipefail

export API_URL=${API_URL:-http://localhost:8080}
export NAMESPACE=kubenova-system

wait_api() {
  local url=${1:-$API_URL}
  local tries=${2:-60}
  # Prefer blocking /wait if available
  if curl -fsS "$url/wait?timeout=60" >/dev/null 2>&1; then return 0; fi
  for i in $(seq 1 "$tries"); do
    if curl -fsS "$url/healthz" >/dev/null 2>&1; then return 0; fi
    sleep 2
  done
  echo "[common] API failed readiness at $url" >&2
  return 1
}

register_cluster() {
  local name=${1:-kind-e2e}
  local raw
  # Obtain current kubeconfig in raw form
  if raw=$(kubectl config view --raw 2>/dev/null); then :; else raw=""; fi
  if [[ -z "$raw" && -n "${KUBECONFIG:-}" && -f "$KUBECONFIG" ]]; then raw=$(cat "$KUBECONFIG"); fi
  if [[ -z "$raw" && -f "$HOME/.kube/config" ]]; then raw=$(cat "$HOME/.kube/config"); fi
  # Rewrite server host to host.docker.internal so Manager (in Docker) can reach Kind API on host
  raw=$(printf "%s" "$raw" | sed -E 's#server: https://(127\.0\.0\.1|localhost)(:[0-9]+)#server: https://host.docker.internal\2#g')
  local kcfg
  kcfg=$(printf "%s" "$raw" | base64 -w0 2>/dev/null || printf "%s" "$raw" | base64)
  # Retry registration to avoid transient resets during container warm-up
  for i in {1..30}; do
    resp=$(curl -sS -m 10 -w "\n%{http_code}" -XPOST "$API_URL/api/v1/clusters" -H 'Content-Type: application/json' \
      -d '{"name":"'"$name"'","kubeconfig":"'"$kcfg"'"}') || resp=""
    body=$(printf "%s" "$resp" | sed '$d')
    code=$(printf "%s" "$resp" | tail -n1)
    if [[ "$code" == "200" ]]; then
      printf "%s" "$body"
      return 0
    fi
    sleep 2
  done
  # last attempt without swallowing errors
  curl -fsS -XPOST "$API_URL/api/v1/clusters" -H 'Content-Type: application/json' \
    -d '{"name":"'"$name"'","kubeconfig":"'"$kcfg"'"}'
}

collect_artifacts() {
  mkdir -p artifacts
  docker compose -f docker-compose.dev.yml logs --no-color > artifacts/compose.log || true
  kubectl get events -A --sort-by=.lastTimestamp > artifacts/events-all.txt || true
  kubectl get crd > artifacts/crds.txt || true
  if kubectl get ns "$NAMESPACE" >/dev/null 2>&1; then
    kubectl -n "$NAMESPACE" get all -o wide > artifacts/kubenova.txt || true
    kubectl -n "$NAMESPACE" describe deploy kubenova-manager > artifacts/desc-manager.txt || true
    kubectl -n "$NAMESPACE" describe deploy kubenova-agent > artifacts/desc-agent.txt || true
    kubectl -n "$NAMESPACE" logs deploy/kubenova-manager --tail=1000 > artifacts/manager.log || true
    kubectl -n "$NAMESPACE" logs deploy/kubenova-agent --tail=1000 > artifacts/agent.log || true
  else
    echo "namespace $NAMESPACE not found; skipping ns-scoped logs" > artifacts/kubenova.txt
  fi
}
