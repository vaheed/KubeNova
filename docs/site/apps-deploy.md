# Apps & Deploy

NOTE: KubeNova is the only API; no direct access to underlying platform components.

- Create an app:
```
curl -s -XPOST $BASE/api/v1/clusters/cluster-a/tenants/acme/projects/web/apps \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"hello-web","components":[{"name":"web","type":"webservice","properties":{"image":"ghcr.io/example/hello:latest"}}]}' | jq
```
- Deploy:
```
curl -s -XPOST $BASE/api/v1/clusters/cluster-a/tenants/acme/projects/web/apps/hello-web:deploy \
  -H "Authorization: Bearer $TOKEN"
```
- Status:
```
curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/v1/clusters/cluster-a/tenants/acme/projects/web/apps/hello-web/status | jq
```
- Revisions:
```
curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/v1/clusters/cluster-a/tenants/acme/projects/web/apps/hello-web/revisions | jq
```
- Logs:
```
curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/v1/clusters/cluster-a/tenants/acme/projects/web/apps/hello-web/logs/web | jq
```
