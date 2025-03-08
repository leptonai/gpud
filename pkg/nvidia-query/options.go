package query

import (
	"github.com/leptonai/gpud/pkg/eventstore"
)

type Op struct {
	xidEventsBucket        eventstore.Bucket
	hwslowdownEventsBucket eventstore.Bucket
	nvidiaSMIQueryCommand  string
	ibstatCommand          string
	debug                  bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.nvidiaSMIQueryCommand == "" {
		op.nvidiaSMIQueryCommand = "nvidia-smi --query"
	}
	if op.ibstatCommand == "" {
		op.ibstatCommand = "ibstat"
	}

	return nil
}

func WithXidEventBucket(bucket eventstore.Bucket) OpOption {
	return func(op *Op) {
		op.xidEventsBucket = bucket
	}
}

func WithHWSlowdownEventBucket(bucket eventstore.Bucket) OpOption {
	return func(op *Op) {
		op.hwslowdownEventsBucket = bucket
	}
}

func WithNvidiaSMIQueryCommand(p string) OpOption {
	return func(op *Op) {
		op.nvidiaSMIQueryCommand = p
	}
}

// Specifies the ibstat binary path to overwrite the default path.
func WithIbstatCommand(p string) OpOption {
	return func(op *Op) {
		op.ibstatCommand = p
	}
}

func WithDebug(debug bool) OpOption {
	return func(op *Op) {
		op.debug = debug
	}
}
