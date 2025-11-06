KubeNova Agent — Helm Chart

Install
- Add repo: `helm repo add kubenova https://vaheed.github.io/kubenova/charts/stable`
- Install/upgrade:
```
helm upgrade --install agent kubenova/agent \
  -n kubenova-system \
  --set image.tag=latest \
  --set manager.url=http://kubenova-manager.kubenova-system.svc.cluster.local:8080 \
  --set redis.enabled=true \
  --set bootstrap.capsuleVersion=0.10.6 \
  --set bootstrap.capsuleProxyVersion=0.9.13
```

Values
- `image.repository` (string) – container registry repo
- `image.tag` (string) – image tag
- `image.pullPolicy` (string) – IfNotPresent/Always
- `manager.url` (string) – Manager service URL
- `manager.batchIntervalSeconds` (int)
- `manager.batchMaxItems` (int)
- `redis.enabled` (bool) – sidecar redis for buffering
- `redis.image` (string)
- `redis.addr` (string)
- `bootstrap.capsuleVersion` (string)
- `bootstrap.capsuleProxyVersion` (string)
- `bootstrap.velaCoreVersion` (string)

