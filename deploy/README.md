# deploy/

Helm charts and deployment assets.

- `helm/kubenova-api` – chart for Manager API
- `helm/kubenova-agent` – chart for Agent

Publishing
- CI publishes charts in two formats:
  - GitHub Pages: charts/dev (develop) and charts/stable (main)
  - OCI (GHCR): oci://ghcr.io/<owner>/kubenova/{kubenova-api,kubenova-agent}
    - lightweight tags: latest-dev (develop) and latest (main)

Install (Helm repo)
```
helm repo add kubenova https://vaheed.github.io/kubenova/charts/stable
helm install kubenova-api kubenova/kubenova-api -n kubenova --create-namespace
```

Install (OCI)
```
helm registry login ghcr.io -u <user> -p <token>
helm pull oci://ghcr.io/<owner>/kubenova/kubenova-api --version latest
```
