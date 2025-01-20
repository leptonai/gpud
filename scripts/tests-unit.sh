#!/usr/bin/env bash
set -xue

# do not mask errors in a pipeline
set -o pipefail

# treat unset variables as an error
set -o nounset

# exit script whenever it errs
set -o errexit

LOG_FILE=${1:-test-coverage.log}
COVERAGE_DIR=${COVERAGE_DIR:-coverage_dir}
COVERAGE_FILE="${COVERAGE_DIR}/coverage.txt"

if ! [[ "$0" =~ scripts/tests-unit.sh ]]; then
    echo "must be run from root"
    exit 255
fi

go fmt ./...
go vet -v ./...
go test -v ./...
go test -v -race ./...

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

exit $test_success