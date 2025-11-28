---
title: API & OpenAPI
---

# API & OpenAPI

- Contract: `docs/openapi/openapi.yaml` (v0.1.1). All routes, models, status codes, and examples must be updated in lockstep with handler changes.
- Base path: `/api/v1`.
- Error shape: structured body with `code` and `message` (`KN-400|401|403|404|409|422|500`).
- Auth/RBAC: when enabled, HS256 JWT with roles `admin`, `ops`, `tenantOwner`, `projectDev`, `readOnly`. Tests may use `X-KN-Roles` for simulation.
- Rate limits: long-running actions must return `202 Accepted` and execute asynchronously.

## Quick references
- Health/version: `/healthz`, `/readyz`, `/version`, `/features`
- Auth: `POST /tokens`, `GET /me`
- Clusters: CRUD + `/capabilities`, `/bootstrap/{component}`
- Tenants/projects/apps: nested routes for creation, updates, workflow runs, revisions, usage
- Usage: `/tenants/{id}/usage`, `/projects/{id}/usage`

See the [API lifecycle walkthrough](../getting-started/api-playbook.md) for concrete curl examples that mirror the spec and tests.
