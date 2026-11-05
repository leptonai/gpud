# Contributing to GPUd

First and foremost, thank you for considering contributing to GPUd! We appreciate the time and effort you're putting into helping improve our project. This guide outlines the process and standards we expect from contributors.

## Signing Off Your Work

We require that all contributors "sign-off" on their commits. This certifies
that the contribution is your original work, or you have rights to submit it
under the same license, or a compatible license.

Any contribution which contains commits that are not Signed-Off will not be
accepted.

To sign off on a commit, use the `--signoff` (or `-s`) option:

```bash
git commit -s -m "Describe your change"
```

This appends the following to your commit message:

```text
Signed-off-by: Your Name <your@email.com>
```

Full text of the [Developer Certificate of Origin](https://developercertificate.org/):

```text
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

## Development

First clone the source code from Github

```bash
git clone https://github.com/leptonai/gpud.git
```

Use `go` to build `gpud` from source

```bash
cd gpud
make all

./bin/gpud -h
```

---

## Building with Docker

### Prerequisites

- Docker with `buildx` enabled.
- Access to a container registry (e.g., Docker Hub, NGC) if you plan to push images.

### Local builds (for Testing)

This is the fastest way to build an image for your local machine's architecture to test changes. The `--load` flag makes the image immediately available in your local Docker library.

```bash
# Set local build variables
export IMAGE_NAME="gpud"
export OS_NAME="ubuntu"
export OS_VERSION="22.04"
export CUDA_VERSION="12.4.1"
export GIT_COMMIT_HASH=$(git rev-parse --short HEAD)

# Create the full tag and build the image
export FULL_TAG="${IMAGE_NAME}:${GIT_COMMIT_HASH}-cuda${CUDA_VERSION}-${OS_NAME}${OS_VERSION}"

docker buildx build \
  --build-arg OS_NAME=${OS_NAME} \
  --build-arg OS_VERSION=${OS_VERSION} \
  --build-arg CUDA_VERSION=${CUDA_VERSION} \
  --tag ${FULL_TAG} \
  --no-cache \
  --load .
```

---

### Multi-platform builds (for Releases)

This process builds a multi-platform image and pushes it directly to a container registry. The image tag should clearly state its dependencies.

1. Log in to your container registry
You must be logged in to the registry where you intend to push the image.

```bash
# Example for NVIDIA's NGC container registry 
docker login nvcr.io
```

2. Define Build variables and Tag

```bash
# The repository for the image (e.g., nvcr.io/leptonai/gpud)
export IMAGE_REPO="your-registry/gpud"
# The git tag version
export GIT_TAG="v..."
# Critical dependencies
export OS_NAME="ubuntu"
export OS_VERSION="22.04"
export CUDA_VERSION="12.4.1"

# A specific, fully discriptive tag
export FULL_TAG="${IMAGE_REPO}:${GIT_TAG#v}-cuda${CUDA_VERSION}-${OS_NAME}${OS_VERSION}"
```

3. Build and Push the image

This single command builds for both `amd64` and `arm64`, tags the resulting manifest, and pushes it to the registry.

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg OS_NAME=${OS_NAME} \
  --build-arg OS_VERSION=${OS_VERSION} \
  --build-arg CUDA_VERSION=${CUDA_VERSION} \
  --tag ${FULL_TAG} \
  --push .
```

---

## Testing

We highly recommend writing tests for new features or bug fixes and ensure all tests passing before submitting a PR.

To run all existing tests locally, simply run

```bash
./scripts/tests-unit.sh
./scripts/tests-e2e.sh
```

## Coding Standards

Ensure your code is clean, readable, and well-commented. We use [golangci-lintblack](https://golangci-lint.run/) as code linter.

To run lint locally, first install linters by doing

```bash
golangci-lint run
```
