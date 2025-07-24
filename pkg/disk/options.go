package disk

import (
	"strings"
	"time"
)

type Op struct {
	matchFuncFstype     MatchFunc
	matchFuncDeviceType MatchFunc
	matchFuncMountPoint MatchFunc
	skipUsage           bool
	statTimeout         time.Duration
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

	if op.matchFuncMountPoint == nil {
		op.matchFuncMountPoint = func(_ string) bool {
			return true
		}
	}

	if op.statTimeout == 0 {
		op.statTimeout = 5 * time.Second
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

// WithMountPoint is used to filter out devices.
// This is useful for filtering out devices that are not mounted, such as swap devices.
func WithMountPoint(matchFunc MatchFunc) OpOption {
	return func(op *Op) {
		op.matchFuncMountPoint = matchFunc
	}
}

func WithSkipUsage() OpOption {
	return func(op *Op) {
		op.skipUsage = true
	}
}

func WithStatTimeout(timeout time.Duration) OpOption {
	return func(op *Op) {
		op.statTimeout = timeout
	}
}

func DefaultMatchFuncDeviceType(deviceType string) bool {
	return deviceType == "disk" // not "part" partitions
}

func DefaultFsTypeFunc(fsType string) bool {
	return fsType == "" ||
		fsType == "ext4" ||
		fsType == "LVM2_member" ||
		fsType == "linux_raid_member" ||
		fsType == "raid0" ||
		strings.HasPrefix(fsType, "nfs") // e.g., "nfs4"
}

func DefaultExt4FsTypeFunc(fsType string) bool {
	return fsType == "ext4"
}

func DefaultNFSFsTypeFunc(fsType string) bool {
	// ref. https://www.weka.io/
	return fsType == "wekafs" || fsType == "virtiofs" || strings.HasPrefix(fsType, "nfs") // e.g., "nfs4"
}

func DefaultDeviceTypeFunc(dt string) bool {
	return dt == "disk" || dt == "lvm" || dt == "part"
}

func DefaultMountPointFunc(mountPoint string) bool {
	if mountPoint == "" {
		return false
	}

	// ref. https://docs.nebius.com/cli/compute-vm#create
	if strings.HasPrefix(mountPoint, "/mnt/cloud-metadata") {
		return false
	}

	return true
}
