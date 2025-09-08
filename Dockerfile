# This Dockerfile is designed for multi-architecture builds using `docker buildx`.

# Define build arguments for OS, version, and CUDA.
# Example: --build-arg OS_NAME=ubuntu --build-arg OS_VERSION=24.04
ARG OS_NAME="ubuntu"
ARG OS_VERSION="22.04"
ARG CUDA_VERSION="12.4.1"

FROM golang:1.24.5 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

COPY api/ api/
COPY client/ client/
COPY cmd/ cmd/
COPY components/ components/
COPY docs/ docs/
COPY pkg/ pkg/
COPY version/ version/
COPY Makefile Makefile

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} make

# Use the NVIDIA CUDA runtime image as the final base. This provides the necessary
# CUDA libraries to interact with the GPU driver.
FROM nvidia/cuda:${CUDA_VERSION}-runtime-${OS_NAME}${OS_VERSION}
WORKDIR /

# Install required runtime dependencies not included in the NVIDIA CUDA runtime image.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      pciutils \
      dmidecode \
      util-linux \
      kmod \
      docker.io \
      containerd \
      sudo && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /workspace/bin/gpud /usr/local/bin/gpud

ENTRYPOINT ["/usr/local/bin/gpud"]
