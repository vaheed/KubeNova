# KubeNova Agents Guide (API‑First, Single‑Source Config)

Scope: Entire repository.

## Contract: API‑Only (/api/v1)
- The HTTP surface is defined by `docs/openapi/openapi.yaml` and the generated types/servers in `internal/http`.
- Any change to routes, models, status codes, or examples MUST update the OpenAPI and corresponding handlers atomically.
- Error responses always use structured bodies with `code` and `message` (KN‑* family):
  - KN‑400 bad request, KN‑401 unauthorized, KN‑403 forbidden, KN‑404 not found,
    KN‑409 conflict, KN‑422 unprocessable, KN‑500 internal error.
- Auth/RBAC: When `KUBENOVA_REQUIRE_AUTH=true`, handlers expect a Bearer JWT (HS256) signed with `JWT_SIGNING_KEY`.
  - Roles: `admin`, `ops`, `tenantOwner`, `projectDev`, `readOnly`.
  - Tests may use `X-KN-Roles` for role simulation.
- Rate limits: Keep handlers efficient and idempotent. Long‑running actions must return 202 Accepted and perform work asynchronously.

## Single‑Source Config
- Docker Compose reads env ONLY from `.env`. No inline defaults in compose files.
- Required env checks occur at startup; the Manager fails fast if `DATABASE_URL` is missing.
- `env.example` lists every variable with purpose, valid values, and examples.

## Pre‑Commit Checklist
- Build + static analysis
  - `go fmt ./...`, `go vet ./...`, `go build ./...` must pass.
  - Optionally run `staticcheck ./...` (don’t change generated code).
- Unit tests: `go test ./... -count=1` must pass.
- Security: `gosec ./...` has no HIGH; `trivy config --severity HIGH,CRITICAL --format table --exit-code 1 deploy` is clean.
- OpenAPI + docs
  - 100% of /api/v1 paths include request/response examples that match DTOs and tests.
  - Update `docs/site` examples when behavior changes.
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
- Unit: `make test-unit`
- Docs: `make docs-build`

By committing, an agent asserts the checklist has been followed.
