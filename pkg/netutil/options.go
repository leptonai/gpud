package netutil

type Op struct {
	prefixesToSkip map[string]any
	suffixesToSkip map[string]any
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

func WithPrefixesToSkip(prefixes ...string) OpOption {
	return func(op *Op) {
		if op.prefixesToSkip == nil {
			op.prefixesToSkip = make(map[string]any)
		}
		for _, pfx := range prefixes {
			op.prefixesToSkip[pfx] = nil
		}
	}
}

func WithSuffixesToSkip(suffixes ...string) OpOption {
	return func(op *Op) {
		if op.suffixesToSkip == nil {
			op.suffixesToSkip = make(map[string]any)
		}
		for _, sfx := range suffixes {
			op.suffixesToSkip[sfx] = nil
		}
	}
}
