# KubeNova 0.1.1
Unified multi-datacenter CaaS/PaaS control plane. Manager runs centrally, clusters stay sovereign, operators connect outbound-only, and Capsule/KubeVela provide multi-tenancy and application delivery.

- API contract: `docs/openapi/openapi.yaml` (`/api/v1`, structured errors `KN-*`, optional JWT/RBAC)
- Charts: `deploy/helm/{manager,operator}` (version `0.1.1`)
- Docs: VitePress under `docs/` with architecture, operations, and quickstarts

## Quick start (docker-compose)
```bash
cp env.example .env                # edit DATABASE_URL, auth, telemetry
docker compose -f docker-compose.dev.yml up -d db manager
curl -s http://localhost:8080/api/v1/readyz
```
- Auth disabled by default (`KUBENOVA_REQUIRE_AUTH=false` in env.example); set `X-KN-Roles: admin` for admin routes. Enable auth + `JWT_SIGNING_KEY` for anything beyond dev.
- Rebuild/stop: `docker compose -f docker-compose.dev.yml build` / `down`.

## Live API walkthrough
Use the [API lifecycle walkthrough](docs/getting-started/api-playbook.md) for curl examples covering clusters → tenants → projects → apps → workflows → usage.

## kind-based E2E tests
```bash
docker network create --subnet 10.250.0.0/16 kind-ipv4 || true
./kind/e2e.sh        # creates cluster, installs MetalLB, writes kind/config
# start manager (see quick start), then:
RUN_LIVE_E2E=1 \
KUBENOVA_E2E_BASE_URL=http://localhost:8080 \
KUBENOVA_E2E_KUBECONFIG=kind/config \
go test -tags=integration ./internal/manager -run LiveAPIE2E -count=1 -v
```
Details in `docs/operations/kind-e2e.md`.

## Docs (VitePress)
```bash
npm install
npm run docs:dev     # live preview
npm run docs:build   # static site in docs/.vitepress/dist
```

## Configuration
- `env.example` is canonical; manager fails fast if `DATABASE_URL` is missing.
- Key env vars: `KUBENOVA_REQUIRE_AUTH`, `JWT_SIGNING_KEY`, `MANAGER_URL`, `PROXY_API_URL`, `OTEL_EXPORTER_OTLP_ENDPOINT`, component version toggles (`CERT_MANAGER_VERSION`, `CAPSULE_VERSION`, `CAPSULE_PROXY_VERSION`, `VELA_VERSION`, `FLUXCD_VERSION`, `VELAUX_VERSION`, `VELA_CLI_VERSION`), `BOOTSTRAP_*` switches.
- Compose and Helm intentionally avoid inline defaults—keep `.env` up to date.

## Roadmap & changes
- Roadmap: `ROADMAP.md` or `docs/roadmap.md` (0.1.1 baseline, live API E2E, docs refresh).
- Changelog: `CHANGELOG.md` (current release 0.1.1).
