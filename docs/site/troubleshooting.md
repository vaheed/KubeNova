# Troubleshooting

- Agent not Ready: `kubectl -n kubenova-system describe deploy agent` and check HPA.
- Capsule not installed: inspect job `kubenova-bootstrap` logs, ensure internet access to Helm repos.
- Vela CRDs missing: `kubectl get crd | grep -E 'capsule|oam'`.
- Manager install failure: ensure `AGENT_IMAGE` and network reachability from API to cluster.
- Fetch conditions: `curl /api/v1/clusters/{id}`.
