package disk

import "strings"

type Op struct {
	matchFuncFstype     MatchFunc
	matchFuncDeviceType MatchFunc
	skipUsage           bool
}

type MatchFunc func(fs string) bool

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.matchFuncFstype == nil {
		op.matchFuncFstype = func(_ string) bool {
			return true
		}
	}

	if op.matchFuncDeviceType == nil {
		op.matchFuncDeviceType = func(_ string) bool {
			return true
		}
	}

	return nil
}

func WithFstype(matchFunc MatchFunc) OpOption {
	return func(op *Op) {
		op.matchFuncFstype = matchFunc
	}
}

func WithDeviceType(matchFunc MatchFunc) OpOption {
	return func(op *Op) {
		op.matchFuncDeviceType = matchFunc
	}
}

func WithSkipUsage() OpOption {
	return func(op *Op) {
		op.skipUsage = true
	}
}

func DefaultMatchFuncFstype(fs string) bool {
	return strings.HasPrefix(fs, "ext4") ||
		strings.HasPrefix(fs, "apfs") ||
		strings.HasPrefix(fs, "xfs") ||
		strings.HasPrefix(fs, "btrfs") ||
		strings.HasPrefix(fs, "zfs") ||
		(strings.HasPrefix(fs, "fuse.") && !strings.HasPrefix(fs, "fuse.lxcfs")) // e.g., "fuse.juicefs"
}

func DefaultMatchFuncDeviceType(deviceType string) bool {
	return deviceType == "disk" // not "part" partitions
}

func DefaultFsTypeFunc(fsType string) bool {
	return fsType == "" ||
		fsType == "ext4" ||
		fsType == "LVM2_member" ||
		fsType == "linux_raid_member" ||
		fsType == "raid0"
}

func DefaultExt4FsTypeFunc(fsType string) bool {
	return fsType == "ext4"
}

func DefaultNFSFsTypeFunc(fsType string) bool {
	// ref. https://www.weka.io/
	return fsType == "wekafs" || fsType == "nfs"
}

func DefaultDeviceTypeFunc(dt string) bool {
	return dt == "disk" || dt == "lvm" || dt == "part"
}
