# Getting Started

- Provision a Kind cluster: `make kind-up`
- Deploy the Manager API: `make deploy-manager`
- Port-forward: `kubectl -n kubenova-system port-forward svc/kubenova-manager 8080:8080 &`
- Register the cluster:
```
curl -XPOST localhost:8080/api/v1/clusters -H 'Content-Type: application/json' \
  -d '{"name":"kind","kubeconfig":"'"$(base64 -w0 ~/.kube/config 2>/dev/null || base64 ~/.kube/config)"'"}'
```
  - If Manager runs via Docker Compose and your kubeconfig uses 127.0.0.1/localhost,
    rewrite the API server host to `host.docker.internal` so the Manager container
    can reach the Kind API server, then base64-encode:
```
kubectl config view --raw \
 | sed -E 's#server: https://(127\\.0\\.0\\.1|localhost)(:[0-9]+)#server: https://host.docker.internal\\2#g' \
 | base64 -w0 2>/dev/null || true
```
  - You can also wait for the Manager to be fully ready (DB connected) using:
```
curl -fS localhost:8080/wait?timeout=60
```
- Inspect status:
```
curl localhost:8080/api/v1/clusters/1 | jq
```
Expect `AgentReady=True` and `AddonsReady=True` once platform add-ons (tenancy, access proxy, and app delivery) are ready.
