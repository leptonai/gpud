#!/usr/bin/env bash
# Runs the NVSentinel integration test in a Linux container.
# Builds the test binary, starts a gRPC receiver on a unix socket,
# sends a simulated NVSentinel health event, and checks the full flow.
set -euo pipefail

cd "$(dirname "$0")/.."

echo "building nvsentinel integration test image..."
podman build -f cmd/nvsentinel-integration-test/Containerfile -t nvsentinel-test .

echo "running integration test..."
podman run --rm nvsentinel-test

echo "building unit test image..."
podman build -f cmd/nvsentinel-integration-test/Containerfile.test -t nvsentinel-unittest .

echo "running unit tests (non-root, skips /dev/kmsg)..."
podman run --rm nvsentinel-unittest
