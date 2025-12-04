# Security Policy

## Supported versions

KubeNova is under active development. The latest `main` branch and the most
recent tagged releases are the primary focus for security fixes. Older
versions may not receive patches for newly discovered vulnerabilities.

## Reporting a vulnerability

If you believe you have found a security issue in KubeNova:

- Do **not** open a public GitHub issue for sensitive reports.
- Instead, use GitHub’s private security advisory workflow, or contact the
  maintainers through a private channel associated with the repository.
- Provide as much detail as possible:
  - A description of the issue and potential impact.
  - Steps to reproduce or proof‑of‑concept (if available).
  - Any relevant logs, configuration snippets, or environment details.

We aim to:

- Acknowledge security reports in a reasonable timeframe.
- Investigate, verify, and prioritize fixes.
- Coordinate disclosure timing with the reporter when appropriate.

## Security posture

- API authentication uses JWT (HS256) and role‑based access control.
- The project uses static analysis and scanning in CI (for example `gosec`
  and Trivy) to help identify high‑severity issues before release.
- Secrets and sensitive configuration should be managed via your platform’s
secret management facilities (for example, Kubernetes Secrets, cloud KMS, or
vault systems).

