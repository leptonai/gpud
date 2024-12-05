package disk

import "strings"

type Op struct {
	fstypeMatchFunc MatchFstypeFunc
	device          string
}

type MatchFstypeFunc func(fs string) bool

func DefaultMatchFstypeFunc(fs string) bool {
	return strings.HasPrefix(fs, "ext4") ||
		strings.HasPrefix(fs, "apfs") ||
		strings.HasPrefix(fs, "xfs") ||
		strings.HasPrefix(fs, "btrfs") ||
		strings.HasPrefix(fs, "zfs") ||
		(strings.HasPrefix(fs, "fuse.") && !strings.HasPrefix(fs, "fuse.lxcfs")) // e.g., "fuse.juicefs"
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.fstypeMatchFunc == nil {
		op.fstypeMatchFunc = DefaultMatchFstypeFunc
	}

	return nil
}

func WithMatchFstypeFunc(matchFunc MatchFstypeFunc) OpOption {
	return func(op *Op) {
		op.fstypeMatchFunc = matchFunc
	}
}

func WithDevice(device string) OpOption {
	return func(op *Op) {
		op.device = device
	}
}
