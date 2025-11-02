# Operations

- Scale Agent: Adjust HPA or deployment replicas in `internal/cluster/manifests` or helm values.
- Upgrade Agent: Set `env.AGENT_IMAGE` for the API helm chart and roll the Agent.
- Rollback: Reapply a prior Agent image; helm job installs add-ons idempotently.
- Health Checks: `/healthz` and `/readyz` on both Manager and Agent.
- Metrics: `/metrics` Prometheus endpoint on Manager; Controller-runtime metrics on Agent.
