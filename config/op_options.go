package config

type Op struct {
	enableFailComponent bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

func WithEnableFailComponent(b bool) OpOption {
	return func(op *Op) {
		op.enableFailComponent = b
	}
}
