# Dockerfile Guide

This document describes how to build, verify, and push the gpud Docker image.

## Overview

The Dockerfile uses a multi-stage build:

1. **Stage 1 (builder)**: Builds the gpud binary and vendors all Go dependencies
2. **Stage 2 (apt-sources)**: Downloads source packages for all APT dependencies
3. **Stage 3 (final)**: Creates the runtime image with third-party sources included

All third-party open source component sources are stored in
`/usr/share/third_party/` inside the container.

---

## Quick Start

### Build and Load Locally on Mac ARM

```bash
./scripts/build-docker.sh \
  --tag gpud:test \
  --platform linux/arm64 \
  --load
```

### Build linux/amd64 for Production Testing

Most production servers use `linux/amd64`. On Mac ARM, use `--skip-verify` since
amd64 containers cannot run natively:

```bash
./scripts/build-docker.sh \
  --tag gpud:test-amd64 \
  --platform linux/amd64 \
  --load \
  --skip-verify
```

### Build and Push to NVIDIA Registry

```bash
# Login to NVIDIA Container Registry
docker login nvcr.io -u '$oauthtoken' -p <NGC_API_KEY>

# Build and push multi-arch image
./scripts/build-docker.sh \
  --tag nvcr.io/nvstaging/dgx-cloud-lepton/gpud:v0.9.0-alpha.27 \
  --platform linux/amd64,linux/arm64 \
  --push
```

---

## Build Script Options

```text
./scripts/build-docker.sh [OPTIONS]

Options:
  --tag TAG           Docker image tag (required)
  --platform PLAT     Target platforms (default: linux/amd64,linux/arm64)
  --os-name NAME      Base OS name (default: ubuntu)
  --os-version VER    Base OS version (default: 22.04)
  --cuda-version VER  CUDA version (default: 12.4.1)
  --push              Push image to registry after build
  --load              Load image to local Docker (single platform only)
  --no-cache          Build without cache
  --skip-verify       Skip verification of third-party sources (verify is ON by default)
  --help              Show help message
```

### Additional Build Options

```bash
# Build with custom CUDA version
./scripts/build-docker.sh --tag gpud:cuda12.6 --cuda-version 12.6.0 --platform linux/arm64 --load

# Build without cache (clean build)
./scripts/build-docker.sh --tag gpud:fresh --platform linux/arm64 --load --no-cache

# Build for different Ubuntu version
./scripts/build-docker.sh --tag gpud:ubuntu24 --os-version 24.04 --platform linux/arm64 --load
```

---

## Verify Third-Party Sources in Container

After building with `--load`, run these commands to verify all third-party
source code is included.

**Note:** You must use `--entrypoint ""` to override the gpud entrypoint and run
shell commands directly.

### Quick Verification (All-in-One)

```bash
docker run --rm --entrypoint "" gpud:test sh -c '
echo "=== Third-Party Source Code Verification ==="
echo ""
echo "=== 1. Directory Structure ==="
ls -la /usr/share/third_party/
echo ""
echo "=== 2. Go Vendor Packages ==="
echo "Total Go packages: $(ls /usr/share/third_party/go/vendor/ | wc -l)"
ls /usr/share/third_party/go/vendor/
echo ""
echo "=== 3. APT Source Packages ==="
ls -la /usr/share/third_party/apt/
echo ""
echo "=== Verification Complete ==="
'
```

### Detailed Verification Commands

#### 1. List Third-Party Directory Structure

```bash
docker run --rm --entrypoint "" gpud:test ls -la /usr/share/third_party/
```

#### 2. Show Main Manifest

```bash
docker run --rm --entrypoint "" gpud:test cat /usr/share/third_party/MANIFEST.txt
```

---

### Verify Go Module Source Code

#### Count Go Vendor Packages

```bash
docker run --rm --entrypoint "" gpud:test ls /usr/share/third_party/go/vendor/ | wc -l
```

#### List All Go Vendor Packages

```bash
docker run --rm --entrypoint "" gpud:test ls /usr/share/third_party/go/vendor/
```

#### Show Go Modules Manifest (with versions)

```bash
docker run --rm --entrypoint "" gpud:test cat /usr/share/third_party/go/GO_MODULES.txt
```

#### Verify Specific Go Package Source Exists

```bash
# Check NVIDIA go-nvml source exists
docker run --rm --entrypoint "" gpud:test \
  ls -la /usr/share/third_party/go/vendor/github.com/NVIDIA/go-nvml/

# Check gin-gonic/gin source exists
docker run --rm --entrypoint "" gpud:test \
  ls -la /usr/share/third_party/go/vendor/github.com/gin-gonic/gin/

# Check prometheus client source exists
docker run --rm --entrypoint "" gpud:test \
  ls -la /usr/share/third_party/go/vendor/github.com/prometheus/client_golang/

# Check kubernetes API source exists
docker run --rm --entrypoint "" gpud:test \
  ls -la /usr/share/third_party/go/vendor/k8s.io/api/
```

#### List All Go Packages with Full Paths

```bash
docker run --rm --entrypoint "" gpud:test \
  find /usr/share/third_party/go/vendor -maxdepth 3 -type d | head -50
```

#### Verify Go Source Files Exist (not just directories)

```bash
docker run --rm --entrypoint "" gpud:test sh -c '
echo "=== Verifying Go source files exist ==="
echo ""
echo "NVIDIA go-nvml:"
ls /usr/share/third_party/go/vendor/github.com/NVIDIA/go-nvml/pkg/nvml/*.go \
  | head -5
echo ""
echo "gin-gonic/gin:"
ls /usr/share/third_party/go/vendor/github.com/gin-gonic/gin/*.go | head -5
echo ""
echo "prometheus/client_golang:"
ls /usr/share/third_party/go/vendor/github.com/prometheus/\
client_golang/prometheus/*.go | head -5
'
```

---

### Verify APT Package Source Code

#### List APT Source Files

```bash
docker run --rm --entrypoint "" gpud:test ls -la /usr/share/third_party/apt/
```

#### Show APT Sources Manifest

```bash
docker run --rm --entrypoint "" gpud:test cat /usr/share/third_party/apt/APT_SOURCES.txt
```

#### Count APT Source Archives

```bash
docker run --rm --entrypoint "" gpud:test sh -c '
echo "=== APT Source Package Count ==="
echo "Total source files: $(ls /usr/share/third_party/apt/ | wc -l)"
echo ""
echo "Tarball archives (.tar.*):"
ls /usr/share/third_party/apt/*.tar.* 2>/dev/null | wc -l
echo ""
echo "Debian source files (.dsc):"
ls /usr/share/third_party/apt/*.dsc 2>/dev/null | wc -l
'
```

#### Verify Specific APT Source Package Exists

```bash
# Check curl source exists
docker run --rm --entrypoint "" gpud:test ls /usr/share/third_party/apt/ | grep -i curl

# Check docker-cli source exists
docker run --rm --entrypoint "" gpud:test ls /usr/share/third_party/apt/ | grep -i docker

# Check containerd source exists
docker run --rm --entrypoint "" gpud:test ls /usr/share/third_party/apt/ | grep -i containerd

# Check all expected packages
docker run --rm --entrypoint "" gpud:test sh -c '
echo "=== Checking APT Source Packages ==="
pkgs="ca-certificates curl gnupg pciutils dmidecode"
pkgs="$pkgs util-linux kmod sudo docker containerd"
for pkg in $pkgs; do
  count=$(ls /usr/share/third_party/apt/ 2>/dev/null | grep -i "$pkg" | wc -l)
  if [ "$count" -gt 0 ]; then
    echo "[OK] $pkg: $count file(s)"
  else
    echo "[MISSING] $pkg: no source files found"
  fi
done
'
```

#### Extract and Inspect APT Source Code

APT sources are downloaded as compressed tarballs. To extract and view the
actual source code:

```bash
# List all APT source archives
docker run --rm --entrypoint "" gpud:test ls -la /usr/share/third_party/apt/

# Extract and view curl source code
docker run --rm --entrypoint "" gpud:test sh -c '
cd /tmp
cp /usr/share/third_party/apt/curl*.orig.tar.* . 2>/dev/null \
  || cp /usr/share/third_party/apt/curl*.tar.* .
tar -xf curl*.tar.* 2>/dev/null
echo "=== curl source directory ==="
ls -la curl*/
echo ""
echo "=== curl source files (first 10) ==="
find curl*/ -name "*.c" -o -name "*.h" | head -10
'

# Extract and view sudo source code
docker run --rm --entrypoint "" gpud:test sh -c '
cd /tmp
cp /usr/share/third_party/apt/sudo*.orig.tar.* . 2>/dev/null \
  || cp /usr/share/third_party/apt/sudo*.tar.* .
tar -xf sudo*.tar.* 2>/dev/null
echo "=== sudo source directory ==="
ls -la sudo*/
echo ""
echo "=== sudo source files (first 10) ==="
find sudo*/ -name "*.c" -o -name "*.h" | head -10
'

# Extract and view docker-cli source code (from GitHub tarball)
docker run --rm --entrypoint "" gpud:test sh -c '
cd /tmp
cp /usr/share/third_party/apt/docker*.tar.gz . 2>/dev/null
tar -xzf docker*.tar.gz 2>/dev/null
echo "=== docker-cli source directory ==="
ls -la cli*/ 2>/dev/null || ls -la docker*/
echo ""
echo "=== docker-cli source files (first 10) ==="
find . -name "*.go" 2>/dev/null | head -10
'

# Extract and view containerd source code (from GitHub tarball)
docker run --rm --entrypoint "" gpud:test sh -c '
cd /tmp
cp /usr/share/third_party/apt/containerd*.tar.gz . 2>/dev/null
tar -xzf containerd*.tar.gz 2>/dev/null
echo "=== containerd source directory ==="
ls -la containerd*/
echo ""
echo "=== containerd source files (first 10) ==="
find containerd*/ -name "*.go" | head -10
'
```

#### View APT Source Package Contents Without Extraction

```bash
# List contents of curl source tarball
docker run --rm --entrypoint "" gpud:test sh -c '
echo "=== Contents of curl source tarball ==="
tar -tvf /usr/share/third_party/apt/curl*.orig.tar.* 2>/dev/null | head -30
'

# List contents of util-linux source tarball
docker run --rm --entrypoint "" gpud:test sh -c '
echo "=== Contents of util-linux source tarball ==="
tar -tvf /usr/share/third_party/apt/util-linux*.orig.tar.* 2>/dev/null | head -30
'

# List contents of all source tarballs
docker run --rm --entrypoint "" gpud:test sh -c '
echo "=== All APT Source Tarballs ==="
for f in /usr/share/third_party/apt/*.tar.*; do
  echo ""
  echo "--- $f ---"
  tar -tvf "$f" 2>/dev/null | head -10
done
'
```

---

### Full Compliance Verification Script

Run this comprehensive verification to ensure all sources are present:

```bash
docker run --rm --entrypoint "" gpud:test sh -c '
echo "=============================================="
echo "  Third-Party Source Code Compliance Check"
echo "=============================================="
echo ""

echo "=== 1. Directory Structure ==="
ls -la /usr/share/third_party/
echo ""

echo "=== 2. Main Manifest ==="
cat /usr/share/third_party/MANIFEST.txt
echo ""

echo "=== 3. Go Module Sources ==="
echo "Total Go packages in vendor/: $(ls /usr/share/third_party/go/vendor/ | wc -l)"
echo ""
echo "Key Go packages verification:"
for pkg in "github.com/NVIDIA/go-nvml" "github.com/NVIDIA/go-nvlib" \
           "github.com/gin-gonic/gin" "github.com/prometheus/client_golang" \
           "k8s.io/api"; do
  if [ -d "/usr/share/third_party/go/vendor/$pkg" ]; then
    echo "[OK] $pkg"
  else
    echo "[MISSING] $pkg"
  fi
done
echo ""

echo "=== 4. Go Modules Manifest ==="
head -40 /usr/share/third_party/go/GO_MODULES.txt
echo ""

echo "=== 5. APT Package Sources ==="
ls -la /usr/share/third_party/apt/
echo ""

echo "=== 6. APT Sources Manifest ==="
cat /usr/share/third_party/apt/APT_SOURCES.txt
echo ""

echo "=== 7. APT Source Package Verification ==="
pkgs="ca-certificates curl gnupg pciutils dmidecode"
pkgs="$pkgs util-linux kmod sudo docker containerd"
for pkg in $pkgs; do
  count=$(ls /usr/share/third_party/apt/ 2>/dev/null | grep -i "$pkg" | wc -l)
  if [ "$count" -gt 0 ]; then
    echo "[OK] $pkg: $count file(s)"
  else
    echo "[MISSING] $pkg"
  fi
done
echo ""

echo "=== 8. APT Source Tarball Contents Sample ==="
for f in /usr/share/third_party/apt/*.orig.tar.* \
         /usr/share/third_party/apt/*.tar.gz; do
  if [ -f "$f" ]; then
    echo ""
    echo "--- $(basename $f) ---"
    tar -tvf "$f" 2>/dev/null | head -5
  fi
done 2>/dev/null
echo ""

echo "=============================================="
echo "  Verification Complete"
echo "=============================================="
'
```

---

## Third-Party Sources Location

The container includes all third-party source code at:

```text
/usr/share/third_party/
  MANIFEST.txt        - Main manifest file
  go/
    vendor/           - Go module source code (~100+ packages)
      github.com/
        NVIDIA/
          go-nvml/    - NVIDIA NVML Go bindings
          go-nvlib/   - NVIDIA Go library
        gin-gonic/
          gin/        - Gin web framework
        prometheus/
          client_golang/  - Prometheus client
        ...
      k8s.io/
        api/          - Kubernetes API
        apimachinery/ - Kubernetes API machinery
        ...
      golang.org/
        x/
          crypto/     - Go crypto packages
          sys/        - Go sys packages
      ...
    GO_MODULES.txt    - Go modules manifest with versions
  apt/
    *.orig.tar.*      - APT original source tarballs
    *.dsc             - APT source description files
    *.debian.tar.*    - Debian packaging files
    APT_SOURCES.txt   - APT sources manifest
```

### Go Dependencies Included

All Go modules from `go.mod` are vendored, including:

- `github.com/NVIDIA/go-nvml` - NVIDIA NVML bindings
- `github.com/NVIDIA/go-nvlib` - NVIDIA Go library
- `github.com/gin-gonic/gin` - HTTP web framework
- `github.com/prometheus/client_golang` - Prometheus metrics
- `k8s.io/api` - Kubernetes API types
- `k8s.io/apimachinery` - Kubernetes API machinery
- `golang.org/x/crypto` - Go crypto packages
- `golang.org/x/sys` - Go system packages
- `google.golang.org/grpc` - gRPC framework
- Plus ~100 more transitive dependencies

### APT Dependencies Included

Source packages for all installed APT packages:

- `ca-certificates` - CA certificates
- `curl` - URL transfer tool
- `gnupg` - GNU Privacy Guard
- `pciutils` - PCI utilities
- `dmidecode` - DMI table decoder
- `util-linux` - Linux utilities
- `kmod` - Kernel module tools
- `sudo` - Superuser do
- `docker-ce-cli` - Docker CLI
- `containerd.io` - Container runtime

---

## Manual Docker Build (without script)

If you prefer to build manually without the script:

```bash
# Multi-arch build for production
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg OS_NAME=ubuntu \
  --build-arg OS_VERSION=22.04 \
  --build-arg CUDA_VERSION=12.4.1 \
  -t nvcr.io/nvstaging/dgx-cloud-lepton/gpud:v0.9.0-alpha.27 \
  -f Dockerfile \
  . \
  --push
```

For local testing on Mac ARM (M1/M2/M3):

```bash
docker buildx build \
  --platform linux/arm64 \
  --build-arg OS_NAME=ubuntu \
  --build-arg OS_VERSION=22.04 \
  --build-arg CUDA_VERSION=12.4.1 \
  -t gpud:test \
  -f Dockerfile \
  . \
  --load
```

For linux/amd64 production testing (most common server architecture):

```bash
# Note: On Mac ARM, the resulting image cannot be run locally for verification
docker buildx build \
  --platform linux/amd64 \
  --build-arg OS_NAME=ubuntu \
  --build-arg OS_VERSION=22.04 \
  --build-arg CUDA_VERSION=12.4.1 \
  -t gpud:test-amd64 \
  -f Dockerfile \
  . \
  --load
```
