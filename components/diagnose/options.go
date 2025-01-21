package diagnose

type Op struct {
	lines         int
	debug         bool
	createArchive bool

	pollXidEvents bool
	pollGPMEvents bool

	netcheck  bool
	diskcheck bool

	dmesgCheck bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	if op.lines == 0 {
		op.lines = 100
	}
	return nil
}

func WithLines(lines int) OpOption {
	return func(op *Op) {
		op.lines = lines
	}
}

func WithDebug(b bool) OpOption {
	return func(op *Op) {
		op.debug = b
	}
}

func WithCreateArchive(b bool) OpOption {
	return func(op *Op) {
		op.createArchive = b
	}
}

func WithPollXidEvents(b bool) OpOption {
	return func(op *Op) {
		op.pollXidEvents = b
	}
}

func WithPollGPMEvents(b bool) OpOption {
	return func(op *Op) {
		op.pollGPMEvents = b
	}
}

// WithNetcheck enables network connectivity checks to global edge/derp servers.
func WithNetcheck(b bool) OpOption {
	return func(op *Op) {
		op.netcheck = b
	}
}

func WithDiskcheck(b bool) OpOption {
	return func(op *Op) {
		op.diskcheck = b
	}
}

func WithDmesgCheck(b bool) OpOption {
	return func(op *Op) {
		op.dmesgCheck = b
	}
}
