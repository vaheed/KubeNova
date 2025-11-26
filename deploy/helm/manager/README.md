KubeNova Manager — Helm Chart

Install
- Add repo: `helm repo add kubenova https://vaheed.github.io/kubenova/charts/stable`
- Install/upgrade:
```
helm upgrade --install manager kubenova/manager \
  -n kubenova-system --create-namespace \
  --set image.tag=latest \
  --set env.KUBENOVA_REQUIRE_AUTH=true \
  --set env.MANAGER_URL_PUBLIC=http://kubenova-manager.kubenova-system.svc.cluster.local:8080 \
```

JWT Signing Key
- Use an existing Secret:
```
--set jwt.existingSecret=my-manager-secret --set jwt.key=JWT_SIGNING_KEY
```
- Or have the chart create one (dev only):
```
--set jwt.value="super-secret-key" --set jwt.key=JWT_SIGNING_KEY
```

Values
- `image.repository` (string) – container registry repo
- `image.tag` (string) – image tag
- `image.pullPolicy` (string) – IfNotPresent/Always
- `env.KUBENOVA_REQUIRE_AUTH` (bool string) – "true" to enforce JWT
- `env.MANAGER_URL_PUBLIC` (string) – public URL for callbacks/clients
- `env.DEFAULT_NS_RESOURCEQUOTA` (string, optional) – JSON for defaults
- `env.DEFAULT_PROJECT_QUOTA` (string, optional) – JSON for defaults
- `jwt.existingSecret` (string) – name of Secret that holds JWT key
- `jwt.value` (string) – inline key (chart will create Secret)
- `jwt.key` (string) – secret key name (default `JWT_SIGNING_KEY`)
