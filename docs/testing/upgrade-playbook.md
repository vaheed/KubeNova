# Upgrade & Bootstrap Validation Playbook

This checklist exercises the manager/operator bootstrap and upgrade paths for cert-manager, Capsule, capsule-proxy (LoadBalancer), KubeVela, VelaUX, and FluxCD. It also covers certificate renewal.

## Prerequisites
- Docker + docker-compose, kind (provided via `docker-compose.dev.yml`).
- Built images:
  - Manager: `docker compose -f docker-compose.dev.yml build manager`
  - Operator: `docker build -t ghcr.io/vaheed/kubenova/operator:latest -f build/Dockerfile.operator .` and `kind load docker-image ... --name nova`
- Env overrides (optional): set in `.env`
  - `CERT_MANAGER_VERSION`, `CAPSULE_VERSION`, `CAPSULE_PROXY_VERSION`, `VELA_VERSION`, `FLUXCD_VERSION`
  - `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` if needed for Helm.

## Fresh start
```bash
docker compose -f docker-compose.dev.yml down
docker volume rm kubenova_dbdata
docker network create --subnet 10.250.0.0/16 kind-ipv4 || true
docker compose -f docker-compose.dev.yml up -d
```

## Register cluster
```bash
KUBE_B64=$(base64 -w0 kind/config)
curl -s -X POST http://localhost:8080/api/v1/clusters \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"dev-cluster\",\"datacenter\":\"dc1\",\"labels\":{\"env\":\"dev\"},\"kubeconfig\":\"$KUBE_B64\"}"
# wait for status=connected
curl -s http://localhost:8080/api/v1/clusters
```

## Verify installs
```bash
docker exec kubenova-kind-1 kubectl --kubeconfig /kubeconfig/config -n kubenova-system get deployments
docker exec kubenova-kind-1 kubectl --kubeconfig /kubeconfig/config -n kubenova-system get secrets | grep sh.helm
```
Expected deployments (Ready): `cert-manager`, `cert-manager-cainjector`, `cert-manager-webhook`, `capsule-controller-manager`, `capsule-proxy`, `vela-core`, `velaux`, `fluxcd`, `kubenova-operator`.

capsule-proxy must be `Service type=LoadBalancer`:
```bash
docker exec kubenova-kind-1 kubectl --kubeconfig /kubeconfig/config -n kubenova-system get svc capsule-proxy -o jsonpath='{.spec.type}'
```

## Upgrade trigger
- HTTP: `POST /api/v1/clusters/{clusterID}/bootstrap/{component}:upgrade` (component=`cert-manager|capsule|capsule-proxy|kubevela|velaux|fluxcd`)
- CLI (inside manager container): reuse the same endpoint with `curl`.
- Watch logs: `docker exec kubenova-kind-1 kubectl --kubeconfig /kubeconfig/config -n kubenova-system logs deploy/kubenova-operator -f --since=5m`

## Certificate renewal
```bash
curl -s -X POST http://localhost:8080/api/v1/clusters/{clusterID}/cert-manager:renew
docker exec kubenova-kind-1 kubectl --kubeconfig /kubeconfig/config -n kubenova-system rollout status deploy/cert-manager-webhook
```

## Rollback expectation
If a Helm upgrade fails, the operator attempts `helm rollback <release> <previous>` and reports the error in the operator logs. Cluster status remains `error` until a successful rerun.
