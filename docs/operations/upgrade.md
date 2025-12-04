---
title: Upgrades & Validations
---

# Upgrades & Validations

Use this runbook to validate bootstrap and upgrades for cert-manager, Capsule, Capsule Proxy, KubeVela, and Velaux.

## Prerequisites
- Manager and operator images built/tagged (default `v0.1.3`).
- `docker-compose.dev.yml` for local manager + Postgres.
- `kind` cluster (see [kind E2E setup](kind-e2e.md)).
- Optional Helm overrides via `.env`: `CERT_MANAGER_VERSION`, `CAPSULE_VERSION`, `CAPSULE_PROXY_VERSION`, `VELA_VERSION`, `VELAUX_VERSION`, proxy settings.

## Upgrade to a new release (step-by-step)
1) Prep the version and config  
   - Pick the target tag (e.g., `v0.1.4`).  
   - Copy `env.example` to `.env` and bump `KUBENOVA_VERSION` (and `OPERATOR_IMAGE_TAG` if you override the operator chart). Keep `DATABASE_URL` and any auth settings intact.  
   - Read the release notes for breaking changes and Helm value updates.
2) Update code + docker compose (local)  
   ```bash
   git fetch --tags
   git checkout v0.1.4           # or main if testing tip
   docker compose -f docker-compose.dev.yml down
   docker compose -f docker-compose.dev.yml build manager   # rebuild with the new tag
   docker compose -f docker-compose.dev.yml up -d db manager
   curl -s http://localhost:8080/api/v1/version
   ```
3) Upgrade manager in-cluster (Helm)  
   ```bash
   helm upgrade --install kubenova-manager deploy/helm/manager \
     --namespace kubenova-system --create-namespace \
     --set image.tag=v0.1.4 \
     --set env.KUBENOVA_REQUIRE_AUTH=true \
     --set env.MANAGER_URL_PUBLIC=http://kubenova-manager.kubenova-system.svc.cluster.local:8080
   ```
   Adjust the public URL/auth flags for your environment or use a custom `values.yaml` pinned to the new tag.
4) Upgrade operator (Helm)  
   ```bash
   helm upgrade --install kubenova-operator deploy/helm/operator \
     --namespace kubenova-system --create-namespace \
     --set image.tag=v0.1.4 \
     --set manager.url=http://kubenova-manager.kubenova-system.svc.cluster.local:8080
   ```
   Optionally bump bootstrap component versions via `--set bootstrap.capsuleVersion=...` etc., or rely on baked defaults for the release.
5) Refresh cluster add-ons (post-upgrade)  
   - For each registered cluster, call `POST /api/v1/clusters/{clusterID}/refresh` to reinstall the release’s baked versions, or use `POST /api/v1/clusters/{clusterID}/bootstrap/{component}:upgrade` for targeted bumps (`cert-manager|capsule|capsule-proxy|kubevela|velaux`).

## Fresh start
```bash
docker compose -f docker-compose.dev.yml down
docker volume rm kubenova_dbdata || true
docker network create --subnet 10.250.0.0/16 kind-ipv4 || true
docker compose -f docker-compose.dev.yml up -d db manager
./kind/e2e.sh
```

## Register cluster
```bash
KUBE_B64=$(base64 < kind/config | tr -d '\n')
curl -s -X POST http://localhost:8080/api/v1/clusters \
  -H 'Content-Type: application/json' -H 'X-KN-Roles: admin' \
  -d "{\"name\":\"dev-cluster\",\"datacenter\":\"dc1\",\"labels\":{\"env\":\"dev\"},\"kubeconfig\":\"$KUBE_B64\"}"
curl -s http://localhost:8080/api/v1/clusters
```

## Verify installs
```bash
kubectl --kubeconfig kind/config -n cert-manager get deployments
kubectl --kubeconfig kind/config -n capsule-system get deployments
kubectl --kubeconfig kind/config -n vela-system get deployments
kubectl --kubeconfig kind/config -n kubenova-system get deployments
kubectl --kubeconfig kind/config -n kubenova-system get secrets | grep sh.helm
kubectl --kubeconfig kind/config -n capsule-system get svc capsule-proxy -o jsonpath='{.spec.type}'
```
Expect deployments Ready: `cert-manager`, `cert-manager-cainjector`, `cert-manager-webhook` (in `cert-manager`), `capsule-controller-manager`, `capsule-proxy` (in `capsule-system`), `vela-core`, `kubenova-operator` (in `kubenova-system`).

Velaux install (optional):
```bash
kubectl --kubeconfig kind/config -n kubenova-system exec deploy/kubenova-operator -- vela addon enable velaux
kubectl --kubeconfig kind/config -n vela-system get deployments
```

## Upgrade triggers
- HTTP: `POST /api/v1/clusters/{clusterID}/bootstrap/{component}:upgrade` where component is `cert-manager|capsule|capsule-proxy|kubevela|velaux`.
- HTTP: `POST /api/v1/clusters/{clusterID}/refresh` to rerun the full bootstrap/install set when you want to purge state or redeploy everything from scratch.
- Logs: `docker exec kubenova-kind-1 kubectl --kubeconfig /kubeconfig/config -n kubenova-system logs deploy/kubenova-operator -f --since=5m`.

## Certificate renewal
```bash
curl -s -X POST http://localhost:8080/api/v1/clusters/{clusterID}/cert-manager:renew
kubectl --kubeconfig kind/config -n cert-manager rollout status deploy/cert-manager-webhook
```

## Rollback expectation
If a Helm upgrade fails, the operator attempts `helm rollback <release> <previous>` and reports the error in logs. Cluster status remains `error` until a successful rerun.
