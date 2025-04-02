package config

import (
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
)

type Op struct {
	KernelModulesToCheck         []string
	DockerIgnoreConnectionErrors bool

	nvidia_common.ToolOverwrites
}

type OpOption func(*Op)

func (op *Op) ApplyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.IbstatCommand == "" {
		op.IbstatCommand = "ibstat"
	}

	return nil
}

func WithKernelModulesToCheck(modules ...string) OpOption {
	return func(op *Op) {
		op.KernelModulesToCheck = append(op.KernelModulesToCheck, modules...)
	}
}

func WithDockerIgnoreConnectionErrors(b bool) OpOption {
	return func(op *Op) {
		op.DockerIgnoreConnectionErrors = b
	}
}

// Specifies the ibstat binary path to overwrite the default path.
func WithIbstatCommand(p string) OpOption {
	return func(op *Op) {
		op.IbstatCommand = p
	}
}
