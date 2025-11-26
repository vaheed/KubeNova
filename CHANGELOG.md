## 0.1.1

- Align container image and chart publishing with `ghcr.io/vaheed/kubenova/*` and `ghcr.io/vaheed/kubenova/charts/*`.
- Bundle `vela` CLI in the operator and reconcile velaux/fluxcd via addon enable/disable with periodic re-upgrades.
- Add uninstall path and per-component summary logs; logging now defaults to debug level.
- Remove unused agent image configuration from the manager chart; add capsule proxy public URL env placeholder.
- Harden CI workflows (Trivy, gosec, multi-arch buildx) and ensure chart pushes target the correct OCI repo.
