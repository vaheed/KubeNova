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
The operator bootstraps cert-manager, Capsule, Capsule Proxy, and KubeVela using Helm. Charts are provided under `deploy/charts/`. Build your operator image with these charts at `/charts`, or mount them and set `HELM_CHARTS_DIR`.

Example (container build):
```
# In your Dockerfile for the operator image
COPY deploy/charts /charts
```

## 3) Configure and deploy the operator
Edit `deploy/operator/deployment.yaml` to set:
- `MANAGER_URL` to your manager service URL
- `HELM_CHARTS_DIR` if charts are mounted elsewhere (default `/charts`)
- image tag for the operator

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
