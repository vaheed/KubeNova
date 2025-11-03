# End-to-End Tests (Kind)

The end-to-end suite is implemented in Go under `tests/e2e/` and mirrors the production control-plane flow:

1. Create a Kind cluster (or reuse an existing cluster when `E2E_USE_EXISTING_CLUSTER=true`).
2. Optionally build fresh Manager and Agent images (`E2E_BUILD_IMAGES=true`) and load them into the Kind nodes.
3. Install the Manager chart into the cluster, port-forward the service, and generate a scoped service-account kubeconfig.
4. Register the cluster through `POST /api/v1/clusters` and wait for the Manager to deploy the Agent.
5. Assert that the Agent runs with two ready replicas, Capsule/capsule-proxy/KubeVela deployments are healthy, and the Manager reports `AgentReady`/`AddonsReady` conditions as `True`.
6. Export Kind diagnostics and clean up Helm releases and the cluster (unless `E2E_SKIP_CLEANUP=true`).

All tests log every major step via `slog` and run in parallel subtests to keep scenarios isolated.

## Running locally

```bash
# Build fresh images, create a Kind cluster, run the suite, and clean everything up
E2E_BUILD_IMAGES=true make test-e2e
```

Key environment flags:

| Variable | Default | Description |
| --- | --- | --- |
| `E2E_KIND_CLUSTER` | `kubenova-e2e` | Name of the Kind cluster the suite manages. |
| `E2E_BUILD_IMAGES` | `false` | When `true`, builds local Manager/Agent images and loads them into Kind before installation. |
| `E2E_USE_EXISTING_CLUSTER` | `false` | Reuse an already-provisioned Kind cluster instead of creating/deleting it. |
| `E2E_SKIP_CLEANUP` | `false` | Preserve the Helm release and Kind cluster for inspection. |
| `E2E_MANAGER_PORT` | `18080` | Local port used for `kubectl port-forward` to the Manager service. |
| `E2E_REPO_ROOT` | auto-detected | Absolute path to the repository root passed to external commands; override when running from a different working directory. |

To capture logs without running CI, export them manually after the suite completes:

```bash
kind export logs artifacts/kind --name ${E2E_KIND_CLUSTER:-kubenova-e2e}
```

## Continuous Integration

`.github/workflows/ci.yml` runs a single `e2e_suite` job on every pull request:

- Installs Kind, kubectl, and Helm.
- Builds local Manager/Agent images and loads them into Kind.
- Executes `make test-e2e` with full logging.
- Uploads the Go test log plus `kind export logs` as artifacts.

The job gates chart publication and OCI pushes, ensuring the real cluster bootstrap path stays healthy.

## Extending the suite

- Add new scenarios in `tests/e2e/scenarios/` and keep them idempotent (`t.Parallel()` in each test function).
- Share setup helpers from `tests/e2e/setup/` and checks from `tests/e2e/assertions/` so that logging and retries stay consistent.
- Use environment toggles (for example `E2E_BUILD_IMAGES`) instead of rewriting helper functions when tests need special behavior.
