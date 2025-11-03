# E2E Tests (Smoke + Kind)

This project ships two layers of E2E validation:

- Smoke (Kind): creates a Kind cluster, runs the Manager API + Postgres via docker compose, registers the cluster through the API, then verifies that the Manager installs the Agent and the Agent bootstraps add‑ons (Capsule, capsule‑proxy, KubeVela).
- Resilience: stops the API during the run to confirm the Agent continues to function in‑cluster; when the API is restored, verifies heartbeats and event sync resume.

## What’s executed
1. Start Kind (k8s) and docker compose (Manager API + Postgres).
2. Register the cluster:
   - `POST /api/v1/clusters` with the kubeconfig from Kind.
3. Wait for:
   - Agent `Deployment` ReadyReplicas >= 2 and HPA present.
   - Capsule, capsule‑proxy, and KubeVela controllers Ready.
   - `GET /api/v1/clusters/{id}` shows `AgentReady=True` and `AddonsReady=True`.
4. Exercise core user endpoints:
   - Tenants, Projects, Apps CRUD and `kubeconfig-grants`.
5. Resilience:
   - Stop Manager container with `docker compose stop manager`.
   - Ensure Agent remains healthy in the cluster.
   - Start Manager; verify `kubenova_heartbeat_total` increased and `/sync/events` ingestion persists.

## Commands
- CI: see `.github/workflows/ci.yml`, `e2e_kind` job.
- Local: the easiest way is to run the user-like flow script which creates Kind, starts Manager via compose, registers the cluster, installs the Agent and bootstraps add-ons:
  - `bash kind/scripts/run_user_flow.sh`
  - Or run individual suites from `e2e/suites/` with `API_URL=http://localhost:8080`.

## Extending E2E
- When adding APIs or flows, update the smoke to exercise them.
- Include resilience scenarios (API down/up) for control‑plane surfaces.
- Keep tests idempotent and with bounded timeouts.
