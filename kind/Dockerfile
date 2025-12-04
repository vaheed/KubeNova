FROM docker:27-cli

RUN apk add --no-cache ca-certificates curl bash && \
    curl -Lo /usr/local/bin/kind https://kind.sigs.k8s.io/dl/v0.23.0/kind-linux-amd64 && \
    chmod +x /usr/local/bin/kind && \
    curl -Lo /usr/local/bin/kubectl https://dl.k8s.io/release/v1.30.0/bin/linux/amd64/kubectl && \
    chmod +x /usr/local/bin/kubectl

COPY kind-config.yaml /kind-config.yaml
COPY metallb-config.yaml /metallb-config.yaml
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]