# How it Works

Sequence (Manager → Agent → Add-ons):
1. Manager receives cluster kubeconfig and installs the Agent Deployment (2 replicas) + HPA.
2. Agent starts with leader election and creates a Helm Job to install platform add-ons (tenancy, access proxy, and app delivery).
3. Agent verifies CRDs and controllers are Ready. It emits Events and pushes metrics.
4. Manager computes live conditions on GET /api/v1/clusters/{id}: AgentReady and AddonsReady.

See diagrams in `docs/diagrams`.
