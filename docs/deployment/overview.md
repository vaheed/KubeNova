---
title: Local & Helm Deployment
---

# Deployment Overview

KubeNova ships with docker-compose for local dev and Helm charts for cluster installs.

## Docker Compose (dev)
- Services: Postgres (`db`) and Manager (`manager`); host networking for easy kubeconfig access.
- Config: `.env` only (Compose does not embed defaults). Copy from `env.example`.
- Commands:
```bash
docker compose -f docker-compose.dev.yml up -d db manager   # start
docker compose -f docker-compose.dev.yml logs -f manager    # follow logs
docker compose -f docker-compose.dev.yml down               # stop
docker compose -f docker-compose.dev.yml build              # rebuild images
```
- To include the kind helper container: `docker compose -f docker-compose.dev.yml up -d kind` (requires `docker network create --subnet 10.250.0.0/16 kind-ipv4` first).

## Helm charts
- Charts live under `deploy/helm/{manager,operator}` with defaults set to `v0.1.3`.
- Add the chart repo (OCI):
```bash
helm registry login ghcr.io -u <user> -p <token>
helm pull oci://ghcr.io/vaheed/kubenova/charts/manager --version v0.1.3
helm pull oci://ghcr.io/vaheed/kubenova/charts/operator --version v0.1.3
```
- Install manager (example):
```bash
helm upgrade --install kubenova-manager deploy/helm/manager \
  -n kubenova-system --create-namespace \
  --set image.tag=v0.1.3 \
  --set env.DATABASE_URL=postgres://user:pass@db:5432/kubenova?sslmode=require \
  --set jwt.value="<strong-random-secret>"
```
- Install operator into a target cluster (charts are also baked into the manager image at `/charts/operator`):
```bash
helm upgrade --install kubenova-operator deploy/helm/operator \
  -n kubenova-system --create-namespace \
  --set image.tag=v0.1.3 \
  --set manager.url=http://kubenova-manager.kubenova-system.svc.cluster.local:8080
```

## Bootstrap/upgrade components
- The operator installs cert-manager, Capsule, Capsule Proxy, KubeVela, and Velaux by default.
- Version overrides: set `CERT_MANAGER_VERSION`, `CAPSULE_VERSION`, `CAPSULE_PROXY_VERSION`, `VELA_VERSION`, `VELAUX_VERSION`, `VELA_CLI_VERSION`.
- Toggle installs: `BOOTSTRAP_CERT_MANAGER`, `BOOTSTRAP_CAPSULE`, `BOOTSTRAP_CAPSULE_PROXY`, `BOOTSTRAP_KUBEVELA`, `BOOTSTRAP_VELAUX`.

## CRDs and manifests
- Nova CRDs live under `deploy/crds/`; apply with `kubectl apply -k deploy/crds`.
- Static operator manifests are under `deploy/operator/` for environments that prefer Kustomize over Helm.
