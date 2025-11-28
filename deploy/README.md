# deploy/

Helm charts and deployment assets.

- `helm/manager` – chart for the Manager API
- `helm/operator` – chart for the in-cluster Operator (installs Capsule, Capsule Proxy, KubeVela, FluxCD, Velaux)

Publishing
- CI publishes OCI charts to `oci://ghcr.io/<owner>/kubenova/charts/{manager,operator}`
  - lightweight tags: `dev` (develop) and `latest` (main)
  - release chart tags (e.g., `0.1.1`) mirror Chart.yaml; container images keep the leading `v` (e.g., `v0.1.1`)

Install (OCI)
```bash
helm registry login ghcr.io -u <user> -p <token>
helm pull oci://ghcr.io/<owner>/kubenova/charts/manager --version 0.1.1
helm pull oci://ghcr.io/<owner>/kubenova/charts/operator --version 0.1.1
```

Notes
- Charts include icons and support JWT secret injection (`deploy/helm/manager/templates/secret-jwt.yaml`).
- See per-chart READMEs and values for configuration: `deploy/helm/manager/values.yaml`, `deploy/helm/operator/values.yaml`.
