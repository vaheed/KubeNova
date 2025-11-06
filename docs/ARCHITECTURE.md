# Architecture

- **Manager/API (Compose)**: canonical intents and audit; exposes `/api/v1/*` and `/sync/*`.
- **Agent (in cluster)**: controller (applies desired state to platform components/K8s) + telemetry buffer (Redis) + sync to Manager.
- **Tenancy**: tenant isolation and access proxy for tenant access.
- **App Delivery**: application delivery (Applications, Workflows, Definitions).
