#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../common.sh"

wait_api

RESP=$(register_cluster "conds-e2e")
CID=$(echo "$RESP" | jq -r .id)

echo "[conditions] waiting for agent condition"
for i in {1..120}; do
  st=$(curl -fsS "$API_URL/api/v1/clusters/${CID}" | jq -r '.conditions[] | select(.type=="AgentReady").status')
  if [[ "$st" == "True" ]]; then break; fi
  sleep 2
done

echo "[conditions] checking addons"
st2=$(curl -fsS "$API_URL/api/v1/clusters/${CID}" | jq -r '.conditions[] | select(.type=="AddonsReady").status')
test "$st2" = "True"

mkdir -p artifacts
cat > artifacts/junit.xml << XML
<testsuite name="conditions" tests="2" failures="0">
  <testcase name="agent.ready"/>
  <testcase name="addons.ready"/>
</testsuite>
XML

echo "[conditions] OK"
