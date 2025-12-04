---
title: Architecture Decisions
---

# Architecture Decisions (ADR)

The ADR set lives at `docs/ADR.md` and must remain intact.

- ADR-001: Global manager with per-datacenter isolated clusters
- ADR-002: Outbound-only gRPC operator model
- ADR-003: Manager must not talk directly to Kubernetes APIs
- ADR-004: Capsule & Capsule Proxy for multi-tenancy
- ADR-005: KubeVela as orchestrator
- ADR-006: Per-datacenter shared cluster
- ADR-007: Pull-from-DB read-only application status
- ADR-008: Hourly usage aggregation in operator
- ADR-009: Two namespaces + two kubeconfigs per tenant

When proposing changes that affect architecture, add a new ADR instead of altering existing ones.
