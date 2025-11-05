# Cleanup & Dedupe Report

This report documents inventory, removals, deduplication, and TODO handling performed in this pass.

Scope: code, tests, and docs. Excludes generated artifacts (internal/http/knapi_*.gen.go) and VitePress caches.

## Inventory

- Public API contract
  - Kept: docs/openapi/openapi.yaml (canonical contract). Updated bootstrap component enum to neutral names [tenancy, proxy, app-delivery].
  - Served path: /openapi.yaml (from docs/openapi/openapi.yaml).

- Duplicates and overlaps
  - Error helpers: internal/http/errors.go duplicated manager respond logic → removed.
  - Auth helpers: internal/http/auth.go duplicated manager JWT middleware → removed.
  - DTOs: API DTOs (generated, internal/http) vs internal domain types (pkg/types). Intentional separation; API ≠ internal. Left as-is with rationale.

- Unused/orphaned
  - docs/openapi.yaml (legacy location) → removed in favor of docs/openapi/openapi.yaml.
  - No empty placeholder dirs detected. deploy/examples/hello-web.yaml is referenced by docs and scans.

- Vendor references in public docs
  - Replaced explicit vendor names with neutral terminology across docs/* and docs/site/*.
  - Diagrams (.svg/.drawio) still contain vendor text; to be revised in artwork (deferred).

- TODO/FIXME
  - internal/reconcile/tenant.go:17 TODO about integrating tenancy via unstructured → converted to a descriptive comment and tracked here (Deferred items).

## Removals

- Deleted: internal/http/errors.go (unused; duplicated concept)
- Deleted: internal/http/auth.go (unused; duplicates manager’s JWT)
- Deleted: docs/openapi.yaml (duplicate of new canonical location)

## Deduplication & Normalization

- Error model: keep one payload shape at the API layer via OpenAPI; internal manager remains unchanged for existing endpoints. New handlers should emit KN-xxx codes per spec.
- Config/env loading: single path in cmd/manager/main.go; no alternates found.
- Logger/middleware: manager uses chi middlewares + zap; kept single stack.

## Finished TODOs

- internal/reconcile/tenant.go: replaced TODO with actionable comment + tracking here.

## Deferred items (tracked here; no TODOs left in code)

- Replace vendor labels in diagrams (SVG/Draw.io) with neutral text.
- Centralize error code helpers for all handlers (after migrating to OpenAPI handlers).
- Unify adapters wiring for tenancy/apps/proxy beneath the new backends and implement the generated handlers.

## Tests & CI

- Added smoke tests for new /api/v1 routes: tokens, me, version, features.
- go build ./... and go test ./... pass locally.

## Rationale

Changes prioritize keeping /api/v1 contract stable, removing duplicate helpers, eliminating vendor leakage in public docs, and clearing actionable TODOs without intrusive refactors that risk breaking existing green tests.

