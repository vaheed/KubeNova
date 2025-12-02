# KubeNova Agents Guide (API‑First, Single‑Source Config)

Scope: Entire repository.

## Contract: API‑Only (/api/v1)
- The HTTP surface is defined by `docs/openapi/openapi.yaml` (0.1.3) and the generated types/servers in `internal/http`.
- Any change to routes, models, status codes, or examples MUST update the OpenAPI and corresponding handlers atomically.
- Error responses always use structured bodies with `code` and `message` (KN‑* family):
  - KN‑400 bad request, KN‑401 unauthorized, KN‑403 forbidden, KN‑404 not found,
    KN‑409 conflict, KN‑422 unprocessable, KN‑500 internal error.
- Auth/RBAC: When `KUBENOVA_REQUIRE_AUTH=true`, handlers expect a Bearer JWT (HS256) signed with `JWT_SIGNING_KEY`.
  - Roles: `admin`, `ops`, `tenantOwner`, `projectDev`, `readOnly`.
  - Tests may use `X-KN-Roles` for role simulation.
- Rate limits: Keep handlers efficient and idempotent. Long‑running actions must return 202 Accepted and perform work asynchronously.

## Single‑Source Config
- Docker Compose reads env ONLY from `.env`. No inline defaults in compose files; keep `.env` in sync with `env.example`.
- Required env checks occur at startup; the Manager fails fast if `DATABASE_URL` is missing and `JWT_SIGNING_KEY` when auth is on.
- `env.example` is canonical; group vars by concern (database, auth, telemetry, bootstrap, proxies, test helpers).

## Pre‑Commit Checklist
- Build + static analysis
  - `go fmt ./...`, `go vet ./...`, `go build ./...` must pass.
  - Optionally run `staticcheck ./...` (don’t change generated code).
- Unit tests: `go test ./... -count=1` must pass.
- Security: `gosec ./...` has no HIGH; `trivy config --severity HIGH,CRITICAL --format table --exit-code 1 deploy` is clean.
- OpenAPI + docs
  - 100% of /api/v1 paths include request/response examples that match DTOs and tests; keep OpenAPI + handlers in lockstep.
  - VitePress docs under `docs/` stay current (quickstart, API walkthrough, operations, config, roadmap). Do not delete ADRs.
- Helm/deploy
  - `helm lint deploy/helm/*` for any chart touched.
- Hygiene
  - No TODO/FIXME left in code. No compiled artifacts.
  - Minimal RBAC; avoid broad cluster admin in charts/manifests.

## Do/Don’t
- Do keep DTOs/helpers canonical (one package per concern).
- Don’t add back‑compat shims or flags. Prefer explicit versioned changes to the OpenAPI if needed.
- Don’t introduce new top‑level folders without discussion.

## Helpful Commands
- Unit: `go test ./... -count=1`
- Docs: `npm install && npm run docs:dev` (or `docs:build`)
- Dev stack: `docker compose -f docker-compose.dev.yml up -d db manager`
- kind E2E: `./kind/e2e.sh` then `RUN_LIVE_E2E=1 go test -tags=integration ./internal/manager -run LiveAPIE2E -count=1`
- Upgrade/Bootstrap validation: see `docs/operations/upgrade.md` (register cluster, verify cert-manager/capsule/capsule-proxy/KubeVela/VelaUX, LB check for capsule-proxy, certificate renewal).

By committing, an agent asserts the checklist has been followed.
