#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../common.sh"

echo "[validation] wait for API"
wait_api

echo "[validation] check /healthz and /readyz"
curl -fsS "$API_URL/healthz" >/dev/null
curl -fsS "$API_URL/readyz" >/dev/null

echo "[validation] check /metrics has heartbeat counter"
grep -q '^kubenova_heartbeat_total' <(curl -fsS "$API_URL/metrics")

echo "[validation] helm lint charts"
helm lint deploy/helm/manager
helm lint deploy/helm/kubenova-agent

mkdir -p artifacts
cat > artifacts/junit.xml << XML
<testsuite name="validation" tests="3" failures="0">
  <testcase name="healthz"/>
  <testcase name="readyz"/>
  <testcase name="metrics"/>
</testsuite>
XML

echo "[validation] OK"
