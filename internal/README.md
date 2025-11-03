# internal/

Private packages for the KubeNova Manager and Agent.

- `manager/` – HTTP server, routes, auth, handlers
- `cluster/` – Manager-side agent installer (embedded manifests)
- `reconcile/` – Agent reconcilers (Projects→Namespaces, Apps→Vela, bootstrap job)
- `adapters/` – Translators to Capsule and KubeVela CRDs (unstructured)
- `store/` – Persistence (Postgres + in-memory)
- `security/` – Envelope encryption utilities
- `telemetry/` – Heartbeat, Redis buffering, OpenTelemetry
- `metrics/` – Prometheus metrics
- `logging/` – Zap logger setup, request/trace correlation helpers
- `util/` – Retry/backoff with jitter

Conventions
- All operations are idempotent; reconcilers use finalizers and backoff.
- New APIs/flows must include E2E coverage and structured telemetry.
