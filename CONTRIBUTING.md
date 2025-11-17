# Contributing to KubeNova

Thanks for your interest in improving KubeNova! This document describes how to
file issues, propose changes, and work with the codebase.

## How to file issues

- Use GitHub Issues for:
  - Bugs – something that is broken or behaves differently from the docs.
  - Feature requests – new capabilities or improvements.
- When reporting a bug, please include:
  - KubeNova version (Manager/Agent tags) and how you deployed it.
  - Kubernetes version and environment (kind, managed cloud, etc.).
  - Reproduction steps and expected vs. actual behavior.
  - Relevant logs or error messages (redacting sensitive data).

## Pull request guidelines

- Discuss larger changes in an issue before opening a PR when possible.
- Keep PRs focused and small; separate unrelated changes into separate PRs.
- Make sure the API, OpenAPI spec, and docs stay in sync:
  - Any change to `/api/v1` routes or models must update
    `docs/openapi/openapi.yaml` and the relevant handlers/tests in
    `internal/http`.
  - Update `docs/index.md` examples when behavior changes.
- Follow the existing code style:
  - Run `go fmt ./...` and `go vet ./...` before pushing.
  - Run `go test ./... -count=1` and fix any failing tests.
- Avoid introducing new top‑level folders without prior discussion.

## Development workflow

Recommended local commands:

- `make test-unit` – run the unit tests.
- `go fmt ./...` – format Go code.
- `go vet ./...` – basic static analysis.
- Optional:
  - `staticcheck ./...`
  - `gosec ./...`

If you modify deployment assets:

- Validate Helm charts with `helm lint deploy/helm/*`.
- Keep RBAC permissions minimal and focused on what the controllers need.

## Code of Conduct

By participating in this project, you agree to follow the guidelines described
in `CODE_OF_CONDUCT.md`.

