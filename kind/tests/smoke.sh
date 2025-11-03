#!/usr/bin/env bash
set -euo pipefail
API_URL=${API_URL:-http://localhost:8080}

echo "[SMOKE] Register cluster via Manager API (${API_URL})"
KCFG=$(base64 -w0 ~/.kube/config 2>/dev/null || base64 ~/.kube/config)
RESP=$(curl -sS -XPOST ${API_URL}/api/v1/clusters -H 'Content-Type: application/json' -d '{"name":"kind-e2e","kubeconfig":"'"$KCFG"'"}')
echo "$RESP"
CID=$(echo "$RESP" | jq -r .id)

echo "[SMOKE] Wait for Agent 2/2 Ready"
kubectl -n kubenova-system rollout status deploy/kubenova-agent --timeout=5m
kubectl -n kubenova-system get hpa kubenova-agent

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

# Assert heartbeat metric appears, then resilience across API restart
echo "[SMOKE] Check heartbeat metric presence and resilience across API restart"
get_hb() { curl -sS ${API_URL}/metrics | awk '/^kubenova_heartbeat_total/ {print $2; exit}'; }

# Wait up to 120s for the first heartbeat to appear
HB_BEFORE=0
for i in {1..60}; do
  v=$(get_hb || echo "")
  if [[ -n "$v" ]]; then HB_BEFORE=$v; break; fi
  sleep 2
done
echo "heartbeat before=${HB_BEFORE}"
if [[ "$HB_BEFORE" == "0" || -z "$HB_BEFORE" ]]; then
  echo "heartbeat metric did not appear in time" >&2
  exit 1
fi

echo "[SMOKE] Stop Manager (docker compose)"
docker compose -f docker-compose.dev.yml stop manager
sleep 5
kubectl -n kubenova-system rollout status deploy/kubenova-agent --timeout=2m

echo "[SMOKE] Start Manager (docker compose)"
docker compose -f docker-compose.dev.yml start manager
for i in {1..30}; do curl -fsS ${API_URL}/healthz && break || sleep 2; done

echo "[SMOKE] Wait for heartbeat to increase"
for i in {1..30}; do HB_AFTER=$(get_hb || echo 0); echo "heartbeat now=${HB_AFTER}"; \
  awk "BEGIN{exit !(${HB_AFTER} > ${HB_BEFORE})}" && break || sleep 2; done
HB_AFTER=$(get_hb || echo 0)
awk "BEGIN{exit !(${HB_AFTER} > ${HB_BEFORE})}"

echo "[SMOKE] POST synthetic events and verify storage"
curl -sS -XPOST "${API_URL}/sync/events?cluster_id=${CID}" -H 'Content-Type: application/json' \
  -d '[{"type":"Info","resource":"agent","payload":{"m":"ok"}}]'
sleep 1
curl -sS ${API_URL}/api/v1/clusters/${CID}/events | jq 'length' | awk 'BEGIN{ok=0} {if ($1 >= 1) ok=1} END{exit ok?0:1}'

# DELETE flows for full CRUD coverage
echo "[SMOKE] DELETE flows"
curl -sS -XDELETE ${API_URL}/api/v1/apps/alice/demo/app | jq .
curl -sS ${API_URL}/api/v1/projects/alice/demo/apps | jq 'length' | awk 'BEGIN{ok=0} {if ($1==0) ok=1} END{exit ok?0:1}'

curl -sS -XDELETE ${API_URL}/api/v1/projects/alice/demo | jq .
curl -sS ${API_URL}/api/v1/tenants/alice/projects | jq 'length' | awk 'BEGIN{ok=0} {if ($1==0) ok=1} END{exit ok?0:1}'

curl -sS -XDELETE ${API_URL}/api/v1/tenants/alice | jq .
TEN_LIST=$(curl -sS ${API_URL}/api/v1/tenants | jq -r '.[].name')
if echo "$TEN_LIST" | grep -q '^alice$'; then echo "tenant still present" >&2; exit 1; fi

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
