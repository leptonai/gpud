#!/usr/bin/env bash

set -e
set -o pipefail

LOG_FILE=${1:-test-coverage.log}
COVERAGE_DIR=${COVERAGE_DIR:-coverage_dir}
COVERAGE_FILE="${COVERAGE_DIR}/coverage.out"

mkdir -p "${COVERAGE_DIR}"

echo "=== Running tests and generating coverage report ==="
go test -coverprofile="${COVERAGE_FILE}" ./... 2>&1 | tee "${LOG_FILE}"
test_success=$?

if [ $test_success -ne 0 ]; then
  echo "Tests failed with exit status: $test_success"
  exit $test_success
fi

echo "=== Checking if coverage file exists ==="
if [ ! -f "${COVERAGE_FILE}" ]; then
  echo "Coverage file not found: ${COVERAGE_FILE}"
  exit 1
fi

echo "=== Uploading coverage report to Codecov ==="
if ! bash <(curl -s https://codecov.io/bash) -f "${COVERAGE_FILE}" -cF all; then
  echo "Failed to upload to Codecov"
  exit 2
fi

echo "=== Upload completed with exit status: $test_success ==="
exit $test_success
