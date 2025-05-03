package scan

type Op struct {
	ibstatCommand   string
	ibstatusCommand string
	debug           bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.ibstatCommand == "" {
		op.ibstatCommand = "ibstat"
	}
	if op.ibstatusCommand == "" {
		op.ibstatusCommand = "ibstatus"
	}

	return nil
}

// Specifies the ibstat binary path to overwrite the default path.
func WithIbstatCommand(p string) OpOption {
	return func(op *Op) {
		op.ibstatCommand = p
	}
}

// Specifies the ibstatus binary path to overwrite the default path.
func WithIbstatusCommand(p string) OpOption {
	return func(op *Op) {
		op.ibstatusCommand = p
	}
}

func WithDebug(b bool) OpOption {
	return func(op *Op) {
		op.debug = b
	}
}
