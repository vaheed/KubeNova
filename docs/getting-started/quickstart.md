---
title: Quickstart
---

# Quickstart (docker-compose)

Run the manager and Postgres locally with Docker; the stack reads configuration from `.env` only.

## Prerequisites
- Docker + docker-compose
- Go 1.24+ (for local builds/tests)
- Node 18+ (for docs) — optional
- `kind` + `kubectl` if you plan to attach a real cluster later

## 1) Configure environment
- Copy `env.example` to `.env` and adjust values (Postgres DSN, auth, telemetry, component versions).
- `DATABASE_URL` and `JWT_SIGNING_KEY` (when auth is on) are required; the manager fails fast if they are missing.

## 2) Start the dev stack
```bash
docker compose -f docker-compose.dev.yml up -d db manager
```
- Compose uses host networking; ports: Postgres `5432`, manager API `8080`.
- Stop with `docker compose -f docker-compose.dev.yml down` and rebuild images with `docker compose -f docker-compose.dev.yml build`.

## 3) Health + auth
```bash
curl -s http://localhost:8080/api/v1/healthz
curl -s http://localhost:8080/api/v1/readyz
```
- With auth disabled (`KUBENOVA_REQUIRE_AUTH=false`), set `X-KN-Roles: admin` for admin-only calls.
- With auth enabled, mint a token:
```bash
curl -s -X POST http://localhost:8080/api/v1/tokens \
  -H 'Content-Type: application/json' \
  -d '{"subject":"admin@example.com","roles":["admin"],"ttlMinutes":60}' \
  | jq -r '.token' > /tmp/kubenova.token
```

## 4) Register a cluster (optional, for real E2E)
If you have a `kind` cluster (see [kind E2E setup](../operations/kind-e2e.md)):
```bash
KUBE_B64=$(base64 -w0 kind/config)
curl -s -X POST http://localhost:8080/api/v1/clusters \
  -H 'Content-Type: application/json' \
  -H 'X-KN-Roles: admin' \
  -d "{\"name\":\"dev-cluster\",\"datacenter\":\"dc1\",\"labels\":{\"env\":\"dev\"},\"kubeconfig\":\"$KUBE_B64\"}"
```
The manager uses Helm (bundled in the image) and the local operator chart to install the operator into the cluster.

## 5) VelaUX LoadBalancer
```bash
- kubectl -n vela-system patch svc velaux-server -p '{"spec": {"type": "LoadBalancer"}}'
```

## 6) Next steps
- Walk through the [API lifecycle](api-playbook.md) for tenants/projects/apps.
- See [Operations](../operations/kind-e2e.md) for running full integration tests with `kind`.
- Build the docs locally: `npm install && npm run docs:dev`.
