package file

import "errors"

var (
	ErrLibraryEmpty    = errors.New("library name is empty")
	ErrLibraryNotFound = errors.New("library not found")
)

func FindLibrary(name string, opts ...OpOption) (string, error) {
	options := &Op{}
	if err := options.applyOpts(opts); err != nil {
		return "", err
	}

	return findLibrary(options.searchDirs, name)
}

type Op struct {
	searchDirs map[string]any
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	if len(op.searchDirs) == 0 {
		op.searchDirs = defaultLibSearchDirs
	}
	return nil
}

func WithSearchDirs(paths ...string) OpOption {
	return func(op *Op) {
		if op.searchDirs == nil {
			op.searchDirs = make(map[string]any)
		}
		for _, path := range paths {
			op.searchDirs[path] = struct{}{}
		}
	}
}
