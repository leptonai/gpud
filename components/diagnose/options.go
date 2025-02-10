package diagnose

type Op struct {
	nvidiaSMICommand         string
	nvidiaSMIQueryCommand    string
	ibstatCommand            string
	infinibandClassDirectory string

	lines         int
	debug         bool
	createArchive bool

	pollGPMEvents bool

	netcheck  bool
	diskcheck bool

	dmesgCheck bool

	checkIb bool
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

	if op.lines == 0 {
		op.lines = 100
	}
	return nil
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

func WithCheckIb(b bool) OpOption {
	return func(op *Op) {
		op.checkIb = b
	}
}
