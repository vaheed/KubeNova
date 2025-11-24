# Helm charts for bootstrap

The operator installs cert-manager, Capsule, Capsule Proxy, and KubeVela. You can supply charts in two ways:

1) **Bake charts into the operator image** at `/charts`
```
# in the operator Dockerfile
RUN apk add --no-cache helm
RUN mkdir -p /charts && \
    helm repo add jetstack https://charts.jetstack.io && \
    helm repo add clastix https://clastix.github.io/charts && \
    helm repo add kubevela https://kubevela.github.io/charts && \
    helm fetch jetstack/cert-manager --version v1.14.4 -d /charts && \
    helm fetch clastix/capsule --version 0.5.0 -d /charts && \
    helm fetch clastix/capsule-proxy --version 0.3.1 -d /charts && \
    helm fetch kubevela/vela-core --version 1.9.11 -d /charts
ENV HELM_CHARTS_DIR=/charts
```

2) **Use remote repos at runtime** by setting `HELM_USE_REMOTE=true` (charts are pulled from the repos above).

The `deploy/operator/deployment.yaml` expects charts at `/charts`; adjust `HELM_CHARTS_DIR` if you change the path.
