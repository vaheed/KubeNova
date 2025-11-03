#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../common.sh"

wait_api

echo "[user] typical user flows"
# create tenant -> project -> app, then list
curl -fsS -XPOST "$API_URL/api/v1/tenants" -H 'Content-Type: application/json' -d '{"name":"team-x"}' >/dev/null
curl -fsS -XPOST "$API_URL/api/v1/projects" -H 'Content-Type: application/json' -d '{"tenant":"team-x","name":"svc"}' >/dev/null
curl -fsS -XPOST "$API_URL/api/v1/apps" -H 'Content-Type: application/json' -d '{"tenant":"team-x","project":"svc","name":"hello"}' >/dev/null
curl -fsS "$API_URL/api/v1/projects/team-x/svc/apps" | jq -e 'map(.name) | index("hello") != null' >/dev/null

mkdir -p artifacts
cat > artifacts/junit.xml << XML
<testsuite name="user_functions" tests="1" failures="0">
  <testcase name="user.flow"/>
</testsuite>
XML

echo "[user] OK"
