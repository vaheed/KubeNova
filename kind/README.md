# KubeNova Kind — Local Helpers

This folder provides minimal helpers to create a Kind cluster and deploy KubeNova charts manually. Automated end‑to‑end testing lives under `tests/e2e/` and is executed by CI through the Go-based suite.

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

2) Start Manager API + Postgres in Docker Compose (or use `make deploy-manager`):
```
docker compose -f docker-compose.dev.yml up -d --build
for i in {1..60}; do curl -fsS http://localhost:8080/healthz && break || sleep 2; done
```

3) Full user-like flow (register, auto-install Agent, bootstrap add-ons):
```
bash kind/scripts/run_user_flow.sh
```

This will:
- Build and load the Agent image into Kind.
- Start Manager + Postgres via Docker Compose.
- Register the Kind cluster with the Manager API.
- Wait for Agent 2/2 Ready and HPA; wait for add-ons readiness; assert cluster conditions.

4) Register your cluster via the Manager API manually or run `E2E_BUILD_IMAGES=true make test-e2e` for the automated flow.

## Files
- `kind-config.yaml` — cluster topology used by `make kind-up`.
- `scripts/run_user_flow.sh` — end‑to‑end user‑like flow (compose manager + register + auto‑install agent).

For end‑to‑end tests, see `docs/tests.md` and the Go scenarios under `tests/e2e/`.
