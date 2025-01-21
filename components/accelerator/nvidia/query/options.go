package query

import "database/sql"

type Op struct {
	dbRW                     *sql.DB
	dbRO                     *sql.DB
	nvidiaSMICommand         string
	nvidiaSMIQueryCommand    string
	ibstatCommand            string
	infinibandClassDirectory string
	debug                    bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.nvidiaSMICommand == "" {
		op.nvidiaSMICommand = "nvidia-smi"
	}
	if op.nvidiaSMIQueryCommand == "" {
		op.nvidiaSMIQueryCommand = "nvidia-smi --query"
	}
	if op.ibstatCommand == "" {
		op.ibstatCommand = "ibstat"
	}
	if op.infinibandClassDirectory == "" {
		op.infinibandClassDirectory = "/sys/class/infiniband"
	}

	return nil
}

func WithDBRW(db *sql.DB) OpOption {
	return func(op *Op) {
		op.dbRW = db
	}
}

func WithDBRO(db *sql.DB) OpOption {
	return func(op *Op) {
		op.dbRO = db
	}
}

// Specifies the nvidia-smi binary path to overwrite the default path.
func WithNvidiaSMICommand(p string) OpOption {
	return func(op *Op) {
		op.nvidiaSMICommand = p
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

func WithInfinibandClassDirectory(p string) OpOption {
	return func(op *Op) {
		op.infinibandClassDirectory = p
	}
}

func WithDebug(debug bool) OpOption {
	return func(op *Op) {
		op.debug = debug
	}
}
