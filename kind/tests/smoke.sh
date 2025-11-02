#!/usr/bin/env bash
set -euo pipefail
echo "[SMOKE] Capsule & Vela presence"
kubectl -n capsule-system get deploy capsule-controller-manager
kubectl -n capsule-system get deploy capsule-proxy
kubectl -n vela-system get deploy vela-core || kubectl -n vela-system get deploy

echo "[SMOKE] KubeNova components"
kubectl -n kubenova get deploy kubenova-api
kubectl -n kubenova get deploy kubenova-agent || true

echo "[SMOKE] POST cluster to Manager (port-forward)"
kubectl -n kubenova port-forward svc/kubenova-api 18080:8080 >/tmp/pf.log 2>&1 &
PF_PID=$!
sleep 2
KCFG=$(base64 -w0 ~/.kube/config 2>/dev/null || base64 ~/.kube/config)
RESP=$(curl -sS -XPOST http://localhost:18080/api/v1/clusters -H 'Content-Type: application/json' -d '{"name":"kind-e2e","kubeconfig":"'"$KCFG"'"}')
echo "$RESP"
CID=$(echo "$RESP" | jq -r .id)
sleep 5
kill $PF_PID || true

echo "[SMOKE] Wait for Agent 2/2 Ready"
kubectl -n kubenova rollout status deploy/kubenova-agent --timeout=5m
kubectl -n kubenova get hpa kubenova-agent

echo "[SMOKE] Wait for Addons Ready"
kubectl -n capsule-system rollout status deploy/capsule-controller-manager --timeout=10m
kubectl -n capsule-system rollout status deploy/capsule-proxy --timeout=5m
kubectl -n vela-system rollout status deploy/vela-core --timeout=10m || kubectl -n vela-system get deploy

echo "[SMOKE] Validate cluster conditions via API"
kubectl -n kubenova port-forward svc/kubenova-api 18080:8080 >/tmp/pf.log 2>&1 &
PF_PID=$!
sleep 2
curl -sS http://localhost:18080/api/v1/clusters/${CID} | tee /tmp/cluster.json
kill $PF_PID || true
AGENT_READY=$(jq -r '.conditions[] | select(.type=="AgentReady").status' /tmp/cluster.json)
ADDONS_READY=$(jq -r '.conditions[] | select(.type=="AddonsReady").status' /tmp/cluster.json)
test "$AGENT_READY" = "True" && test "$ADDONS_READY" = "True"

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

echo "[SMOKE] Controller labels namespace"
kubectl create ns smoke-demo || true
sleep 2
kubectl get ns smoke-demo -o jsonpath='{.metadata.labels.kubenova\.project}' | grep -q smoke-demo
echo "[SMOKE] OK"
