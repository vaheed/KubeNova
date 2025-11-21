---
title: Phase 6 — Tests & Validation
---

# Phase 6 — Tests & Validation

This guide condenses the Phase 6 checklist into runnable steps for units, integration, and RBAC/security verification.

## 1) Unit test checklist

Run the standard Go suite after every change:

```bash
go test ./...
```

Key areas covered by the current codebase:

- `internal/reconcile/app_test.go` now validates metadata propagation, security labels, and the `credentialsSecretRef` wiring.
- `internal/http/server_orphans_test.go` exercises the orphan detection API and ensures only admin/ops roles can query it.
- Catalog, policy set, and App CRUD tests already cover installation/upgrade flows, RBAC enforcement, and metadata snapshots; rerun them via `go test ./internal/http`.

If you add new scenarios (e.g., catalog secrets), expand the relevant package’s tests and keep the `go test ./...` gate clean before committing.

## 2) Kind integration sanity flow

Use the following script to validate the Manager + Agent + Vela chain in a real kind cluster.

```bash
#!/usr/bin/env bash
set -euo pipefail

CLUSTER=${CLUSTER:-kubenova-phase6}
MANAGER_IMAGE=${MANAGER_IMAGE:-$(go env GOPATH)/bin/manager}

kind create cluster --name "$CLUSTER"

# Build the manager binary and run it targeting kind's kubeconfig.
go build -o "$MANAGER_IMAGE" ./cmd/manager
KUBECONFIG=$(kind get kubeconfig-path --name "$CLUSTER")

# Run the manager in the background; capture logs for debugging.
"$MANAGER_IMAGE" >/tmp/manager.log 2>&1 &
MANAGER_PID=$!
trap 'kill $MANAGER_PID; kind delete cluster --name "$CLUSTER"' EXIT

export BASE=http://localhost:8080
export KN_TOKEN=$(
  curl -sS -X POST "$BASE/api/v1/tokens" \
    -H 'Content-Type: application/json' \
    -d '{"subject":"manager-integration","roles":["admin"],"ttlSeconds":3600}' \
  | jq -r '.token'
)
export AUTH_HEADER="Authorization: Bearer $KN_TOKEN"

# Register the kind cluster and wait for the agent to become ready.
curl -sS -X POST "$BASE/api/v1/clusters" \
  -H 'Content-Type: application/json' \
  -H "$AUTH_HEADER" \
  -d '{"name":"kind","kubeconfig":"'"$(base64 < "$KUBECONFIG" | tr -d '\n")"'","capsuleProxyUrl":"https://capsule-proxy.example.com:9001"}' \
  | jq .

# Complete the rest of the flow (tenant → project → catalog install → App → secrets) via curl/kubectl.

# Query the orphan endpoint; it should return empty unless you inject drift.
curl -sS "$BASE/api/v1/clusters/<cluster-id>/apps/orphans" -H "$AUTH_HEADER"
```

The above script is opinionated; adapt the cluster name, manager binary path, and post-registration steps to fit your environment.

## 3) RBAC & secret propagation validation

Use these scenarios to verify security posture:

1. **Read-only vs sandbox**:
   - `X-KN-Roles: readOnly` against `/api/v1/clusters/{c}/tenants/{t}/projects/{p}/apps`: should succeed for GET and fail for POST.
   - `X-KN-Roles: tenantOwner` when hitting `/api/v1/tenants/{t}/sandbox`: should succeed.
2. **Secret usage for App deployments**:
   - Create `SecretRef` secrets (docker-registry, Git, Helm) in the app namespace.
   - Install a catalog item referencing them.
   - After the Agent reconciles, describe the resulting Vela Application (`kubectl describe applications.core.oam.dev/<app> -n tn-...`) and ensure `imagePullSecrets`/`credentialsSecretRef` mirror the referenced secrets.
3. **Drift detection**
   - Delete the `App` row or remove the `kubenova.io/app-id` label from a Vela Application.
   - Query `/api/v1/clusters/{c}/apps/orphans` (admin/ops only) and confirm the orphan is listed with the right reason.

## Notes

- There is no automated Kind test in this repo yet; the script above is a starting point for a CI job once you containerize the manager and agent.
- Telemetry hooks already log kubeconfig issuance and orphan reconciliation; consider adding metrics around secret usage/orphan counts as a follow-up.
