package config

type Op struct {
	filesToCheck []string
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

func WithFilesToCheck(files ...string) OpOption {
	return func(op *Op) {
		op.filesToCheck = append(op.filesToCheck, files...)
	}
}
