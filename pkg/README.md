# pkg/

Public packages intended for use by external code.

- `types/` – canonical API types (Cluster, Tenant, Project, App, PolicySet, KubeconfigGrant, Event, Condition)
- `client/` – simple typed HTTP client for the Manager API (auth header support)

Stability
- Backwards compatible types and client as much as possible. Breaking changes should be versioned.
