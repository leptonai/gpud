# This Dockerfile is designed for multi-architecture builds using `docker buildx`.

# Define build arguments for OS, version, and CUDA.
# Example: --build-arg OS_NAME=ubuntu --build-arg OS_VERSION=24.04
ARG OS_NAME="ubuntu"
ARG OS_VERSION="22.04"
ARG CUDA_VERSION="12.4.1"

FROM golang:1.24.7 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Download dependencies as a separate step to take advantage of Docker's caching
RUN go mod download

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
  ca-certificates \
  curl \
  gnupg \
  pciutils \
  dmidecode \
  util-linux \
  kmod \
  sudo && \
  # Install Docker from official repository to get patched version (CVE-2024-41110)
  install -m 0755 -d /etc/apt/keyrings && \
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
  chmod a+r /etc/apt/keyrings/docker.gpg && \
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null && \
  apt-get update && \
  apt-get install -y --no-install-recommends docker-ce-cli containerd.io && \
  rm -rf /var/lib/apt/lists/*

COPY --from=builder /workspace/bin/gpud /usr/local/bin/gpud

ENTRYPOINT ["/usr/local/bin/gpud"]
