---
title: KubeNova API v1 – cURL Quickstart
---

# KubeNova API v1 – cURL Quickstart

This page walks through a complete end‑to‑end flow using `curl`, from getting an access token to deploying a simple app. Every step shows:

- the exact command to run, and
- a short explanation of what it does.

All examples use the v1 HTTP API defined in `docs/openapi/openapi.yaml` and implemented by the manager in `internal/http`. Only implemented endpoints are shown here; for the full contract (including future additions) see `docs/openapi/openapi.yaml` and `docs/README.md`.

> You can copy‑paste these snippets into any POSIX shell (bash/zsh). On Windows, run them from WSL or adapt the syntax to PowerShell.

---

## 0) Prerequisites & base URL


---

## Where to go next

- For the full OpenAPI contract and example payloads, see `docs/openapi/openapi.yaml`.
- For a high‑level overview and endpoint matrix, see `docs/README.md`.
- For future and not‑yet‑implemented endpoints, track the roadmap via GitHub issues and milestones and the OpenAPI file; they are intentionally omitted from this quickstart until fully implemented in `internal/http`.
