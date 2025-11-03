# E2E Tests (Smoke + Kind)

This project ships two layers of E2E validation:

- Smoke (Kind): creates a Kind cluster, runs the Manager API + Postgres via docker compose, registers the cluster through the API, then verifies that the Manager installs the Agent and the Agent bootstraps add‑ons (Capsule, capsule‑proxy, KubeVela).
- Resilience: stops the API during the run to confirm the Agent continues to function in‑cluster; when the API is restored, verifies heartbeats and event sync resume.

## What’s executed
1. Start Kind (k8s) and docker compose (Manager API + Postgres).
2. Register the cluster:
   - `POST /api/v1/clusters` with the kubeconfig from Kind.
   - Example (run on your host, after compose is up and Kind is ready):
```
curl -XPOST localhost:8080/api/v1/clusters -H 'Content-Type: application/json' \
  -d '{"name":"kind","kubeconfig":"'"$(base64 -w0 ~/.kube/config 2>/dev/null || base64 ~/.kube/config)"'"}'
```
   - If Manager runs in Docker and your kubeconfig points to 127.0.0.1/localhost,
     replace the server host with host.docker.internal before base64‑encoding so the
     Manager container can reach the Kind API server.
     For example: `kubectl config view --raw | sed -E 's#server: https://(127\.0\.0\.1|localhost)(:[0-9]+)#server: https://host.docker.internal\2#g' | base64 -w0`
3. Wait for:
   - Agent `Deployment` ReadyReplicas >= 2 and HPA present.
   - Capsule, capsule‑proxy, and KubeVela controllers Ready.
   - `GET /api/v1/clusters/{id}` shows `AgentReady=True` and `AddonsReady=True`.
   - Manually inspect at any time:
```
curl localhost:8080/api/v1/clusters/1 | jq
```
4. Exercise core user endpoints:
   - Tenants, Projects, Apps CRUD and `kubeconfig-grants`.
5. Resilience:
   - Stop Manager container with `docker compose stop manager`.
   - Ensure Agent remains healthy in the cluster.
   - Start Manager; verify `kubenova_heartbeat_total` increased and `/sync/events` ingestion persists.

## Commands
- Local (user‑like flow, recommended):
  - `bash kind/scripts/run_user_flow.sh`
- Local (manual):
  - Start Manager + Postgres: `docker compose -f docker-compose.dev.yml up -d --build`
  - Register the cluster using the curl command above.
  - Check conditions via `GET /api/v1/clusters/{id}`.
  - The Manager installs the Agent automatically; the Agent bootstraps Capsule, capsule‑proxy, and KubeVela.
- CI: see `.github/workflows/ci.yml` parallel E2E jobs.

## Extending E2E
- When adding APIs or flows, update the smoke to exercise them.
- Include resilience scenarios (API down/up) for control‑plane surfaces.
- Keep tests idempotent and with bounded timeouts.
