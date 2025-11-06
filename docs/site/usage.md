# Usage

NOTE: KubeNova is the only API; no direct access to underlying platform components.

- Tenant usage (24h):
```
curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/v1/tenants/acme/usage?range=24h | jq
```
- Project usage (7d):
```
curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/v1/projects/web/usage?range=7d | jq
```
