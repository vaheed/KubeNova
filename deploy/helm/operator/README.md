KubeNova Operator — Helm Chart

Install
- Add repo: `helm repo add kubenova https://vaheed.github.io/kubenova/charts/stable`
- Install/upgrade:
```
helm upgrade --install operator kubenova/operator \
  -n kubenova-system \
  --set image.tag=v0.1.3 \
  --set manager.url=http://kubenova-manager.kubenova-system.svc.cluster.local:8080 \
  --set redis.enabled=true \
  --set bootstrap.capsuleVersion=0.10.6 \
  --set bootstrap.capsuleProxyVersion=0.9.13
```
Release images follow git tag names and keep the leading `v` (e.g., `v0.1.3`).
The chart bundles the NovaTenant/NovaProject/NovaApp CRDs under `crds/` so no extra `kubectl apply -k deploy/crds` step is required.

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
- `otel.endpoint` (string) – OTLP gRPC endpoint (e.g., SigNoz collector)
- `otel.insecure` (bool) – set true for `http://` endpoints
- `otel.environment` (string) – value for `deployment.environment` in traces
- `otel.version` (string) – overrides reported service version
- `otel.resourceAttributes` (string) – comma-separated OTEL_RESOURCE_ATTRIBUTES
