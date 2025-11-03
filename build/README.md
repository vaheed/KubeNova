# build/

Container build assets.

- `Dockerfile.manager` – multi-stage build for the Manager service (distroless runtime).
- `Dockerfile.agent` – multi-stage build for the in-cluster Agent (distroless runtime).

Notes
- Images are built in CI and pushed to GHCR.
- E2E uses the Agent image locally and loads it into Kind.
- Keep base images up to date and prefer distroless for minimal attack surface.
