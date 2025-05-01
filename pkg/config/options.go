package config

import (
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
)

type Op struct {
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
	if op.IbstatusCommand == "" {
		op.IbstatusCommand = "ibstatus"
	}

	return nil
}

// Specifies the ibstat binary path to overwrite the default path.
func WithIbstatCommand(p string) OpOption {
	return func(op *Op) {
		op.IbstatCommand = p
	}
}

// Specifies the ibstat binary path to overwrite the default path.
func WithIbstatusCommand(p string) OpOption {
	return func(op *Op) {
		op.IbstatusCommand = p
	}
}
