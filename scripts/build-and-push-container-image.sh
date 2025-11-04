#!/usr/bin/env bash
#
# Build and push the GPUd container image to nvcr.io using Docker.
#
# The target repository must already exist in NGC; create/check it with:
#   ngc registry image list --org "${NVCR_ORG}"
#   ngc registry image create --org "${NVCR_ORG}" --name "${IMAGE_NAME}" --visibility public
#   ngc registry image info --org "${NVCR_ORG}" --name "${IMAGE_NAME}"
#
# Refer to:
# https://docs.nvidia.com/ngc/latest/ngc-private-registry-user-guide.html#accessing-the-ngc-container-registry
# https://docs.nvidia.com/ngc/latest/ngc-private-registry-user-guide.html#uploading-an-nvidia-container-image-onto-your-system
# https://docs.nvidia.com/ngc/latest/ngc-private-registry-user-guide.html#tagging-and-pushing-a-container-image
# https://docs.nvidia.com/ngc/latest/ngc-private-registry-user-guide.html#pushing-a-container-image
#
# e.g.,
# ngc config set
# ngc registry image list --org omniverse-internal
# ngc registry image create omniverse-internal/kit-dev/gpud
# ngc registry image create gpud
# ngc registry image info --org omniverse-internal --name gpud
# NVCR_ORG=leptonai NGC_API_KEY=<api-key> IMAGE_TAG=v0.7.1 scripts/build-and-push-container-image.sh

set -euo pipefail

if [[ "${TRACE:-0}" == "1" ]]; then
  set -x
fi

: "${NVCR_ORG:?Set NVCR_ORG to your NGC organization identifier (for example, leptonai).}"
: "${NGC_API_KEY:?Set NGC_API_KEY to an API key with push access to nvcr.io.}"

IMAGE_NAME="${IMAGE_NAME:-gpud}"
IMAGE_TAG="${IMAGE_TAG:-}"
NVCR_TEAM="${NVCR_TEAM:-}"
NVCR_NAMESPACE="${NVCR_NAMESPACE:-}"
DOCKERFILE_PATH="${DOCKERFILE_PATH:-Dockerfile}"
BUILD_CONTEXT="${BUILD_CONTEXT:-.}"
PLATFORM="${PLATFORM:-linux/amd64}"

if [[ -z "${IMAGE_TAG}" && -f deployments/helm/gpud/Chart.yaml ]]; then
  IMAGE_TAG="$(awk -F': *' '/^appVersion:/ {gsub(/"/, "", $2); print $2; exit}' deployments/helm/gpud/Chart.yaml)"
fi

: "${IMAGE_TAG:?Set IMAGE_TAG to control the pushed image tag (defaults to the Helm chart appVersion when available).}"

if [[ -n "${NVCR_NAMESPACE}" ]]; then
  NVCR_PATH="${NVCR_NAMESPACE}"
elif [[ -n "${NVCR_TEAM}" ]]; then
  NVCR_PATH="${NVCR_ORG}/${NVCR_TEAM}"
else
  NVCR_PATH="${NVCR_ORG}"
fi

IMAGE_URI="nvcr.io/${NVCR_PATH}/${IMAGE_NAME}:${IMAGE_TAG}"

echo "Logging in to nvcr.io as \$oauthtoken..."
echo "${NGC_API_KEY}" | docker login nvcr.io --username '$oauthtoken' --password-stdin

# Docker CLI supports pushing to nvcr.io directly; see
# https://docs.nvidia.com/ngc/latest/ngc-private-registry-user-guide.html#pushing-a-container-image.
echo "Building ${IMAGE_URI} from ${DOCKERFILE_PATH}..."
declare -a BUILD_ARGS=()
[[ -n "${OS_NAME:-}" ]] && BUILD_ARGS+=(--build-arg "OS_NAME=${OS_NAME}")
[[ -n "${OS_VERSION:-}" ]] && BUILD_ARGS+=(--build-arg "OS_VERSION=${OS_VERSION}")
[[ -n "${CUDA_VERSION:-}" ]] && BUILD_ARGS+=(--build-arg "CUDA_VERSION=${CUDA_VERSION}")

docker build \
  --platform "${PLATFORM}" \
  "${BUILD_ARGS[@]}" \
  -f "${DOCKERFILE_PATH}" \
  -t "${IMAGE_URI}" \
  "${BUILD_CONTEXT}"

echo "Pushing ${IMAGE_URI} to nvcr.io..."
docker push "${IMAGE_URI}"

echo "Image pushed: ${IMAGE_URI}"
