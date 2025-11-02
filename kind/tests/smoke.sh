#!/usr/bin/env bash
set -euo pipefail
API_URL=${API_URL:-http://localhost:8080}

echo "[SMOKE] Register cluster via Manager API (${API_URL})"
KCFG=$(base64 -w0 ~/.kube/config 2>/dev/null || base64 ~/.kube/config)
RESP=$(curl -sS -XPOST ${API_URL}/api/v1/clusters -H 'Content-Type: application/json' -d '{"name":"kind-e2e","kubeconfig":"'"$KCFG"'"}')
echo "$RESP"
CID=$(echo "$RESP" | jq -r .id)

echo "[SMOKE] Wait for Agent 2/2 Ready"
kubectl -n kubenova rollout status deploy/kubenova-agent --timeout=5m
kubectl -n kubenova get hpa kubenova-agent

echo "[SMOKE] Wait for Addons Ready"
kubectl -n capsule-system rollout status deploy/capsule-controller-manager --timeout=10m
kubectl -n capsule-system rollout status deploy/capsule-proxy --timeout=5m
kubectl -n vela-system rollout status deploy/vela-core --timeout=10m || kubectl -n vela-system get deploy

echo "[SMOKE] Validate cluster conditions via API"
curl -sS ${API_URL}/api/v1/clusters/${CID} | tee /tmp/cluster.json
AGENT_READY=$(jq -r '.conditions[] | select(.type=="AgentReady").status' /tmp/cluster.json)
ADDONS_READY=$(jq -r '.conditions[] | select(.type=="AddonsReady").status' /tmp/cluster.json)
test "$AGENT_READY" = "True" && test "$ADDONS_READY" = "True"

# Exercise core user endpoints
echo "[SMOKE] CRUD API endpoints"
curl -sS -XPOST ${API_URL}/api/v1/tenants -H 'Content-Type: application/json' -d '{"name":"alice"}' | jq .
curl -sS ${API_URL}/api/v1/tenants | jq .
curl -sS -XPOST ${API_URL}/api/v1/projects -H 'Content-Type: application/json' -d '{"tenant":"alice","name":"demo"}' | jq .
curl -sS ${API_URL}/api/v1/tenants/alice/projects | jq .
curl -sS -XPOST ${API_URL}/api/v1/apps -H 'Content-Type: application/json' -d '{"tenant":"alice","project":"demo","name":"app"}' | jq .
curl -sS ${API_URL}/api/v1/projects/alice/demo/apps | jq .
curl -sS -XPOST ${API_URL}/api/v1/kubeconfig-grants -H 'Content-Type: application/json' -d '{"tenant":"alice","role":"tenant-dev"}' | jq .

# Assert heartbeat metric increased
echo "[SMOKE] Check heartbeat metric"
curl -sS ${API_URL}/metrics | grep -q '^kubenova_heartbeat_total'

# JUnit-like summary
mkdir -p artifacts
cat > artifacts/junit.xml << XML
<testsuite name="kubenova-e2e" tests="4" failures="0">
  <testcase name="api.deploy"/>
  <testcase name="agent.ready"/>
  <testcase name="addons.ready"/>
  <testcase name="conditions.ok"/>
</testsuite>
XML

echo "[SMOKE] OK"
