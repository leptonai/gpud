#!/usr/bin/env bash
set -xue

# do not mask errors in a pipeline
set -o pipefail

# treat unset variables as an error
set -o nounset

# exit script whenever it errs
set -o errexit

# requires
# go install github.com/swaggo/swag/v2/cmd/swag@latest

# Generate swagger documentation
# --dir . : Search from root directory to find all Go files
# --generalInfo ./cmd/gpud/main.go : Specify the main file with general API info
# --output ./docs/apis : Output directory for generated docs
# --parseDependency : Parse dependency files
# --parseInternal : Parse internal packages
# --parseDepth 10 : Increase parse depth to ensure all handlers are found
swag init --dir . --generalInfo ./cmd/gpud/main.go --output ./docs/apis --parseDependency --parseInternal --parseDepth 10
