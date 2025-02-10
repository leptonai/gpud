package config

import (
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
)

type Op struct {
	FilesToCheck                  []string
	KernelModulesToCheck          []string
	DockerIgnoreConnectionErrors  bool
	KubeletIgnoreConnectionErrors bool

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

// Specifies the nvidia-smi binary path to overwrite the default path.
func WithNvidiaSMICommand(p string) OpOption {
	return func(op *Op) {
		op.NvidiaSMICommand = p
	}
}

func WithNvidiaSMIQueryCommand(p string) OpOption {
	return func(op *Op) {
		op.NvidiaSMIQueryCommand = p
	}
}

// Specifies the ibstat binary path to overwrite the default path.
func WithIbstatCommand(p string) OpOption {
	return func(op *Op) {
		op.IbstatCommand = p
	}
}
