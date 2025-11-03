#!/usr/bin/env bash
set -euo pipefail
helm upgrade --install manager ./deploy/helm/manager -n kubenova-system --create-namespace
kubectl -n kubenova-system rollout status deploy/kubenova-manager
