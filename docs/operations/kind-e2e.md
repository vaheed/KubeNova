---
title: kind E2E Test Setup
---

# kind E2E Test Setup

Run live API/integration tests against a real Kubernetes cluster using `kind`. Assets live under `kind/` (config, MetalLB pool, Dockerfile, helper script).

## Prerequisites
- Docker + docker-compose
- kind v0.23+ and kubectl v1.30+
- Go 1.24+ (to run the integration test)
- Optional: build local `ghcr.io/vaheed/kubenova/*:v0.1.1` images if you want to `kind load` them (`LOAD_IMAGES=1`)

## 1) Create network + cluster
```bash
docker network create --subnet 10.250.0.0/16 kind-ipv4 || true
./kind/e2e.sh                             # creates cluster 'nova', installs MetalLB, writes kind/config
# Optional: LOAD_IMAGES=1 IMAGE_TAG=v0.1.1 ./kind/e2e.sh   # also loads local images into kind
```

## 2) Run the manager (Postgres + API)
```bash
cp env.example .env   # edit DATABASE_URL, JWT_SIGNING_KEY if auth is on
docker compose -f docker-compose.dev.yml up -d db manager
curl -s http://localhost:8080/api/v1/readyz
```

## 3) Register the kind cluster with the manager
```bash
KUBE_B64=$(base64 < kind/config | tr -d '\n')
curl -s -X POST http://localhost:8080/api/v1/clusters \
  -H 'Content-Type: application/json' -H 'X-KN-Roles: admin' \
  -d "{\"name\":\"kind-nova\",\"datacenter\":\"dev\",\"labels\":{\"env\":\"dev\"},\"kubeconfig\":\"$KUBE_B64\"}"
```
The manager uses the baked-in operator chart to install the operator; status progresses from `bootstrapping` to `connected`.

## 4) Run live integration test
Integration test lives in `internal/manager/live_api_e2e_test.go` (build tag `integration`). It uses the running manager and kind kubeconfig to exercise the HTTP API end-to-end.
```bash
RUN_LIVE_E2E=1 \
KUBENOVA_E2E_BASE_URL=http://localhost:8080 \
KUBENOVA_E2E_KUBECONFIG=kind/config \
go test -tags=integration ./internal/manager -run LiveAPIE2E -count=1 -v
```
- Auth enabled? Set `KUBENOVA_E2E_TOKEN=<bearer>`; otherwise the test sends `X-KN-Roles: admin`.
- The test creates temporary cluster/tenant/project/app records and cleans up the app; Postgres data remains for inspection.

## 5) Teardown
```bash
kind delete cluster --name nova
docker compose -f docker-compose.dev.yml down
```
