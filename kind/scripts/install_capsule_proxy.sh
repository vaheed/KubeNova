#!/usr/bin/env bash
set -euo pipefail
helm repo add clastix https://clastix.github.io/charts >/dev/null || true
helm repo update >/dev/null
kubectl create ns capsule-system >/dev/null 2>&1 || true
helm upgrade --install capsule-proxy clastix/capsule-proxy -n capsule-system   --set service.enabled=true --set options.allowedUserGroups='{tenant-admins,tenant-maintainers}'
kubectl -n capsule-system rollout status deploy/capsule-proxy
