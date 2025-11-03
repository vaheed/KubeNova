# KubeNova Kind — Local Helpers

This folder provides minimal helpers to create a Kind cluster and deploy KubeNova charts manually. Automated end‑to‑end testing lives under `e2e/` and is executed by CI in parallel suites.

## Prerequisites
- Docker 20+
- kubectl
- kind v0.20+
- Helm 3.13+
- jq

## Quick Start (manual)

1) Create a Kind cluster:
```
make kind-up
```

2) Start Manager API + Postgres in Docker Compose:
```
docker compose -f docker-compose.dev.yml up -d --build
for i in {1..60}; do curl -fsS http://localhost:8080/healthz && break || sleep 2; done
```

3) Deploy KubeNova components into the cluster (optional — Manager normally installs Agent automatically when a cluster is registered):
```
bash kind/scripts/deploy_manager.sh
bash kind/scripts/deploy_agent.sh
```

4) Register your cluster via the Manager API and exercise flows or run the E2E suites from `e2e/suites/`.

## Files
- `kind-config.yaml` — cluster topology used by `make kind-up`.
- `scripts/deploy_manager.sh` — installs the Manager chart to `kubenova-system`.
- `scripts/deploy_agent.sh` — installs the Agent chart to `kubenova-system`.

For end‑to‑end tests, see `e2e/README.md` and the suite scripts under `e2e/suites/`.
