# Tenants & Projects

NOTE: KubeNova is the only API; no direct Capsule/KubeVela usage.

- Create a tenant on a cluster:
```
curl -s -XPOST $BASE/api/v1/clusters/cluster-a/tenants \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"acme","owners":["alice"],"labels":{"tier":"gold"}}' | jq
```
- List tenants (with pagination):
```
curl -s -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/clusters/cluster-a/tenants?limit=50" | jq
```
- Create a project:
```
curl -s -XPOST $BASE/api/v1/clusters/cluster-a/tenants/acme/projects \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"web","labels":{"env":"dev"}}' | jq
```
- Get tenant summary:
```
curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/v1/clusters/cluster-a/tenants/acme/summary | jq
```

