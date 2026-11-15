package scan

import (
	"github.com/leptonai/gpud/components"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	infinibandtypes "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
)

type Op struct {
	infinibandClassRootDir       string
	infinibandExpectedPortStates *infinibandtypes.ExpectedPortStates
	debug                        bool
	failureInjector              *components.FailureInjector
}

type OpOption func(*Op)

// WithInfinibandExpectedPortStates gives the installed NIC inventory precedence
// over a product profile. A non-nil zero value explicitly disables the threshold.
func WithInfinibandExpectedPortStates(states infinibandtypes.ExpectedPortStates) OpOption {
	return func(op *Op) {
		op.infinibandExpectedPortStates = &states
	}
}

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
