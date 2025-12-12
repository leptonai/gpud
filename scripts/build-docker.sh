#!/bin/bash
#
# build-docker.sh
#
# Build script for gpud Docker image with third-party source code included.
#
# This script wraps docker buildx to build the Dockerfile with proper
# configuration for multi-arch builds and registry push.
#
# ==============================================================================
# USAGE
# ==============================================================================
#
#   ./scripts/build-docker.sh [OPTIONS]
#
# ==============================================================================
# OPTIONS
# ==============================================================================
#
#   --tag TAG           Docker image tag (required)
#   --platform PLAT     Target platforms (default: linux/amd64,linux/arm64)
#   --os-name NAME      Base OS name (default: ubuntu)
#   --os-version VER    Base OS version (default: 22.04)
#   --cuda-version VER  CUDA version (default: 12.4.1)
#   --push              Push image to registry after build
#   --load              Load image to local Docker (single platform only)
#   --no-cache          Build without cache
#   --skip-verify       Skip verification of third-party sources (verify is ON by default)
#   --help              Show this help message
#
# ==============================================================================
# EXAMPLES
# ==============================================================================
#
# 1. Build and push to NVIDIA registry (multi-arch, for production):
#
#    docker login nvcr.io -u '$oauthtoken' -p <NGC_API_KEY>
#
#    ./scripts/build-docker.sh \
#      --tag nvcr.io/nvstaging/dgx-cloud-lepton/gpud:v0.9.0-alpha.27 \
#      --platform linux/amd64,linux/arm64 \
#      --push
#
# 2. Build and load locally for testing on Mac ARM (M1/M2/M3):
#
#    ./scripts/build-docker.sh \
#      --tag gpud:test \
#      --platform linux/arm64 \
#      --load
#
# 3. Build linux/amd64 for production testing (most common server architecture):
#
#    # Note: On Mac ARM, use --skip-verify since amd64 containers can't run natively
#    ./scripts/build-docker.sh \
#      --tag gpud:test-amd64 \
#      --platform linux/amd64 \
#      --load \
#      --skip-verify
#
# 4. Build with custom CUDA version:
#
#    ./scripts/build-docker.sh \
#      --tag gpud:cuda12.6 \
#      --cuda-version 12.6.0 \
#      --platform linux/arm64 \
#      --load
#
# 5. Build without cache (clean build):
#
#    ./scripts/build-docker.sh \
#      --tag gpud:fresh \
#      --platform linux/arm64 \
#      --load \
#      --no-cache
#
# 6. Build for different Ubuntu version:
#
#    ./scripts/build-docker.sh \
#      --tag gpud:ubuntu24 \
#      --os-version 24.04 \
#      --platform linux/arm64 \
#      --load
#
# ==============================================================================
# VERIFICATION COMMANDS (run after build with --load)
# ==============================================================================
#
# After building with --load, you can manually verify third-party sources:
#
#    # List third-party directory structure
#    docker run --rm <TAG> ls -la /usr/share/third_party/
#
#    # Show main manifest
#    docker run --rm <TAG> cat /usr/share/third_party/MANIFEST.txt
#
#    # Count Go vendor packages
#    docker run --rm <TAG> ls /usr/share/third_party/go/vendor/ | wc -l
#
#    # List Go vendor packages
#    docker run --rm <TAG> ls /usr/share/third_party/go/vendor/
#
#    # Show Go modules manifest
#    docker run --rm <TAG> cat /usr/share/third_party/go/GO_MODULES.txt
#
#    # List APT source files
#    docker run --rm <TAG> ls -la /usr/share/third_party/apt/
#
#    # Show APT sources manifest
#    docker run --rm <TAG> cat /usr/share/third_party/apt/APT_SOURCES.txt
#
# ==============================================================================
# THIRD-PARTY SOURCES LOCATION IN CONTAINER
# ==============================================================================
#
#    /usr/share/third_party/
#      MANIFEST.txt        - Main manifest file
#      go/
#        vendor/           - Go module source code (~100+ packages)
#        GO_MODULES.txt    - Go modules manifest with versions
#      apt/
#        *.orig.tar.*      - APT source tarballs
#        *.dsc             - APT source description files
#        *.debian.tar.*    - Debian packaging files
#        APT_SOURCES.txt   - APT sources manifest
#
# ==============================================================================

set -euo pipefail

# Default values
PLATFORMS="linux/amd64,linux/arm64"
OS_NAME="ubuntu"
OS_VERSION="22.04"
CUDA_VERSION="12.4.1"
TAG=""
PUSH=false
LOAD=false
NO_CACHE=false
VERIFY=true  # Enabled by default

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

show_help() {
    head -100 "$0" | grep "^#" | sed 's/^#//' | sed 's/^ //'
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --tag)
            TAG="$2"
            shift 2
            ;;
        --platform)
            PLATFORMS="$2"
            shift 2
            ;;
        --os-name)
            OS_NAME="$2"
            shift 2
            ;;
        --os-version)
            OS_VERSION="$2"
            shift 2
            ;;
        --cuda-version)
            CUDA_VERSION="$2"
            shift 2
            ;;
        --push)
            PUSH=true
            shift
            ;;
        --load)
            LOAD=true
            shift
            ;;
        --no-cache)
            NO_CACHE=true
            shift
            ;;
        --skip-verify)
            VERIFY=false
            shift
            ;;
        --help|-h)
            show_help
            ;;
        *)
            log_error "Unknown option: $1"
            show_help
            ;;
    esac
done

# Validate required arguments
if [[ -z "$TAG" ]]; then
    log_error "Docker image tag is required. Use --tag to specify."
    exit 1
fi

# Check for Dockerfile
if [[ ! -f "$PROJECT_ROOT/Dockerfile" ]]; then
    log_error "Dockerfile not found in $PROJECT_ROOT"
    exit 1
fi

# Check if docker buildx is available
if ! docker buildx version &>/dev/null; then
    log_error "docker buildx is required but not available"
    exit 1
fi

# If verify is enabled but load is not, we need load for single platform
if [[ "$VERIFY" == "true" && "$LOAD" != "true" && "$PUSH" != "true" ]]; then
    log_warn "Verification requires --load. Enabling --load automatically."
    LOAD=true
fi

# Adjust platform for --load
if [[ "$LOAD" == "true" && "$PLATFORMS" == *","* ]]; then
    log_warn "--load only works with single platform. Using linux/amd64 only."
    PLATFORMS="linux/amd64"
fi

# Build the command
BUILD_CMD="docker buildx build"

# Add platform
BUILD_CMD="$BUILD_CMD --platform $PLATFORMS"

# Add build args
BUILD_CMD="$BUILD_CMD --build-arg OS_NAME=$OS_NAME"
BUILD_CMD="$BUILD_CMD --build-arg OS_VERSION=$OS_VERSION"
BUILD_CMD="$BUILD_CMD --build-arg CUDA_VERSION=$CUDA_VERSION"

# Add tag
BUILD_CMD="$BUILD_CMD -t $TAG"

# Add Dockerfile
BUILD_CMD="$BUILD_CMD -f $PROJECT_ROOT/Dockerfile"

# Add optional flags
if [[ "$PUSH" == "true" ]]; then
    BUILD_CMD="$BUILD_CMD --push"
fi

if [[ "$LOAD" == "true" ]]; then
    BUILD_CMD="$BUILD_CMD --load"
fi

if [[ "$NO_CACHE" == "true" ]]; then
    BUILD_CMD="$BUILD_CMD --no-cache"
fi

# Add context
BUILD_CMD="$BUILD_CMD $PROJECT_ROOT"

# Print build information
echo ""
log_info "========================================"
log_info "GPUd Docker Build"
log_info "========================================"
log_info "Tag:          $TAG"
log_info "Platforms:    $PLATFORMS"
log_info "OS:           $OS_NAME $OS_VERSION"
log_info "CUDA Version: $CUDA_VERSION"
log_info "Push:         $PUSH"
log_info "Load:         $LOAD"
log_info "Verify:       $VERIFY"
log_info "No Cache:     $NO_CACHE"
log_info "========================================"
echo ""

log_step "Building Docker image..."
echo ""
log_info "Command: $BUILD_CMD"
echo ""

# Execute build
eval "$BUILD_CMD"

BUILD_STATUS=$?

if [[ $BUILD_STATUS -ne 0 ]]; then
    log_error "Build failed with exit code $BUILD_STATUS"
    exit $BUILD_STATUS
fi

echo ""
log_info "========================================"
log_info "Build completed successfully!"
log_info "========================================"
echo ""

# Verify third-party sources if requested and image is loaded locally
# Note: Must use --entrypoint "" to override the gpud entrypoint for shell commands
if [[ "$VERIFY" == "true" && "$LOAD" == "true" ]]; then
    echo ""
    log_step "Verifying third-party sources in container..."
    echo ""

    log_info "Checking /usr/share/third_party/ directory structure:"
    echo ""
    docker run --rm --entrypoint "" "$TAG" ls -la /usr/share/third_party/
    echo ""

    log_info "Checking Go vendor directory (first 20 entries):"
    echo ""
    docker run --rm --entrypoint "" "$TAG" ls /usr/share/third_party/go/vendor/ | head -20
    GO_VENDOR_COUNT=$(docker run --rm --entrypoint "" "$TAG" ls /usr/share/third_party/go/vendor/ | wc -l)
    echo "... ($GO_VENDOR_COUNT total Go packages)"
    echo ""

    log_info "Checking APT source packages:"
    echo ""
    docker run --rm --entrypoint "" "$TAG" ls -la /usr/share/third_party/apt/
    echo ""

    log_info "Manifest file content:"
    echo ""
    docker run --rm --entrypoint "" "$TAG" cat /usr/share/third_party/MANIFEST.txt
    echo ""

    log_info "Go modules manifest (first 30 lines):"
    echo ""
    docker run --rm --entrypoint "" "$TAG" head -30 /usr/share/third_party/go/GO_MODULES.txt
    echo ""

    log_info "APT sources manifest:"
    echo ""
    docker run --rm --entrypoint "" "$TAG" cat /usr/share/third_party/apt/APT_SOURCES.txt
    echo ""

    log_info "========================================"
    log_info "Verification complete!"
    log_info "========================================"
elif [[ "$VERIFY" == "true" && "$PUSH" == "true" && "$LOAD" != "true" ]]; then
    log_warn "Skipping verification: image was pushed but not loaded locally."
    log_info "To verify, pull the image and run:"
    echo ""
    echo "  docker run --rm --entrypoint \"\" $TAG ls -la /usr/share/third_party/"
    echo "  docker run --rm --entrypoint \"\" $TAG cat /usr/share/third_party/MANIFEST.txt"
    echo ""
fi

# Print summary
echo ""
log_info "Image: $TAG"
echo ""
log_info "Third-party sources location: /usr/share/third_party/"
log_info "  go/vendor/          - Go module source code"
log_info "  go/GO_MODULES.txt   - Go module manifest"
log_info "  apt/                - APT package sources"
log_info "  apt/APT_SOURCES.txt - APT sources manifest"
log_info "  MANIFEST.txt        - Main manifest file"
echo ""

if [[ "$PUSH" == "true" ]]; then
    log_info "Image has been pushed to registry."
fi

if [[ "$LOAD" == "true" ]]; then
    echo ""
    log_info "Manual verification commands (use --entrypoint \"\" to run shell commands):"
    echo ""
    echo "  docker run --rm --entrypoint \"\" $TAG ls -la /usr/share/third_party/"
    echo "  docker run --rm --entrypoint \"\" $TAG cat /usr/share/third_party/MANIFEST.txt"
    echo "  docker run --rm --entrypoint \"\" $TAG ls /usr/share/third_party/go/vendor/ | wc -l"
    echo "  docker run --rm --entrypoint \"\" $TAG ls -la /usr/share/third_party/apt/"
    echo ""
fi
