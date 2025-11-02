# KubeNova Kind E2E and Local Workflow

This folder contains everything to spin up a local Kubernetes cluster with Kind and run end‑to‑end (E2E) smoke tests against the KubeNova Manager API and the in‑cluster Agent.

The E2E flow mirrors production:
- Manager API + Postgres run outside the cluster (Docker Compose).
- A Kind cluster is created.
- The cluster is registered via the Manager API (`POST /api/v1/clusters`).
- Manager installs the Agent in the cluster; the Agent bootstraps add‑ons (Capsule, capsule‑proxy, KubeVela).
- The smoke script asserts readiness, exercises user endpoints, and checks resilience (API stop/start, heartbeats, event ingestion).

## Prerequisites
- Docker 20+
- kubectl
- kind v0.20+ (or `make kind-up` which installs via action in CI)
- Helm 3.13+
- jq

Optional:
- make (GNU)

## Quick Start

1) Create the Kind cluster:
```
make kind-up
```
This uses `kind/kind-config.yaml` (1 control plane + 2 workers by default).

2) Build the Agent and load into Kind (ensures the cluster can pull it without a registry):
```
docker build -t ghcr.io/vaheed/kubenova-agent:dev -f build/Dockerfile.agent .
kind load docker-image ghcr.io/vaheed/kubenova-agent:dev --name kubenova-e2e
```

3) Start Manager API + Postgres with Docker Compose:
```
docker compose -f docker-compose.dev.yml up -d --build
# Wait for API to be ready
for i in {1..60}; do curl -fsS http://localhost:8080/healthz && break || sleep 2; done
```

4) Run the smoke test:
```
API_URL=http://localhost:8080 bash kind/tests/smoke.sh
```
The script will:
- Register the Kind cluster via `POST /api/v1/clusters`.
- Wait for the Agent (2/2 Ready) and HPA.
- Wait for Capsule, capsule‑proxy and KubeVela controllers to be Ready.
- Validate `GET /api/v1/clusters/{id}` shows `AgentReady=True` and `AddonsReady=True`.
- Exercise CRUD APIs (tenants, projects, apps, kubeconfig‑grants).
- Stop the API container, confirm Agent is still healthy in‑cluster, start API again, and verify the heartbeat metric increases.
- POST synthetic events to `/sync/events` and verify they appear in `/api/v1/clusters/{id}/events`.

Artifacts (if you run in CI) include logs from compose services and cluster namespaces.

5) Cleanup:
```
# Remove add‑ons and agent
kubectl delete ns kubenova --ignore-not-found=true --wait=true || true
kubectl delete ns capsule-system --ignore-not-found=true || true
kubectl delete ns vela-system --ignore-not-found=true || true
# Stop Manager API + Postgres
docker compose -f docker-compose.dev.yml down -v
# Delete the Kind cluster
make down
```

## Files and Scripts

- `kind-config.yaml` – Kind cluster topology
- `tests/smoke.sh` – E2E smoke script (idempotent)
- `scripts/install_capsule.sh` – Installs Capsule via Helm (used by older flows; the Agent now bootstraps add‑ons itself in E2E)
- `scripts/install_capsule_proxy.sh` – Installs capsule‑proxy via Helm
- `scripts/install_kubevela.sh` – Installs KubeVela via Helm
- `scripts/deploy_kubenova_agent.sh` – Helm chart for Agent (for manual flows)
- `scripts/deploy_kubenova_api.sh` – Helm chart for API (for manual flows)

You can still use the scripts to install add‑ons manually for debugging, but the E2E smoke relies on the Agent to bootstrap them automatically.

## Environment Variables
- `API_URL` – Base URL to the Manager API (default `http://localhost:8080`).
- `MANAGER_URL_PUBLIC` – When running API with Compose/Helm, public URL for the Agent to call back (defaults set in CI to `http://localhost:8080`).
- `AGENT_IMAGE` – Image that Manager uses to install the Agent (`ghcr.io/vaheed/kubenova-agent:dev`).

## Tips
- If you prefer to test using the registry images only, push the Agent to GHCR and remove the `kind load` step, letting Kubernetes pull the image directly.
- If API fails to start, check compose logs:
```
docker compose -f docker-compose.dev.yml logs --no-color
```
- To rerun only the smoke without re‑creating the Kind cluster, keep the cluster and compose running and run:
```
API_URL=http://localhost:8080 bash kind/tests/smoke.sh
```

## CI Parity
The GitHub Actions `e2e_kind` job runs these steps automatically and always uploads the following logs as artifacts:
- Cluster‑wide pods and events, CRDs
- kubenova, capsule‑system, and vela‑system resources + logs
- Docker Compose service logs (API and DB)

This ensures failures are diagnosable and the flow remains representative of production.
