# This Dockerfile is designed for multi-architecture builds using `docker buildx`.
FROM ubuntu:24.04 AS builder

# GPUD_VERSION is a mandatory build argument for the GPUd version to install.
# Example: --build-arg GPUD_VERSION=v0.6.0
ARG GPUD_VERSION
# TARGETARCH is an automatic build argument provided by buildx (e.g., amd64, arm64).
ARG TARGETARCH

RUN if [ -z "$GPUD_VERSION" ]; then \
        echo "\n\nERROR: The build argument 'GPUD_VERSION' is required.\n" >&2; \
        echo "Please provide it using: --build-arg GPUD_VERSION=<version>\n" >&2; \
        exit 1; \
    fi

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      curl && \
    rm -rf /var/lib/apt/lists/* && \
    URL="https://github.com/leptonai/gpud/releases/download/${GPUD_VERSION}/gpud_${GPUD_VERSION}_linux_${TARGETARCH}_ubuntu24.04.tgz" && \
    curl -L "${URL}" | tar -xz -C /usr/local/bin/ gpud

FROM debian:bookworm-slim

ARG GPUD_VERSION
ARG TARGETARCH

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      pciutils && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/bin/gpud /usr/local/bin/gpud

ENTRYPOINT ["/usr/local/bin/gpud"]
