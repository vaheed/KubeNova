#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../common.sh"

wait_api

echo "[runtime] invalid JSON should 400"
code=$(curl -sS -o /dev/null -w "%{http_code}" -XPOST "$API_URL/api/v1/tenants" -H 'Content-Type: application/json' -d 'invalid')
test "$code" = "400"

echo "[runtime] not found cluster id should 404"
code=$(curl -sS -o /dev/null -w "%{http_code}" "$API_URL/api/v1/clusters/999999")
test "$code" = "404"

mkdir -p artifacts
cat > artifacts/junit.xml << XML
<testsuite name="runtime" tests="2" failures="0">
  <testcase name="invalid.json.400"/>
  <testcase name="cluster.404"/>
</testsuite>
XML

echo "[runtime] OK"
