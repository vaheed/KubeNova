# SigNoz (OTLP) integration

KubeNova now emits OpenTelemetry spans that can be sent directly to [SigNoz](https://github.com/SigNoz/signoz) via OTLP/gRPC. HTTP traffic is traced automatically (chi middleware) and logs include the `trace_id` for correlation.

## 1) Run SigNoz (quick start)
```
git clone https://github.com/SigNoz/signoz.git
cd signoz/deploy/docker
docker compose -f docker-compose.yaml up -d
```
The OTLP gRPC endpoint defaults to `http://localhost:4317`.

## 2) Configure KubeNova
- Set these env vars for both Manager and Operator (locally or in the Helm values):
```
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_EXPORTER_OTLP_INSECURE=true
KUBENOVA_ENV=dev
KUBENOVA_VERSION=0.0.1
# Optional extra attributes (comma-separated)
OTEL_RESOURCE_ATTRIBUTES=service.namespace=control-plane,team=platform
```
- Helm examples (replace namespaces/hosts as needed):
```
helm upgrade --install manager kubenova/manager \
  -n kubenova-system --create-namespace \
  --set image.tag=latest \
  --set otel.endpoint=http://signoz-otel-collector:4317 \
  --set otel.insecure=true \
  --set otel.environment=staging \
  --set otel.version=0.0.1

helm upgrade --install operator kubenova/operator \
  -n kubenova-system \
  --set image.tag=latest \
  --set manager.url=http://kubenova-manager.kubenova-system.svc.cluster.local:8080 \
  --set otel.endpoint=http://signoz-otel-collector:4317 \
  --set otel.insecure=true \
  --set otel.environment=staging \
  --set otel.version=0.0.1
```
For the local `docker-compose.dev.yml`, place the variables above into `.env` (already referenced by the compose file).

## 3) Verify in SigNoz
- Open the SigNoz UI and look for the `kubenova-manager` and `kubenova-operator` services.
- Trace and log correlation: request logs now include `trace_id`; use it to jump between SigNoz traces and manager logs.
