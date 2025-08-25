# This Dockerfile is designed for multi-architecture builds using `docker buildx`.

# Define build arguments for OS, version, and CUDA.
# Example: --build-arg OS_NAME=ubuntu --build-arg OS_VERSION=24.04
ARG OS_NAME="ubuntu"
ARG OS_VERSION="22.04"
ARG CUDA_VERSION="12.4.1"

FROM ${OS_NAME}:${OS_VERSION} AS builder

# GPUD_VERSION is a mandatory build argument for the GPUd version to install.
# Example: --build-arg GPUD_VERSION=v0.6.0
ARG GPUD_VERSION
# TARGETARCH is an automatic build argument provided by buildx (e.g., amd64, arm64).
ARG TARGETARCH
# Re-declare args to make them available inside this build stage.
ARG OS_NAME
ARG OS_VERSION

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
    URL="https://github.com/leptonai/gpud/releases/download/${GPUD_VERSION}/gpud_${GPUD_VERSION}_linux_${TARGETARCH}_${OS_NAME}${OS_VERSION}.tgz" && \
    curl -L "${URL}" | tar -xz -C /usr/local/bin/ gpud

# Use the NVIDIA CUDA runtime image as the final base. This provides the necessary
# CUDA libraries to interact with the GPU driver.
FROM nvidia/cuda:${CUDA_VERSION}-runtime-${OS_NAME}${OS_VERSION}

# Install required runtime dependencies not included in the NVIDIA CUDA runtime image.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      pciutils \
      dmidecode \
      util-linux \
      kmod \
      docker.io \
      containerd && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/bin/gpud /usr/local/bin/gpud

ENTRYPOINT ["/usr/local/bin/gpud"]
