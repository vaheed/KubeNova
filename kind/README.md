# kind assets

Utilities for running KubeNova integration tests on a local kind cluster.

- `kind-config.yaml` – cluster config (ipv4, registry mirrors, 3 nodes).
- `metallb-config.yaml` – MetalLB IP pool.
- `Dockerfile` – optional container image for the compose-based kind helper.
- `entrypoint.sh` – used by the compose helper to create the cluster and export kubeconfig.
- `e2e.sh` – host-side script to create the cluster, install MetalLB, export kubeconfig, and optionally load images/register with the manager.

See `docs/operations/kind-e2e.md` for the full workflow and teardown steps.
