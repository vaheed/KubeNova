# Getting Started

- Provision a Kind cluster: `make kind-up`
- Deploy the Manager API: `make deploy-manager`
- Port-forward: `kubectl -n kubenova-system port-forward svc/kubenova-manager 8080:8080 &`
- Register the cluster:
```
curl -XPOST localhost:8080/api/v1/clusters -H 'Content-Type: application/json' \
  -d '{"name":"kind","kubeconfig":"'"$(base64 -w0 ~/.kube/config 2>/dev/null || base64 ~/.kube/config)"'"}'
```
- Inspect status:
```
curl localhost:8080/api/v1/clusters/1 | jq
```
Expect `AgentReady=True` and `AddonsReady=True` once Capsule, capsule-proxy, and KubeVela are ready.
