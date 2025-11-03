# E2E Test Suites (Parallel)

This folder contains parallelizable end-to-end test suites for KubeNova. Each suite is a shell script under `e2e/suites/` and can be run locally or by CI.

Environment per suite
- Creates a Kind cluster (`kubenova-e2e`) using `helm/kind-action` in CI or `make kind-up` locally.
- Builds Agent image and loads it into Kind.
- Starts Manager + Postgres via `docker-compose.dev.yml`.
- Exports artifacts on completion (compose logs, cluster dumps, JUnit-like summaries).

Suites
- validation.sh – /healthz, /readyz, /metrics checks + Helm lint.
- functional.sh – CRUD for Tenants, Projects, Apps, and KubeconfigGrants.
- load.sh – simple concurrent request generator for API responsiveness.
- runtime.sh – malformed requests, error code and message validation.
- pentest.sh – auth required; verifies 401/403 on protected routes (no secrets committed).
- end_to_end.sh – registers cluster; waits Agent 2/2 and verifies add-ons & conditions.
- integration.sh – Postgres-backed integration tests via testcontainers.
- user_functions.sh – common user flows end-to-end.
- conditions.sh – explicit checks for AgentReady/AddonsReady conditions.

Usage (local)
- Ensure Docker, Kind, Helm, jq, and curl are available.
- `docker compose -f docker-compose.dev.yml up -d --build`
- Run a suite: `bash e2e/suites/validation.sh` (sets `API_URL=http://localhost:8080`).

Artifacts
- Written to `artifacts/` by each suite: logs, descriptions, event dumps, and a `junit.xml` snippet.
