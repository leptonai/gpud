package config

import (
	"github.com/leptonai/gpud/components"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
)

type Op struct {
	pkgconfigcommon.ToolOverwrites

	FailureInjector *components.FailureInjector

	DataDir string
}

type OpOption func(*Op)

func (op *Op) ApplyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.InfinibandClassRootDir == "" {
		op.InfinibandClassRootDir = infinibandclass.DefaultRootDir
	}

	return nil
}

// Specifies the root directory of the InfiniBand class.
func WithInfinibandClassRootDir(p string) OpOption {
	return func(op *Op) {
		op.InfinibandClassRootDir = p
	}
}

func WithFailureInjector(injector *components.FailureInjector) OpOption {
	return func(op *Op) {
		op.FailureInjector = injector
	}
}

// WithDataDir overrides the default data directory for GPUd artifacts.
func WithDataDir(dataDir string) OpOption {
	return func(op *Op) {
		op.DataDir = dataDir
	}
}
