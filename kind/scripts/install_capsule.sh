#!/usr/bin/env bash
set -euo pipefail
helm repo add clastix https://clastix.github.io/charts >/dev/null || true
helm repo update >/dev/null
kubectl create ns capsule-system >/dev/null 2>&1 || true
helm upgrade --install capsule clastix/capsule -n capsule-system --set manager.leaderElection=true
kubectl -n capsule-system rollout status deploy/capsule-controller-manager
