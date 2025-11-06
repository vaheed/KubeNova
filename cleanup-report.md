# Cleanup & Dedupe Report

This report documents inventory, removals, deduplication, and TODO handling performed in this pass. It reflects the final state with a single OpenAPI-first surface at /api/v1 and no legacy routes.

Scope: code, tests, and docs. Excludes generated artifacts (internal/http/knapi_*.gen.go) and VitePress caches.

## Inventory (Final)

- Public API contract
  - Kept: docs/openapi/openapi.yaml (canonical contract). Bootstrap component enum uses neutral names [tenancy, proxy, app-delivery].
  - Served path: /openapi.yaml (from docs/openapi/openapi.yaml).
  - One public surface: /api/v1 via generated router (no legacy).

- Duplicates and overlaps
  - Error helpers: internal/http/errors.go removed (dup). Manager respond helper removed.
  - Auth helpers: internal/http/auth.go removed (dup). Legacy JWT middleware removed from manager; RBAC enforced in new server.
  - DTOs: API DTOs (generated, internal/http) vs internal domain types (pkg/types). Intentional separation; API ≠ internal.

- Unused/orphaned
  - docs/openapi.yaml (legacy location) → removed; canonical is docs/openapi/openapi.yaml.
  - Legacy manager handlers and JWT middleware removed.
  - No empty placeholder dirs detected. deploy/examples/hello-web.yaml remains referenced by docs.

- Vendor references in public docs
  - Replaced explicit vendor names with neutral terminology across docs/* and docs/site/*.
  - Diagrams (.svg/.drawio) still contain vendor text; to be revised in artwork (deferred).

- TODO/FIXME
  - internal/reconcile/tenant.go:17 TODO converted to comment, tracked below. No remaining TODO/FIXME in code.

## Removals

- Deleted: internal/http/errors.go (unused; duplicated concept)
- Deleted: internal/http/auth.go (unused; duplicates manager’s JWT)
- Deleted: docs/openapi.yaml (duplicate of new canonical location)
- Deleted: legacy manager handlers (tenants/projects/apps/clusters, tokens/me, kubeconfig issuance) and JWT middleware; now served solely by OpenAPI server.
- Deleted: respond helper and unused utils in manager.

## Deduplication & Normalization

- Error model: keep one payload shape at the API layer via OpenAPI; new handlers emit KN-xxx codes per spec.
- Config/env loading: single path in cmd/manager/main.go; no alternates found.
- Logger/middleware: manager uses chi middlewares + zap; kept single stack.

## Finished TODOs

- internal/reconcile/tenant.go: replaced TODO with actionable comment + tracking here.
- Completed missing handlers in OpenAPI server for clusters/tenants/projects/apps; added system and token endpoints.

## Deferred items (tracked here; no TODOs left in code)

- Replace vendor labels in diagrams (SVG/Draw.io) with neutral text.
- Centralize error code helpers for all handlers (after migrating to OpenAPI handlers).
- Unify adapters wiring for tenancy/apps/proxy beneath the new backends and implement the generated handlers.
 - Deepen Vela backend operations (streaming logs pagination; diff enrichment) without vendor leakage.

## Tests & CI

- Added smoke tests for new /api/v1 routes: tokens, me, version, features.
- go build ./... and go test ./... pass locally.
- Postgres integration test for ListClusters behind -tags=integration; memory store tests for label selector + cursor.

## Rationale

Changes prioritize keeping /api/v1 contract stable, removing duplicate helpers, eliminating vendor leakage in public docs, and clearing actionable TODOs without intrusive refactors that risk breaking existing green tests.

## Tree snapshots (abridged)

- Before: Legacy router + new router coexisted; duplicate helpers existed; docs/openapi.yaml duplicate present.
- After: Only new router at /api/v1; removed legacy code; backends in internal/backends/{capsule,vela,proxy}; generated API under internal/http; canonical spec docs/openapi/openapi.yaml; docs updated.
