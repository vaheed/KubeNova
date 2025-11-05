# Policies & Security

NOTE: KubeNova is the only API; no direct access to underlying platform components.

- Catalog:
```
curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/v1/clusters/cluster-a/policysets/catalog | jq
```
- Create a PolicySet and attach to a tenant:
```
curl -s -XPOST $BASE/api/v1/clusters/cluster-a/tenants/acme/policysets \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"podsecurity-baseline","rules":[{"action":"enforce","name":"baseline"}]}' | jq
```
