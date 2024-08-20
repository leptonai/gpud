GO ?= go
INSTALL ?= install

# Root directory of the project (absolute path).
ROOTDIR=$(dir $(abspath $(lastword $(MAKEFILE_LIST))))

BUILD_TIMESTAMP ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VERSION ?= $(shell git describe --match 'v[0-9]*' --dirty='.m' --always)
REVISION=$(shell git rev-parse HEAD)$(shell if ! git diff --no-ext-diff --quiet --exit-code; then echo .m; fi)
PACKAGE=github.com/leptonai/gpud

ifneq "$(strip $(shell command -v $(GO) 2>/dev/null))" ""
	GOOS ?= $(shell $(GO) env GOOS)
	GOARCH ?= $(shell $(GO) env GOARCH)
else
	ifeq ($(GOOS),)
		# approximate GOOS for the platform if we don't have Go and GOOS isn't
		# set. We leave GOARCH unset, so that may need to be fixed.
		ifeq ($(OS),Windows_NT)
			GOOS = windows
		else
			UNAME_S := $(shell uname -s)
			ifeq ($(UNAME_S),Linux)
				GOOS = linux
			endif
			ifeq ($(UNAME_S),Darwin)
				GOOS = darwin
			endif
			ifeq ($(UNAME_S),FreeBSD)
				GOOS = freebsd
			endif
		endif
	else
		GOOS ?= $$GOOS
		GOARCH ?= $$GOARCH
	endif
endif


ifndef GODEBUG
	EXTRA_LDFLAGS += -s -w
	DEBUG_GO_GCFLAGS :=
	DEBUG_TAGS :=
else
	DEBUG_GO_GCFLAGS := -gcflags=all="-N -l"
	DEBUG_TAGS := static_build
endif

WHALE = "🇩"
ONI = "👹"

RELEASE=gpud-$(VERSION:v%=%)-${GOOS}-${GOARCH}

COMMANDS=gpud swagger

GO_BUILD_FLAGS=-ldflags '-s -X $(PACKAGE)/version.BuildTimestamp=$(BUILD_TIMESTAMP) -X $(PACKAGE)/version.Version=$(VERSION) -X $(PACKAGE)/version.Revision=$(REVISION) -X $(PACKAGE)/version.Package=$(PACKAGE)'

ifdef BUILDTAGS
    GO_BUILDTAGS = ${BUILDTAGS}
endif
GO_BUILDTAGS ?=
GO_BUILDTAGS += ${DEBUG_TAGS}
ifneq ($(STATIC),)
	GO_BUILDTAGS += osusergo netgo static_build
endif
GO_TAGS=$(if $(GO_BUILDTAGS),-tags "$(strip $(GO_BUILDTAGS))",)

PACKAGES=$(shell $(GO) list ${GO_TAGS} ./... | grep -v /vendor/)

GOPATHS=$(shell echo ${GOPATH} | tr ":" "\n" | tr ";" "\n")

BINARIES=$(addprefix bin/,$(COMMANDS))

.PHONY: clean all binaries
.DEFAULT: default

all: binaries

FORCE:

define BUILD_BINARY
@echo "$(WHALE) $@"
@$(GO) build ${GO_BUILD_FLAGS} ${DEBUG_GO_GCFLAGS} -o $@ ${GO_TAGS}  ./$<
endef

# Build a binary from a cmd.
bin/%: cmd/% FORCE
	$(call BUILD_BINARY)

binaries: $(BINARIES) ## build binaries
	@echo "$(WHALE) $@"

clean: ## clean up binaries
	@echo "$(WHALE) $@"
	@rm -f $(BINARIES)
