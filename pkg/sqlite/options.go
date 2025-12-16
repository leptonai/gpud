package sqlite

type Op struct {
	readOnly bool
	cache    string // cache mode for in-memory databases (e.g., "shared")
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

// WithCache sets the SQLite cache mode for in-memory databases.
// Use "shared" to allow multiple connections to share the same in-memory database.
// If empty (default), no cache parameter is added to the connection string.
// ref. https://github.com/mattn/go-sqlite3?tab=readme-ov-file#faq
func WithCache(mode string) OpOption {
	return func(op *Op) {
		op.cache = mode
	}
}
