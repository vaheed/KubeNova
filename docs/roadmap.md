---
title: KubeNova Roadmap
---

# KubeNova Roadmap

This roadmap lists upcoming work only. It assumes the existing API surface, Capsule/capsule‑proxy integration, and KubeVela adapter are in place, and focuses on what we still need to build to deliver a clean “user area” app platform.

> Goal: Use the KubeNova API to create tenants, projects, and apps; let users manage workloads via Capsule/capsule‑proxy (`kubectl`); and provide app flows such as WordPress (via Helm) and Grafana (via container images), including edit/delete operations.

## 1. Tenant/Project/App API Flow (User Area)

**Goal:** A user provisions their own “area” via the API (Tenant + Project + App) and then manages resources through `kubectl` (capsule‑proxy).

- **1.1 API flows & docs**
  - Keep the canonical flow in `docs/index.md`:
    - `POST /clusters`
    - `POST /clusters/{c}/tenants`
    - `POST /clusters/{c}/tenants/{t}/projects`
    - `POST /clusters/{c}/tenants/{t}/projects/{p}/apps`
  - Ensure errors on these paths return consistent KN‑style codes (`KN‑4xx`/`KN‑500`) and response examples match actual behavior.

- **1.2 Kubeconfig usage**
  - Treat:
    - `POST /api/v1/tenants/{t}/kubeconfig`
    - `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/kubeconfig`
    as the canonical entry points for user‑area access.
  - Extend `docs/index.md` with explicit examples for:
    - Saving tenant/project kubeconfigs to files.
    - Running `kubectl` against capsule‑proxy from tenant and project scopes.
  - Make the role→group mapping (`tenantOwner`/`projectDev`/`readOnly`) obvious to operators and users.

## 2. WordPress App via API (Helm‑based)

**Goal:** A user creates a WordPress app via the API; KubeNova (via Agent/Vela/Helm) deploys it into the project namespace.

- **2.1 WordPress App spec**
  - Design an App spec (components/traits/policies) that maps to a WordPress deployment:
    - Chart location (Helm repo/name/version) or a Vela component that wraps the WordPress chart.
    - Basic configuration: DB credentials, storage class, ingress hostname.
  - Add an example WordPress App payload to:
    - `docs/openapi/openapi.yaml` (App examples).
    - `docs/index.md` (curl section for “Create WordPress app”).

- **2.2 Controller/integration**
  - Extend the Vela backend or add a small adapter so a “WordPress app”:
    - Renders and applies the Helm chart into the project namespace.
    - Supports upgrades and rollbacks via existing app operations (`:deploy`, `:rollback`, etc.).

- **2.3 User flow & checks**
  - Document the full flow in `docs/index.md`:
    - `POST /apps` to create the WordPress App resource.
    - `POST /apps/{a}:deploy` to roll it out.
  - Add `kubectl` checks:
    - Pods, Services, and Ingress resources for WordPress in the project namespace.

## 3. Grafana App via API (Image + Registry)

**Goal:** A user creates a Grafana app via the API using a container image from a registry, optionally with secrets for credentials and datasources.

- **3.1 Grafana App spec**
  - Define a standard App component for “webservice + config”:
    - `image` and `tag`.
    - Optional `env`, `config`/`secretRef`, `ingress` traits.
  - Ensure the Vela backend supports:
    - Initial deploy from the image.
    - Image updates via the `:image-update` endpoint.

- **3.2 Registry & credentials**
  - Decide how registry credentials are provided:
    - Via Kubernetes `Secret` referenced in the App spec.
    - Or via a platform‑level default; document expectations in `docs/README.md`.

- **3.3 User flow & checks**
  - Add curl examples for:
    - Creating a Grafana App that references a registry image.
    - Deploying it and updating its image using `:image-update`.
  - Add `kubectl` checks:
    - Pods, Services, and Ingress for Grafana in the project namespace.

## 4. App Lifecycle in the User Area (Edit / Delete)

**Goal:** Users can safely edit and delete their apps using only the API and their `kubectl` access, without breaking the platform.

- **4.1 Editing apps**
  - Clarify and, where necessary, implement how `PUT /apps/{a}` affects:
    - Stored App spec (components/traits/policies).
    - ConfigMaps consumed by `AppReconciler`.
    - Vela `Application` objects (redeploy vs. rolling update semantics).
  - Document supported edit operations:
    - For example: image, traits (scaling, ingress), and policies.
    - Which edits trigger a redeploy.

- **4.2 Deleting apps**
  - Ensure `DELETE /apps/{a}` and `POST /apps/{a}:delete` clean up:
    - Store records.
    - Vela `Application` and revisions (via the Vela backend).
    - App‑specific ConfigMaps (if they are not reused).
  - Add delete flows to `docs/index.md` with `kubectl` checks to confirm workloads are removed.

- **4.3 Guardrails & UX**
  - Add validation to prevent edits that leave Vela Apps in an inconsistent state.
  - Consider lightweight status fields on App resources indicating:
    - Last successful deploy.
    - Last edit time.
    - Last error (if any).

## 5. Operator & User Experience Enhancements

- **Unified documentation**
  - Keep `docs/index.md` as the single “curl + kubectl” guide for:
    - Creating tenants, projects, and apps.
    - Issuing tenant/project kubeconfigs and using them with capsule‑proxy.
    - Creating, deploying, editing, and deleting WordPress and Grafana apps.

- **Examples & templates**
  - Provide ready‑to‑use example payloads (JSON) for:
    - Minimal tenant + project.
    - WordPress app spec.
    - Grafana app spec.
  - Optionally ship these as JSON files under `docs/examples` for easy reuse.

