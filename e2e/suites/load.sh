#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../common.sh"

wait_api

echo "[load] concurrent GET /api/v1/tenants"
seq 1 50 | xargs -n1 -P10 -I{} curl -fsS "$API_URL/api/v1/tenants" -o /dev/null

echo "[load] concurrent POST /sync/metrics (heartbeat)"
seq 1 50 | xargs -n1 -P10 -I{} curl -fsS -XPOST "$API_URL/sync/metrics" -o /dev/null

mkdir -p artifacts
cat > artifacts/junit.xml << XML
<testsuite name="load" tests="2" failures="0">
  <testcase name="tenants.list.concurrent"/>
  <testcase name="heartbeat.concurrent"/>
</testsuite>
XML

echo "[load] OK"
