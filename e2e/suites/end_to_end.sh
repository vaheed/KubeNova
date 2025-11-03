#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../common.sh"

wait_api

echo "[e2e] Register cluster"
RESP=$(register_cluster "kind-e2e")
echo "$RESP"
CID=$(echo "$RESP" | jq -r .id)

echo "[e2e] Poll cluster conditions until ready"
for i in {1..120}; do
  json=$(curl -sS "$API_URL/api/v1/clusters/${CID}" || echo "")
  if [[ -z "$json" ]]; then sleep 2; continue; fi
  AGENT_READY=$(printf "%s" "$json" | jq -r '.conditions[] | select(.type=="AgentReady").status')
  ADDONS_READY=$(printf "%s" "$json" | jq -r '.conditions[] | select(.type=="AddonsReady").status')
  if [[ "$AGENT_READY" == "True" && "$ADDONS_READY" == "True" ]]; then break; fi
  sleep 2
done

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
  <testcase name="conditions.poll"/>
  <testcase name="conditions.ok"/>
</testsuite>
XML

echo "[e2e] OK"
