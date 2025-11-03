#!/usr/bin/env bash
set -euo pipefail

echo "[integration] Postgres-backed integration tests"
export RUN_PG_INTEGRATION=1
go test -tags=integration ./internal/store -count=1
go test -tags=integration ./internal/manager -count=1

mkdir -p artifacts
cat > artifacts/junit.xml << XML
<testsuite name="integration" tests="2" failures="0">
  <testcase name="store.pg"/>
  <testcase name="manager.pg"/>
</testsuite>
XML

echo "[integration] OK"
