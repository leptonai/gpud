# This Dockerfile is designed for multi-architecture builds using `docker buildx`.
# Use scripts/build-docker.sh to build this image.
#
# This image includes source code for all third-party open source components
# in /usr/share/third_party/ for compliance requirements.

# Define build arguments for OS, version, and CUDA.
# Example: --build-arg OS_NAME=ubuntu --build-arg OS_VERSION=24.04
ARG OS_NAME="ubuntu"
ARG OS_VERSION="22.04"
ARG CUDA_VERSION="12.4.1"

# ==============================================================================
# Stage 1: Builder - Build gpud binary and vendor Go dependencies
# ==============================================================================
FROM golang:1.24.7 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Download dependencies to module cache (for caching)
RUN go mod download

# Copy source code BEFORE vendoring so Go can analyze imports
COPY api/ api/
COPY client/ client/
COPY cmd/ cmd/
COPY components/ components/
COPY docs/ docs/
COPY pkg/ pkg/
COPY version/ version/
COPY Makefile Makefile

# Create vendor directory with all source code (for compliance)
# Must be after source code copy so Go can analyze imports
RUN go mod vendor

# Build the binary
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} make

# Generate Go module manifest with versions
RUN echo "# Go Module Dependencies" > /workspace/GO_MODULES.txt && \
    echo "# Generated at $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> /workspace/GO_MODULES.txt && \
    echo "" >> /workspace/GO_MODULES.txt && \
    go list -m -json all 2>/dev/null | \
    grep -E '"(Path|Version|Dir)"' | \
    sed 's/[",]//g' | \
    paste - - - | \
    awk '{print $2, $4}' >> /workspace/GO_MODULES.txt || true

# ==============================================================================
# Stage 2: APT Sources - Download source packages for all apt dependencies
# ==============================================================================
FROM ${OS_NAME}:${OS_VERSION} AS apt-sources

# Avoid interactive prompts
ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /apt-sources

# Enable source repositories and install required tools
RUN sed -i 's/^# deb-src/deb-src/' /etc/apt/sources.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
        dpkg-dev \
        ca-certificates \
        curl \
        gnupg

# Add Docker's official GPG key and source repository
RUN install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
    chmod a+r /etc/apt/keyrings/docker.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
    $(. /etc/os-release && echo "$VERSION_CODENAME") stable" > /etc/apt/sources.list.d/docker.list && \
    echo "deb-src [signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
    $(. /etc/os-release && echo "$VERSION_CODENAME") stable" >> /etc/apt/sources.list.d/docker.list && \
    apt-get update

# Download source packages for all runtime dependencies
# Note: Some packages may not have source available via apt-get source
RUN mkdir -p /apt-sources/packages && cd /apt-sources/packages && \
    # Standard Ubuntu packages
    apt-get source --download-only ca-certificates || true && \
    apt-get source --download-only curl || true && \
    apt-get source --download-only gnupg2 || true && \
    apt-get source --download-only pciutils || true && \
    apt-get source --download-only dmidecode || true && \
    apt-get source --download-only util-linux || true && \
    apt-get source --download-only kmod || true && \
    apt-get source --download-only sudo || true

# Download Docker package sources
# First try apt-get source, then fall back to GitHub with matching versions
RUN cd /apt-sources/packages && \
    # Get docker-ce-cli source - try apt first, fallback to GitHub
    (apt-get source --download-only docker-ce-cli 2>/dev/null || \
     (echo "docker-ce-cli source not available via apt, downloading from GitHub..." && \
      DOCKER_VERSION=$(apt-cache policy docker-ce-cli 2>/dev/null | grep Candidate | awk '{print $2}' | sed 's/.*://' | cut -d'-' -f1) && \
      curl -fsSL -o docker-cli-v${DOCKER_VERSION}-source.tar.gz https://github.com/docker/cli/archive/refs/tags/v${DOCKER_VERSION}.tar.gz)) && \
    # Get containerd.io source - try apt first, fallback to GitHub
    (apt-get source --download-only containerd.io 2>/dev/null || \
     (echo "containerd.io source not available via apt, downloading from GitHub..." && \
      CONTAINERD_VERSION=$(apt-cache policy containerd.io 2>/dev/null | grep Candidate | awk '{print $2}' | sed 's/.*://' | cut -d'-' -f1) && \
      curl -fsSL -o containerd-v${CONTAINERD_VERSION}-source.tar.gz https://github.com/containerd/containerd/archive/refs/tags/v${CONTAINERD_VERSION}.tar.gz))

# Generate manifest of downloaded sources
RUN echo "# APT Package Sources" > /apt-sources/APT_SOURCES.txt && \
    echo "# Generated at $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> /apt-sources/APT_SOURCES.txt && \
    echo "" >> /apt-sources/APT_SOURCES.txt && \
    echo "## Downloaded source packages:" >> /apt-sources/APT_SOURCES.txt && \
    ls -la /apt-sources/packages/ >> /apt-sources/APT_SOURCES.txt && \
    echo "" >> /apt-sources/APT_SOURCES.txt && \
    echo "## Package versions installed in final image:" >> /apt-sources/APT_SOURCES.txt && \
    echo "ca-certificates: $(apt-cache policy ca-certificates | grep Candidate | awk '{print $2}')" >> /apt-sources/APT_SOURCES.txt && \
    echo "curl: $(apt-cache policy curl | grep Candidate | awk '{print $2}')" >> /apt-sources/APT_SOURCES.txt && \
    echo "gnupg: $(apt-cache policy gnupg | grep Candidate | awk '{print $2}')" >> /apt-sources/APT_SOURCES.txt && \
    echo "pciutils: $(apt-cache policy pciutils | grep Candidate | awk '{print $2}')" >> /apt-sources/APT_SOURCES.txt && \
    echo "dmidecode: $(apt-cache policy dmidecode | grep Candidate | awk '{print $2}')" >> /apt-sources/APT_SOURCES.txt && \
    echo "util-linux: $(apt-cache policy util-linux | grep Candidate | awk '{print $2}')" >> /apt-sources/APT_SOURCES.txt && \
    echo "kmod: $(apt-cache policy kmod | grep Candidate | awk '{print $2}')" >> /apt-sources/APT_SOURCES.txt && \
    echo "sudo: $(apt-cache policy sudo | grep Candidate | awk '{print $2}')" >> /apt-sources/APT_SOURCES.txt && \
    echo "docker-ce-cli: $(apt-cache policy docker-ce-cli | grep Candidate | awk '{print $2}')" >> /apt-sources/APT_SOURCES.txt && \
    echo "containerd.io: $(apt-cache policy containerd.io | grep Candidate | awk '{print $2}')" >> /apt-sources/APT_SOURCES.txt

# ==============================================================================
# Stage 3: Final Runtime Image
# ==============================================================================
# Use the NVIDIA CUDA runtime image as the final base. This provides the necessary
# CUDA libraries to interact with the GPU driver.
FROM nvidia/cuda:${CUDA_VERSION}-runtime-${OS_NAME}${OS_VERSION}
WORKDIR /

# Install required runtime dependencies not included in the NVIDIA CUDA runtime image.
# NOTE: gnupg is installed temporarily for Docker GPG key verification, then purged
# to address CVE-2025-68973 (out-of-bounds write in GnuPG armor_filter before 2.4.9)
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
  # Remove gnupg and related packages to address CVE-2025-68973
  # These are only needed for GPG key verification during build, not at runtime
  apt-get purge -y --auto-remove gnupg gnupg-l10n gnupg-utils gpg gpg-agent gpg-wks-client gpg-wks-server gpgconf gpgsm dirmngr && \
  rm -rf /var/lib/apt/lists/*

# Copy the gpud binary
COPY --from=builder /workspace/bin/gpud /usr/local/bin/gpud

# ==============================================================================
# Third-Party Source Code
# ==============================================================================
# Create third_party directory structure
RUN mkdir -p /usr/share/third_party/go \
             /usr/share/third_party/apt

# Copy Go module source code (vendored dependencies)
COPY --from=builder /workspace/vendor /usr/share/third_party/go/vendor
COPY --from=builder /workspace/GO_MODULES.txt /usr/share/third_party/go/

# Copy APT package sources
COPY --from=apt-sources /apt-sources/packages /usr/share/third_party/apt/
COPY --from=apt-sources /apt-sources/APT_SOURCES.txt /usr/share/third_party/apt/

# Generate main manifest file
RUN echo "# Third-Party Open Source Components" > /usr/share/third_party/MANIFEST.txt && \
    echo "# Source Code Inclusion for Compliance" >> /usr/share/third_party/MANIFEST.txt && \
    echo "# Generated at $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> /usr/share/third_party/MANIFEST.txt && \
    echo "" >> /usr/share/third_party/MANIFEST.txt && \
    echo "This container includes source code for all third-party open source" >> /usr/share/third_party/MANIFEST.txt && \
    echo "components as required for compliance." >> /usr/share/third_party/MANIFEST.txt && \
    echo "" >> /usr/share/third_party/MANIFEST.txt && \
    echo "## Directory Structure" >> /usr/share/third_party/MANIFEST.txt && \
    echo "" >> /usr/share/third_party/MANIFEST.txt && \
    echo "/usr/share/third_party/" >> /usr/share/third_party/MANIFEST.txt && \
    echo "  go/           - Go module dependencies (source code)" >> /usr/share/third_party/MANIFEST.txt && \
    echo "    vendor/     - Vendored Go packages" >> /usr/share/third_party/MANIFEST.txt && \
    echo "    GO_MODULES.txt - List of Go modules with versions" >> /usr/share/third_party/MANIFEST.txt && \
    echo "  apt/          - APT package sources" >> /usr/share/third_party/MANIFEST.txt && \
    echo "    APT_SOURCES.txt - List of APT packages with versions" >> /usr/share/third_party/MANIFEST.txt && \
    echo "  MANIFEST.txt  - This file" >> /usr/share/third_party/MANIFEST.txt && \
    echo "" >> /usr/share/third_party/MANIFEST.txt && \
    echo "## Go Dependencies" >> /usr/share/third_party/MANIFEST.txt && \
    echo "See go/GO_MODULES.txt for complete list" >> /usr/share/third_party/MANIFEST.txt && \
    echo "" >> /usr/share/third_party/MANIFEST.txt && \
    echo "## APT Dependencies" >> /usr/share/third_party/MANIFEST.txt && \
    echo "See apt/APT_SOURCES.txt for complete list" >> /usr/share/third_party/MANIFEST.txt && \
    echo "" >> /usr/share/third_party/MANIFEST.txt && \
    echo "## Base Image" >> /usr/share/third_party/MANIFEST.txt && \
    echo "This image is based on nvidia/cuda runtime image." >> /usr/share/third_party/MANIFEST.txt && \
    echo "CUDA base image sources are managed separately by NVIDIA." >> /usr/share/third_party/MANIFEST.txt

ENTRYPOINT ["/usr/local/bin/gpud"]
