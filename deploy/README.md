# deploy/

Helm charts and deployment assets.

- `helm/manager` – chart for Manager (API)
- `helm/kubenova-agent` – chart for Agent

Publishing
- CI publishes charts in two formats:
  - GitHub Pages: charts/dev (develop) and charts/stable (main)
- OCI (GHCR): oci://ghcr.io/<owner>/kubenova-charts/{manager,kubenova-agent}
    - lightweight tags: dev (develop) and latest (main)

Install (Helm repo)
```
helm repo add kubenova https://vaheed.github.io/kubenova/charts/stable
helm install manager kubenova/manager -n kubenova-system --create-namespace
```

Install (OCI)
```
helm registry login ghcr.io -u <user> -p <token>
helm pull oci://ghcr.io/<owner>/kubenova-charts/manager --version latest
```
