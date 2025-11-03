#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../common.sh"

wait_api

echo "[e2e] Register cluster"
RESP=$(register_cluster "kind-e2e")
echo "$RESP"
CID=$(echo "$RESP" | jq -r .id)

echo "[e2e] Wait for Agent 2/2 and HPA present"
kubectl -n "$NAMESPACE" rollout status deploy/kubenova-agent --timeout=5m
kubectl -n "$NAMESPACE" get hpa kubenova-agent

echo "[e2e] Addons ready (best-effort)"
kubectl -n capsule-system rollout status deploy/capsule-controller-manager --timeout=10m || true
kubectl -n capsule-system rollout status deploy/capsule-proxy --timeout=5m || true
kubectl -n vela-system rollout status deploy/vela-core --timeout=10m || true

echo "[e2e] Validate cluster conditions"
curl -fsS "$API_URL/api/v1/clusters/${CID}" | tee artifacts/cluster.json
AGENT_READY=$(jq -r '.conditions[] | select(.type=="AgentReady").status' artifacts/cluster.json)
ADDONS_READY=$(jq -r '.conditions[] | select(.type=="AddonsReady").status' artifacts/cluster.json)
test "$AGENT_READY" = "True"
test "$ADDONS_READY" = "True"

mkdir -p artifacts
cat > artifacts/junit.xml << XML
<testsuite name="end_to_end" tests="3" failures="0">
  <testcase name="agent.ready"/>
  <testcase name="hpa.present"/>
  <testcase name="conditions.ok"/>
</testsuite>
XML

echo "[e2e] OK"
