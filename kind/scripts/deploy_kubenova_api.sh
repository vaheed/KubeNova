#!/usr/bin/env bash
set -euo pipefail
helm upgrade --install kubenova-api ./deploy/helm/kubenova-api -n kubenova --create-namespace
kubectl -n kubenova rollout status deploy/kubenova-api
