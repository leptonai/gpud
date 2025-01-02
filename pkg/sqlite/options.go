package sqlite

type Op struct {
	readOnly bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

// ref. https://github.com/mattn/go-sqlite3/issues/1179#issuecomment-1638083995
func WithReadOnly(b bool) OpOption {
	return func(op *Op) {
		op.readOnly = b
	}
}
