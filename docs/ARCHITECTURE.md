# Architecture

- **Manager/API (Compose)**: canonical intents and audit; exposes `/api/v1/*` and `/sync/*`.
- **Agent (in cluster)**: controller (applies desired state to Capsule/Vela/K8s) + telemetry buffer (Redis) + sync to Manager.
- **Capsule**: tenant isolation and capsule-proxy for tenant access.
- **KubeVela**: application delivery (OAM Applications, Workflows, Definitions).
