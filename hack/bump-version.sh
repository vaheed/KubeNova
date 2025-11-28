#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <version e.g. v0.1.3 or 0.1.3>" >&2
  exit 1
fi

input="$1"
if [[ "$input" =~ ^v?([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  plain="${BASH_REMATCH[1]}"
  new="v${plain}"
else
  echo "version must look like v0.1.3 or 0.1.3" >&2
  exit 1
fi

repo_root="$(cd -- "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

old=$(perl -ne 'if (/version\s*=\s*"(v?\d+\.\d+\.\d+)"/) { print $1; exit }' internal/manager/server.go)
if [[ -z "${old:-}" ]]; then
  echo "could not detect current version from internal/manager/server.go" >&2
  exit 1
fi
old_plain="${old#v}"

echo "bumping from ${old} to ${new} (${plain})"

v_files=(
  README.md
  ROADMAP.md
  docs/roadmap.md
  docs/index.md
  docs/deployment/overview.md
  docs/operations/observability.md
  docs/operations/kind-e2e.md
  docs/operations/upgrade.md
  docs/getting-started/api-playbook.md
  docs/reference/api.md
  deploy/README.md
  deploy/helm/operator/README.md
  env.example
  docs/reference/configuration.md
  kind/e2e.sh
  deploy/operator/deployment.yaml
  deploy/helm/manager/values.yaml
  deploy/helm/operator/values.yaml
  internal/manager/server.go
  internal/manager/e2e_test.go
  internal/manager/live_api_e2e_test.go
  internal/cluster/installer.go
  docs/openapi/openapi.yaml
)

plain_files=(
  deploy/helm/manager/Chart.yaml
  deploy/helm/operator/Chart.yaml
  package.json
  docs/openapi/openapi.yaml
  AGENTS.md
)

for f in "${v_files[@]}"; do
  if [[ -f "$f" ]]; then
    perl -pi -e 's/\Q'"${old}"'\E/'"${new}"'/g' "$f"
  fi
done

for f in "${plain_files[@]}"; do
  if [[ -f "$f" ]]; then
    perl -pi -e 's/\Q'"${old_plain}"'\E/'"${plain}"'/g' "$f"
  fi
done

echo "done. verify and commit:"
echo "  git diff"
