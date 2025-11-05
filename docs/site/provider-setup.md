# Provider Setup (capsule-proxy DNS/TLS)

This guide shows how to expose capsule‑proxy on your own domain (e.g. `https://cproxy.example.com`) and configure KubeNova so issued kubeconfigs use that domain.

## 1) Expose capsule‑proxy

The Agent bootstrap installs capsule‑proxy with `service.type=LoadBalancer` by default. Get the external IP:

```bash
kubectl -n capsule-system get svc capsule-proxy -w
```

Create a DNS record:
- `A` (or `AAAA`) record `cproxy.example.com` → the LoadBalancer external IP (or hostname).

Optionally, use an Ingress instead of a LoadBalancer for TLS/HTTP routing control:

```bash
helm upgrade --install capsule-proxy oci://ghcr.io/projectcapsule/charts/capsule-proxy \
  --version 0.9.13 -n capsule-system \
  --set service.enabled=true \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=cproxy.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.tls[0].hosts[0]=cproxy.example.com \
  --set ingress.tls[0].secretName=cproxy-tls
```

If you use cert‑manager, add your ClusterIssuer annotation on the Ingress for automatic certificates.

## 2) Point KubeNova to your domain

KubeNova issues tenant kubeconfigs using `CAPSULE_PROXY_URL`. Set it to your public domain:

- docker‑compose:
```yaml
environment:
  - CAPSULE_PROXY_URL=https://cproxy.example.com
```

- Helm (manager chart):
```bash
helm upgrade --install kubenova-manager deploy/helm/manager -n kubenova-system \
  --set env.CAPSULE_PROXY_URL=https://cproxy.example.com
```

- Direct .env (for compose):
```
CAPSULE_PROXY_URL=https://cproxy.example.com
```

Redeploy the Manager to pick up the change.

## 3) Verify capsule‑proxy

```bash
curl -I https://cproxy.example.com
```
You should get a 200/404 from the proxy endpoint depending on path. Next, issue a kubeconfig via the API:

```bash
curl -sS -XPOST $API/api/v1/kubeconfig-grants \
  -H 'Content-Type: application/json' \
  -d '{"tenant":"acme","role":"tenant-admin"}' | jq -r .kubeconfig > acme.kubeconfig

grep server: acme.kubeconfig
# Expect: server: https://cproxy.example.com
```

## 4) Tenant experience (hide Capsule)

KubeNova installs a discovery allow‑list (ClusterRole/Binding) so tenant users do not see `capsule.clastix.io` resources when running `kubectl api-resources`.

Tenants can interact via:
- KubeNova APIs (recommended)
- Issued kubeconfig to your `CAPSULE_PROXY_URL` domain (no direct Capsule exposure required)

## 5) Notes

- On bare‑metal clusters, ensure MetalLB (or equivalent) is configured for `LoadBalancer` services.
- The Agent bootstrap pins versions by env (`CAPSULE_VERSION`, `CAPSULE_PROXY_VERSION`, `VELA_CORE_VERSION`). Override as needed.
- For strict edge filtering, you can route capsule‑proxy through a gateway/ingress and block Capsule discovery paths explicitly; KubeNova’s RBAC already hides them at the API level.

