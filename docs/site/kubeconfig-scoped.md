# Kubeconfig (Scoped)

NOTE: KubeNova is the only API; no direct Capsule/KubeVela usage.

- Issue tenant-scoped kubeconfig via proxy:
```
curl -s -XPOST $BASE/api/v1/tenants/acme/kubeconfig \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"project":"web","role":"projectDev","ttlSeconds":3600}' | jq -r .kubeconfig | base64 --decode
```
- Save to file:
```
curl -s -XPOST $BASE/api/v1/tenants/acme/kubeconfig -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"project":"web","role":"readOnly"}' \
  | jq -r .kubeconfig | base64 --decode > kubeconfig-tenant
```

