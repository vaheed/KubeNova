#!/usr/bin/env bash
set -euo pipefail
helm upgrade --install kubenova-agent ./deploy/helm/kubenova-agent -n kubenova --create-namespace
kubectl -n kubenova rollout status deploy/kubenova-agent
