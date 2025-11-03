#!/usr/bin/env bash
set -euo pipefail
helm upgrade --install kubenova-agent ./deploy/helm/kubenova-agent -n kubenova-system --create-namespace
kubectl -n kubenova-system rollout status deploy/kubenova-agent
