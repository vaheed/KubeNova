# Troubleshooting

- Agent not Ready: `kubectl -n kubenova-system describe deploy agent` and check HPA.
- Capsule not installed: inspect job `kubenova-bootstrap` logs, ensure internet access to Helm repos.
- Vela CRDs missing: `kubectl get crd | grep -E 'capsule|oam'`.
- Manager install failure: ensure `AGENT_IMAGE` and network reachability from API to cluster.
- Fetch conditions: `curl /api/v1/clusters/{id}`.
- KubeNova agent logs: `kubectl -n kubenova-system logs -f deployment/agent`
- KubeNova job logs: `kubectl -n kubenova-system logs -f job/kubenova-bootstrap`
- Clear cluster: `for ns in $(kubectl get ns --no-headers | awk '{print $1}' | grep -vE '^(kube-system|kube-public|default|local-path-storage|metallb-system)$'); do   echo "Deleting namespace: $ns"; kubectl delete ns "$ns" --ignore-not-found; done`
- Clear cluster: `for ns in $(kubectl get ns --no-headers | awk '/Terminating/ {print $1}'); do   echo "Force deleting namespace: $ns";   kubectl get ns "$ns" -o json | sed 's/"kubernetes"//' | kubectl replace --raw "/api/v1/namespaces/$ns/finalize" -f -; done`
