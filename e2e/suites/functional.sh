#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../common.sh"

wait_api

echo "[functional] tenants/projects/apps CRUD"
curl -fsS -XPOST "$API_URL/api/v1/tenants" -H 'Content-Type: application/json' -d '{"name":"alice"}' >/dev/null
curl -fsS "$API_URL/api/v1/tenants" | jq -e 'map(.name) | index("alice") != null' >/dev/null
curl -fsS -XPOST "$API_URL/api/v1/projects" -H 'Content-Type: application/json' -d '{"tenant":"alice","name":"demo"}' >/dev/null
curl -fsS "$API_URL/api/v1/tenants/alice/projects" | jq -e 'map(.name) | index("demo") != null' >/dev/null
curl -fsS -XPOST "$API_URL/api/v1/apps" -H 'Content-Type: application/json' -d '{"tenant":"alice","project":"demo","name":"app"}' >/dev/null
curl -fsS "$API_URL/api/v1/projects/alice/demo/apps" | jq -e 'map(.name) | index("app") != null' >/dev/null
curl -fsS -XPOST "$API_URL/api/v1/kubeconfig-grants" -H 'Content-Type: application/json' -d '{"tenant":"alice","role":"tenant-dev"}' >/dev/null

echo "[functional] delete flows"
curl -fsS -XDELETE "$API_URL/api/v1/apps/alice/demo/app" >/dev/null
test "$(curl -fsS "$API_URL/api/v1/projects/alice/demo/apps" | jq 'length')" -eq 0
curl -fsS -XDELETE "$API_URL/api/v1/projects/alice/demo" >/dev/null
test "$(curl -fsS "$API_URL/api/v1/tenants/alice/projects" | jq 'length')" -eq 0
curl -fsS -XDELETE "$API_URL/api/v1/tenants/alice" >/dev/null
test "$(curl -fsS "$API_URL/api/v1/tenants" | jq -r '.[].name' | grep -c '^alice$' || true)" -eq 0

mkdir -p artifacts
cat > artifacts/junit.xml << XML
<testsuite name="functional" tests="4" failures="0">
  <testcase name="tenant.crud"/>
  <testcase name="project.crud"/>
  <testcase name="app.crud"/>
  <testcase name="grant.issue"/>
</testsuite>
XML

echo "[functional] OK"
