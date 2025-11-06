# deploy/

Helm charts and deployment assets.

- `helm/manager` – chart for Manager (API)
- `helm/agent` – chart for Agent

Publishing
- CI publishes charts in two formats:
  - GitHub Pages: charts/dev (develop) and charts/stable (main)
- OCI (GHCR): oci://ghcr.io/<owner>/kubenova-charts/{manager,agent}
    - lightweight tags: dev (develop) and latest (main)

Install (Helm repo)
```
helm repo add kubenova https://vaheed.github.io/kubenova/charts/stable
helm install manager kubenova/manager -n kubenova-system --create-namespace
```

Notes
- Charts now include icons and the Manager chart supports JWT secret injection via values.
- See per-chart READMEs for install flags and values: `deploy/helm/manager/README.md`, `deploy/helm/agent/README.md`.

Install (OCI)
```
helm registry login ghcr.io -u <user> -p <token>
helm pull oci://ghcr.io/<owner>/kubenova/manager --version latest
```
