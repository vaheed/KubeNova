# KubeNova Agents Guide (Required Pre‑Commit Rules)

This repository uses agents to automate changes. Every agent MUST follow these rules before committing any change that touches files within this repo.

Scope: Entire repository unless a more specific AGENTS.md exists deeper in the tree.

## Pre‑Commit Checklist (Must Pass)
- Build and basic static analysis
  - `go fmt ./...` and ensure no diffs remain.
  - `go vet ./...` must pass.
  - `go build ./...` must succeed.
- Unit tests
  - `go test ./... -count=1` must pass.
  - When adding features or fixing bugs, add or update unit tests near the code changed.
- Security checks
  - `gosec ./...` must report no HIGH severity findings. Address or mark explicit, justified `// #nosec` with rationale.
  - Trivy repo config scan for IaC (Helm/manifests): run `trivy config --severity HIGH,CRITICAL --format table --exit-code 1 deploy` (no `--scanners` flag).
- API and docs
  - If API routes, request/response, or models change: update `docs/openapi.yaml` and any affected docs under `docs/site`.
  - Keep diagrams in `docs/diagrams` in sync when architecture or flows change.
- Helm and deploy artifacts
  - Lint charts you modified: `helm lint deploy/helm/*`.
  - Keep chart versions coherent with changes (dev builds may append `-dev`).
- Repository hygiene
  - No TODO stubs left in changed files.
  - No binary artifacts added to the repo (images/docs SVG are ok). Do not commit build outputs.
  - Follow least‑privilege for RBAC and keep CRDs/permissions minimal.
  - Each top‑level folder MUST contain a `README.md` explaining purpose and usage. When folder contents or conventions change, update the folder README in the same PR.
- Observability
  - Emit structured logs (zap), metrics, and traces for new components/paths.
  - For any new API or control‑plane flow, add or update E2E (Kind) smoke coverage to exercise the path, including resilience (manager/API down/up, retry/backoff, idempotency) where applicable.

## Coding Conventions
- Go 1.24.x; prefer stdlib first, then minimal dependencies.
- Keep changes minimal and focused on the task; avoid unrelated refactors.
- Idempotency: reconcilers and installers must tolerate re‑runs.
- Backoff with jitter for retries (see `internal/util/backoff.go`).
- Finalizers for cleanup of cluster resources.

## Commit/PR Policy
- Use conventional commit messages, e.g., `feat: …`, `fix: …`, `docs: …`, `chore: …`.
- After completing the checklist, agents MUST `git add -A` and `git commit -m "…"`. Do NOT push; opening PRs is handled outside this step.
- Reference modified files or components in the commit message body when relevant.

## Don’ts
- Don’t introduce new top‑level directories unless explicitly requested.
- Don’t disable CI checks to make a change pass.
- Don’t commit secrets or credentials. Use env/secret managers.

## Helpful Commands
- Unit: `make test-unit`
- E2E smoke (Kind): `make test-smoke`
- Docs: `make docs-build`

By committing, an agent asserts the checklist has been followed.
