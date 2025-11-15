---
title: KubeNova Roadmap
---

# KubeNova Roadmap

This roadmap focuses on a clear platform goal:

> Use the KubeNova API to create tenants, projects, and apps; let users manage workloads via Capsule/capsule‑proxy (`kubectl`); and provide “user‑area” app delivery flows such as WordPress (via Helm) and Grafana (via container images), including edit/delete operations.

It is based on what exists today in the codebase (Tenants/Projects/Apps, Capsule & Vela integrations, kubeconfig endpoints) and what is still needed to deliver these flows end‑to‑end.

## 0. Current Baseline (already implemented)

- **Tenants & Projects**
  - Tenants and Projects are first‑class API resources (`docs/openapi/openapi.yaml`, `internal/http`).
  - Projects are mirrored into Kubernetes `Namespace` objects with Capsule labels and `kubenova.project`/`kubenova.tenant` via `internal/reconcile/project.go`.
  - Project access is managed via `PUT /projects/{p}/access`, which creates `Role`/`RoleBinding` objects for roles `tenantOwner`, `projectDev`, and `readOnly` (`internal/cluster/projects.go`).

- **Apps & KubeVela**
  - Apps exist in the API (`/clusters/{c}/tenants/{t}/projects/{p}/apps`) and are persisted via `internal/store`.
  - App operations (deploy/suspend/resume/rollback/status/logs/traits/policies/image-update) are wired to KubeVela via `internal/backends/vela`.
  - The Agent’s `AppReconciler` projects ConfigMaps describing apps into Vela `Application` resources (`internal/reconcile/app.go`).

- **Access proxy & kubeconfigs**
  - Cluster registration stores kubeconfigs centrally and installs an Agent into target clusters.
  - Tenant- and project-scoped kubeconfig endpoints:
    - `POST /api/v1/tenants/{t}/kubeconfig`
    - `GET /api/v1/clusters/{c}/tenants/{t}/projects/{p}/kubeconfig`
  - Kubeconfigs:
    - Point at the configured access proxy (`CAPSULE_PROXY_URL`), not the raw kube‑apiserver.
    - Embed JWTs with `tenant`, optional `project`, `roles`, and `exp`.
    - Use the same role→group mapping documented in `docs/README.md` and `docs/index.md` so Capsule/capsule‑proxy + RBAC can enforce access.

With this baseline, operators can already create clusters, tenants, projects, and simple apps via the API, and users can manage workloads with `kubectl` using issued kubeconfigs.

## 1. Tenant/Project/App API Flow (User Area)

**Goal:** A user provisions their own “area” via the API (Tenant + Project + App) and then manages resources through `kubectl` (capsule‑proxy).

- **1.1 API flows & docs**
  - Keep the canonical flow in `docs/index.md`:
    - `POST /clusters`
    - `POST /clusters/{c}/tenants`
    - `POST /clusters/{c}/tenants/{t}/projects`
    - `POST /clusters/{c}/tenants/{t}/projects/{p}/apps`
  - Ensure errors on these paths return consistent KN‑style codes (`KN‑4xx/KN‑500`) and examples are updated accordingly.

- **1.2 Kubeconfig usage**
  - Treat `POST /tenants/{t}/kubeconfig` and `GET /clusters/{c}/tenants/{t}/projects/{p}/kubeconfig` as the canonical entry points for users.
  - Extend `docs/index.md` with explicit examples for:
    - Saving kubeconfigs to files.
    - Running `kubectl` through capsule‑proxy from tenant and project scopes.

## 2. WordPress App via API (Helm‑based)

**Goal:** A user creates a WordPress app via the API; KubeNova (via Agent/Vela/Helm) deploys it into the project namespace.

- **2.1 WordPress App spec**
  - Design an App spec (components/traits/policies) that maps to a WordPress deployment:
    - Chart location (Helm repo/name/version) or a Vela component that wraps the chart.
    - Basic configuration: DB credentials, storage class, ingress hostname.
  - Add an example WordPress App payload to `docs/openapi/openapi.yaml` and `docs/index.md`.

- **2.2 Controller/integration**
  - Extend the Vela backend or add a small adapter so a “WordPress app”:
    - Renders and applies the Helm chart into the project namespace.
    - Supports upgrades and rollbacks via existing app operations.

- **2.3 User flow & checks**
  - Document the flow:
    - `POST /apps` to create WordPress.
    - `POST /apps/{a}:deploy` to roll it out.
    - `kubectl` checks for WordPress pods, Services, and Ingress readiness.

## 3. Grafana App via API (Image + Registry)

**Goal:** A user creates a Grafana app via the API using a container image from a registry, optionally with secrets for credentials and datasources.

- **3.1 Grafana App spec**
  - Define a standard App component for “webservice + config”:
    - `image` and `tag`.
    - Optional `env`, `config`/`secretRef`, and `ingress` traits.
  - Ensure the Vela backend supports updating image (`image-update`) and traits/policies for this app.

- **3.2 Registry & credentials**
  - Decide how registry credentials are provided:
    - Via Kubernetes `Secret` referenced in the App spec.
    - Or via a platform‑level default, documented clearly for operators.

- **3.3 User flow & checks**
  - Add curl examples for:
    - Creating a Grafana App with a registry image.
    - Deploying it and updating its image using `:image-update`.
  - Add `kubectl` checks:
    - Pods, Services, and Ingress for Grafana in the project namespace.

## 4. App Lifecycle in the User Area (Edit / Delete)

**Goal:** Users can safely edit and delete their apps using only the API and their `kubectl` access, without breaking the platform.

- **4.1 Editing apps**
  - Clarify and, where necessary, implement how `PUT /apps/{a}` affects:
    - Stored App spec (components/traits/policies).
    - ConfigMaps consumed by `AppReconciler`.
    - Vela `Application` objects (redeploy or rolling update semantics).
  - Document supported edit operations (e.g., image, traits, policies) and which ones trigger redeploys.

- **4.2 Deleting apps**
  - Ensure `DELETE /apps/{a}` and `POST /apps/{a}:delete` clean up:
    - Store records.
    - Vela `Application` and revisions (via AppsAdapter).
    - Optional ConfigMaps used as app projections.
  - Add delete flows to `docs/index.md` plus `kubectl` checks to confirm workloads are removed.

- **4.3 Guardrails & UX**
  - Add validation to prevent edits that leave Vela Apps in an inconsistent state.
  - Consider lightweight status fields on App resources indicating last deploy, last edit, and last error.

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

This roadmap should make it clear how to get from the current implementation to a user‑facing “app platform” where tenants, projects, and apps are created via the API and managed day‑to‑day via capsule‑proxy and standard Kubernetes tools.
