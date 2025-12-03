---
title: Multi-Tenancy Model
---

# Multi-Tenancy Model

KubeNova enforces tenant isolation with Capsule and Capsule Proxy. Each tenant receives predictable namespaces, service accounts, kubeconfigs, and access proxy endpoints.

## Tenant layout
- Capsule Tenant object
- Namespaces:
  - `<tenant>-owner` (admin/owner operations)
  - `<tenant>-apps` (application workloads)
- ServiceAccounts:
  - Owner (full access within tenant namespaces)
  - Readonly (get/list/watch)
- Two kubeconfigs per tenant derived from the ServiceAccounts (Secret `kubenova-kubeconfigs` in `<tenant>-owner`; Manager API surfaces these). The kubeconfigs point to the per-tenant Capsule Proxy endpoint (`<proxy>/<tenant>/<role>`), never the raw kube-apiserver.
- One KubeVela project per tenant; unlimited KubeVela applications per project

## Defaulting rules
- If owner/apps namespaces are not provided, the operator derives them from the tenant name.
- Proxy endpoint defaults to `https://proxy.kubenova.local/<tenant>` when not provided; set `capsuleProxyEndpoint` when registering the cluster to publish tenant kubeconfigs via your real proxy.
- Plan/policy catalogs are plumbed via adapters; defaults apply during tenant creation when configured.

## Enforcement path
1. Tenant CRD → Capsule Tenant + namespaces + RBAC.
2. Proxy backend publishes tenant-specific endpoints (Capsule Proxy).
3. Projects → namespaces ensured, Vela Project created, kubeconfig issuance endpoints exposed by the Manager.
4. Apps → translated into KubeVela Applications with traits/policies maintained by the operator.

## Relevant ADRs
- ADR-001/003/006: isolated clusters, manager never talks directly to kube-apiservers, single shared cluster per datacenter.
- ADR-004: Capsule + Capsule Proxy for tenancy boundaries.
- ADR-009: Two namespaces + two kubeconfigs per tenant.

See [ADR index](../reference/adr.md) for the complete decision set.
