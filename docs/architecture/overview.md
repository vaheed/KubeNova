---
title: Architecture Overview
---

# Architecture Overview

KubeNova is a federated control plane: a single global **Manager** orchestrates metadata, tenancy, and app lifecycles across many **sovereign clusters**. The Manager never calls cluster APIs directly; each cluster runs a **KubeNova Operator** that connects outbound-only to the Manager.

## Components
- **Manager** – HTTP API (`/api/v1`), Postgres-backed store, Helm client to bootstrap operators, OpenTelemetry hooks, and structured logging.
- **Operator** – controller-runtime manager that installs and reconciles:
  - Capsule (multi-tenancy), Capsule Proxy (per-tenant LB isolation)
  - KubeVela (application orchestration) + optional FluxCD/Velaux
  - Nova CRDs (tenants, projects, apps) projected into Capsule/Vela resources.
- **Store** – Postgres (required) with in-memory fallback only in tests.
- **Adapters** – translate Nova models into Capsule/Vela manifests; Helm installer for addons.

![KubeNova Diagram](../diagrams/KubeNova.png)

## Control plane principles
- Clusters are isolated; no cross-datacenter sharing.
- Outbound-only connectivity from clusters; operator maintains the only gRPC stream.
- Idempotent APIs and reconcilers; prefer retries + backoff over imperative flows.
- Structured observability (logs/metrics/traces) for every handler and bootstrap step.

## Lifecycle (happy path)
1. Register cluster → Manager installs operator via Helm using the provided kubeconfig.
2. Operator bootstraps Capsule, Capsule Proxy, KubeVela (and addons) into their dedicated namespaces (`cert-manager`, `capsule-system`, `vela-system`) while the operator itself and Manager live in `kubenova-system`.
3. Tenants/projects/apps are created through the Manager API and projected by the operator.
4. Operator reports status/usage back to the Manager; manager serves read-only summaries.

## Error handling & API contract
- OpenAPI source of truth: `docs/openapi/openapi.yaml`.
- Errors use structured payloads with `code` + `message` (`KN-400/401/403/404/409/422/500`).
- Auth/RBAC (when enabled): HS256 JWT with roles `admin|ops|tenantOwner|projectDev|readOnly`. Tests can simulate roles with `X-KN-Roles`.
