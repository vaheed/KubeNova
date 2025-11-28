---
title: Observability
---

# Observability

KubeNova emits structured logs, Prometheus metrics, and OpenTelemetry traces.

## OpenTelemetry / SigNoz
- Manager and operator expose OTLP/gRPC; configure via environment:
  - `OTEL_EXPORTER_OTLP_ENDPOINT` (e.g. `http://localhost:4317`)
  - `OTEL_EXPORTER_OTLP_INSECURE=true` when using HTTP endpoints
  - `KUBENOVA_ENV` (dev|staging|prod), `KUBENOVA_VERSION` (default `v0.1.1`)
  - Optional: `OTEL_RESOURCE_ATTRIBUTES` (comma-separated key=value pairs)
- SigNoz quickstart:
```bash
git clone https://github.com/SigNoz/signoz.git
cd signoz/deploy/docker
docker compose -f docker-compose.yaml up -d
```
- Helm examples (release images are tagged with a leading `v`):
```bash
helm upgrade --install manager deploy/helm/manager \
  -n kubenova-system --create-namespace \
  --set image.tag=v0.1.1 \
  --set otel.endpoint=http://signoz-otel-collector:4317 \
  --set otel.insecure=true \
  --set otel.environment=staging \
  --set otel.version=v0.1.1

helm upgrade --install operator deploy/helm/operator \
  -n kubenova-system \
  --set image.tag=v0.1.1 \
  --set manager.url=http://kubenova-manager.kubenova-system.svc.cluster.local:8080 \
  --set otel.endpoint=http://signoz-otel-collector:4317 \
  --set otel.insecure=true \
  --set otel.environment=staging \
  --set otel.version=v0.1.1
```
- Expect services `kubenova-manager` and `kubenova-operator` to appear in SigNoz; logs include `trace_id` for correlation.

## Metrics and logging
- Manager logs are JSON with `request_id`, `tenant`, `cluster`, `adapter`, `trace_id`.
- Metrics (examples): `kubenova_reconcile_seconds`, `kubenova_events_total`, `kubenova_adapter_errors_total`.
- Scrape metrics endpoints via the Kubernetes service when deployed with Helm.
