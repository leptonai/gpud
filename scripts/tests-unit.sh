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

# Run tests with coverage
# -gcflags="all=-N -l": Disable optimizations (-N) and inlining (-l) required by mockey
go test -v -gcflags="all=-N -l" -coverprofile="coverage.txt" ./...

# Run race detector tests
# -gcflags="all=-N -l": Disable optimizations/inlining for mockey compatibility
#                       (see https://github.com/bytedance/mockey#requirements)
# -d=checkptr=0: Disable pointer checking to prevent non-deterministic crashes with mockey's
#                runtime function patching (mockey uses unsafe pointer manipulation which can
#                trigger checkptr violations even though the code is correct)
#                (see https://github.com/golang/go/issues/34964 and
#                 https://github.com/chromedp/chromedp/issues/578)
go test -v -race -gcflags="all=-N -l -d=checkptr=0" ./...
