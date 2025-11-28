---
title: Local Development
---

# Local Development

## Toolchain
- Go 1.24+
- Docker + docker-compose (for Postgres + manager)
- Node 18+ (only for docs) and `npm install`
- kind + kubectl (for integration tests)

## Build & test
```bash
go fmt ./...
go vet ./...
go build ./...
go test ./... -count=1
# Optional (follow AGENTS guide): staticcheck ./..., gosec ./..., trivy config deploy
```

## Running the manager locally
```bash
cp env.example .env   # edit required values
go run ./cmd/manager
# or containerized:
docker compose -f docker-compose.dev.yml up -d db manager
```
- Auth required? Set `KUBENOVA_REQUIRE_AUTH=true` and `JWT_SIGNING_KEY` in `.env`.
- Manager refuses to start without `DATABASE_URL`.

## Docs (VitePress)
```bash
npm install
npm run docs:dev     # live reload at http://localhost:5173
npm run docs:build   # outputs to docs/.vitepress/dist
```

## Contract discipline
- API surface is defined in `docs/openapi/openapi.yaml` and mirrored in handlers under `internal/http` + `internal/manager`.
- Any route/model/status change must update the OpenAPI spec and handler/tests atomically.
- Structured errors only (`KN-*` codes).
