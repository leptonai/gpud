package config

import "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"

type Op struct {
	FilesToCheck                  []string
	KernelModulesToCheck          []string
	ExpectedPortStates            *infiniband.ExpectedPortStates
	DockerIgnoreConnectionErrors  bool
	KubeletIgnoreConnectionErrors bool
}

type OpOption func(*Op)

func (op *Op) ApplyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

func WithFilesToCheck(files ...string) OpOption {
	return func(op *Op) {
		op.FilesToCheck = append(op.FilesToCheck, files...)
	}
}

func WithKernelModulesToCheck(modules ...string) OpOption {
	return func(op *Op) {
		op.KernelModulesToCheck = append(op.KernelModulesToCheck, modules...)
	}
}

func WithExpectedPortStates(exp infiniband.ExpectedPortStates) OpOption {
	return func(op *Op) {
		op.ExpectedPortStates = &exp
	}
}

func WithDockerIgnoreConnectionErrors(b bool) OpOption {
	return func(op *Op) {
		op.DockerIgnoreConnectionErrors = b
	}
}

func WithKubeletIgnoreConnectionErrors(b bool) OpOption {
	return func(op *Op) {
		op.KubeletIgnoreConnectionErrors = b
	}
}
