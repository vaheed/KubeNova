## v0.1.2 – Vela bootstrap hardening

- Wait for the `vela-addon-registry` ConfigMap before enabling Vela addons so Velaux installs stop failing when the registry is still pending.
- Disable kubevela multicluster/cluster-gateway by default to avoid installing `kubevela-cluster-gateway-service` in managed clusters.
- Bump default images/charts/docs to v0.1.2.

## v0.1.1 – Cleaned baseline

- Repository hygiene: removed generated artifacts, standardized docs to VitePress, added kind helper script, and clarified single-source env handling.
- Documentation: new quickstart, API lifecycle walkthrough, operations (observability, upgrades, kind E2E), refreshed README, and roadmap.
- Version alignment: bumped charts/OpenAPI/manager version to v0.1.1 and introduced live integration test scaffolding against a running manager + kind cluster.
