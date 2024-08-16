#!/usr/bin/env bash
set -xue

# do not mask errors in a pipeline
set -o pipefail

# treat unset variables as an error
set -o nounset

# exit script whenever it errs
set -o errexit

if ! [[ "$0" =~ scripts/tests-unit.sh ]]; then
    echo "must be run from root"
    exit 255
fi

go fmt ./...
go vet -v ./...
go test -v ./...
go test -v -race ./...
