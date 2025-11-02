#!/usr/bin/env bash
set -euo pipefail
helm repo add kubevela https://kubevela.github.io/charts >/dev/null || true
helm repo update >/dev/null
kubectl create ns vela-system >/dev/null 2>&1 || true
helm upgrade --install vela-core kubevela/vela-core -n vela-system --set admissionWebhooks.enabled=true
kubectl -n vela-system rollout status deploy/vela-core
