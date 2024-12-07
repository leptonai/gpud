package pci

type Op struct {
	nameMatchFunc func(string) bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.nameMatchFunc == nil {
		op.nameMatchFunc = func(name string) bool {
			return true
		}
	}

	return nil
}

func WithNameMatch(fn func(string) bool) OpOption {
	return func(op *Op) {
		op.nameMatchFunc = fn
	}
}
