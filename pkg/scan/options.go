package scan

import (
	"github.com/leptonai/gpud/components"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
)

type Op struct {
	infinibandClassRootDir string
	debug                  bool
	failureInjector        *components.FailureInjector
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.infinibandClassRootDir == "" {
		op.infinibandClassRootDir = infinibandclass.DefaultRootDir
	}

	return nil
}

// Specifies the root directory of the InfiniBand class.
func WithInfinibandClassRootDir(p string) OpOption {
	return func(op *Op) {
		op.infinibandClassRootDir = p
	}
}

func WithFailureInjector(injector *components.FailureInjector) OpOption {
	return func(op *Op) {
		op.failureInjector = injector
	}
}

func WithDebug(b bool) OpOption {
	return func(op *Op) {
		op.debug = b
	}
}
