# KubeNova Installation Guide (Dev/Staging)

This guide installs the KubeNova CRDs and operator into a cluster. It assumes you have:
- `kubectl` and `kustomize` (or kubectl >=1.21 with `-k`)
- Cluster admin on the target cluster
- Manager endpoint reachable by the operator (set `MANAGER_URL`)

## 1) Apply CRDs
```
kubectl apply -k deploy/crds
```

## 2) Provide Helm charts to the operator
The operator bootstraps cert-manager, Capsule, Capsule Proxy, and KubeVela using Helm.
- Preferred: bake charts into the operator image at `/charts` (set `HELM_CHARTS_DIR=/charts`).
  ```
  # In the operator Dockerfile
  COPY deploy/charts /charts
  ENV HELM_CHARTS_DIR=/charts
  ```
- Remote fallback: set `HELM_USE_REMOTE=true` and the operator will install from upstream repos:
  - cert-manager: https://charts.jetstack.io (v1.14.4)
  - capsule: https://clastix.github.io/charts (0.5.0)
  - capsule-proxy: https://clastix.github.io/charts (0.3.1)
  - kubevela: https://kubevela.github.io/charts (1.9.11)

## 3) Configure and deploy the operator
Edit `deploy/operator/deployment.yaml` to set:
- `MANAGER_URL` to your manager service URL
- `HELM_CHARTS_DIR` if charts are mounted elsewhere (default `/charts`)
- image tag for the operator
 - `HELM_USE_REMOTE=true` if you prefer pulling charts from upstream repos at runtime

Then apply:
```
kubectl apply -k deploy/operator
```

## 4) Verify
```
kubectl -n kubenova-system get pods
kubectl get crd novatenants.kubenova.io
```
When a cluster is registered via the manager API (`/api/v1/clusters`), the manager will use the provided kubeconfig to install the operator, which in turn installs the dependencies and reconciles Nova CRDs into Capsule/Vela/Proxy resources.
