# API Quickstart

NOTE: KubeNova is the only API; no direct access to underlying platform components.

- Auth: JWT (HS256). Obtain a token:
```
curl -s -XPOST http://localhost:8080/api/v1/tokens \
  -H 'Content-Type: application/json' \
  -d '{"subject":"demo","roles":["admin"],"ttlSeconds":3600}' | jq -r .token
```
Export as `TOKEN` and base URL:
```
export BASE=http://localhost:8080
export TOKEN="$(curl -s -XPOST $BASE/api/v1/tokens -H 'Content-Type: application/json' -d '{"subject":"demo","roles":["admin"],"ttlSeconds":3600}' | jq -r .token)"
```
- Versioning: All routes under `/api/v1`.

- Register a cluster (base64 kubeconfig):
```
KCFG=$(base64 -w0 ~/.kube/config 2>/dev/null || base64 ~/.kube/config)
curl -s -XPOST $BASE/api/v1/clusters \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"kind","kubeconfig":"'"$KCFG"'","labels":{"env":"dev"}}' | jq
```
- List clusters:
```
curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/v1/clusters | jq
```
- OpenAPI: `GET $BASE/openapi.yaml` (contract-first).
