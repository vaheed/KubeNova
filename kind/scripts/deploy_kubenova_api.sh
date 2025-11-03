#!/usr/bin/env bash
set -euo pipefail
helm upgrade --install manager ./deploy/helm/manager -n kubenova --create-namespace
kubectl -n kubenova rollout status deploy/kubenova-manager
