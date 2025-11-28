---
title: KubeNova v0.1.2
---

# KubeNova

Unified multi-datacenter CaaS/PaaS control plane that keeps clusters sovereign while a single manager coordinates metadata, tenant lifecycle, and application orchestration through outbound-only operators.

- Manager API: `/api/v1` (JWT optional), contract defined in `docs/openapi/openapi.yaml`
- Operators: install Capsule, Capsule Proxy, KubeVela, FluxCD, and Velaux per cluster
- Tenants: opinionated two-namespace layout with kubeconfigs per role
- Transport: outbound gRPC from clusters; manager never talks directly to kube-apiservers

## Why this repo matters
- Canonical API + OpenAPI spec for the manager and its DTOs
- Helm charts for manager/operator, dev docker-compose, kind assets, and diagrams
- Baseline release `v0.1.2`: cleaned repository, VitePress docs, env hardening, refreshed roadmap, and kind-based E2E guidance

## Start here
- Read the [Quickstart](getting-started/quickstart.md) to run the stack locally.
- Follow the [API lifecycle walkthrough](getting-started/api-playbook.md) for end-to-end flows.
- See [Configuration](reference/configuration.md) for required environment variables.
- Validate with [kind E2E tests](operations/kind-e2e.md) and [Upgrades](operations/upgrade.md).
