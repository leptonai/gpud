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

	// findmntCommand overrides how the "findmnt" binary is invoked.
	// Empty preserves the default behavior of locating "findmnt" on PATH and
	// running it directly in the current namespace. When set, it is used as the
	// command prefix (e.g. "nsenter --target 1 --mount -- findmnt") and gpud
	// appends the flags it controls (--target, --json, --df).
	findmntCommand string

	// lsblkCommand overrides how the "lsblk" binary is invoked.
	// Empty preserves the default behavior of locating "lsblk" on PATH and
	// running it directly in the current namespace. When set, it is used as the
	// command prefix (e.g. "nsenter --target 1 --mount -- lsblk") and gpud
	// appends the flags it controls.
	lsblkCommand string

	// blockdevUsageCommand overrides how block device usage/partitions are
	// collected. Empty preserves the default behavior of enumerating mounts via
	// gopsutil (/proc/self/mountinfo) and measuring usage via the statfs syscall.
	// When set, it is used as the command prefix (e.g.
	// "nsenter --target 1 --mount -- df") and gpud appends the flags it controls
	// (-T -B1 -P) to collect partitions and usage from its output.
	blockdevUsageCommand string
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

// WithFindmntCommand overrides how the "findmnt" binary is invoked.
// Empty (the default) preserves the legacy behavior of locating "findmnt" on
// PATH and running it directly. When set, the value is used as the command
// prefix (e.g. "nsenter --target 1 --mount -- findmnt") so the command can run
// in the host mount namespace; gpud still appends the flags it controls.
func WithFindmntCommand(command string) OpOption {
	return func(op *Op) {
		op.findmntCommand = command
	}
}

// WithLsblkCommand overrides how the "lsblk" binary is invoked.
// Empty (the default) preserves the legacy behavior of locating "lsblk" on PATH
// and running it directly. When set, the value is used as the command prefix
// (e.g. "nsenter --target 1 --mount -- lsblk") so the command can run in the
// host mount namespace; gpud still appends the flags it controls.
func WithLsblkCommand(command string) OpOption {
	return func(op *Op) {
		op.lsblkCommand = command
	}
}

// WithBlockdevUsageCommand overrides how block device partitions and usage are
// collected. Empty (the default) preserves the legacy behavior of enumerating
// mounts via gopsutil and measuring usage via the statfs syscall. When set, the
// value is used as the command prefix (e.g. "nsenter --target 1 --mount -- df")
// so the measurement can run in the host mount namespace; gpud appends the flags
// it controls (-T -B1 -P) and parses the output.
func WithBlockdevUsageCommand(command string) OpOption {
	return func(op *Op) {
		op.blockdevUsageCommand = command
	}
}

func DefaultMatchFuncDeviceType(deviceType string) bool {
	return deviceType == "disk" // not "part" partitions
}

// DefaultFsTypeFunc returns true for common filesystem types.
// This function is used by the disk component to filter which filesystems to monitor.
//
// Supported filesystem types:
//   - "": Empty/unformatted (parent disks)
//   - "ext4": Most common Linux filesystem
//   - "xfs": Default for RHEL 7+, CentOS 7+, Rocky Linux, AlmaLinux, Fedora Server
//   - "vfat": Required for EFI System Partitions (ESP) per UEFI specification
//   - "LVM2_member": LVM physical volumes
//   - "linux_raid_member": Software RAID members
//   - "raid0": RAID-0 devices
//   - "nfs*": Network File System (e.g., nfs, nfs4)
func DefaultFsTypeFunc(fsType string) bool {
	return fsType == "" ||
		fsType == "ext4" ||
		fsType == "xfs" ||
		fsType == "vfat" ||
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
	// ref. https://wiki.lustre.org/ (Azure Managed Lustre, AWS FSx for Lustre, etc.)
	return fsType == "wekafs" || fsType == "virtiofs" || fsType == "lustre" || strings.HasPrefix(fsType, "nfs") // e.g., "nfs4"
}

// DefaultDeviceTypeFunc returns true for common block device types.
// This function is used by the disk component to filter which devices to monitor.
//
// Supported device types:
//   - "disk": Physical disks (HDDs, SSDs, NVMe)
//   - "part": Disk partitions
//   - "lvm": Logical Volume Manager volumes
//   - "raid*": Software RAID devices (raid0, raid1, raid5, raid10, etc.)
//   - "md*": MD (multiple device) RAID arrays (md, md0, md127, etc.)
//
// RAID/MD support was added to fix https://github.com/leptonai/gpud/issues/1107
// where RAID devices containing the root filesystem were being filtered out,
// causing false "no block device found" warnings.
func DefaultDeviceTypeFunc(dt string) bool {
	if dt == "disk" || dt == "lvm" || dt == "part" {
		return true
	}

	// Support for RAID devices (e.g., raid0, raid1, raid5, raid10)
	// and MD arrays (e.g., md, md0, md127)
	// See: https://github.com/leptonai/gpud/issues/1107
	if strings.HasPrefix(dt, "raid") || strings.HasPrefix(dt, "md") {
		return true
	}

	return false
}

func DefaultMountPointFunc(mountPoint string) bool {
	if mountPoint == "" {
		return false
	}

	// ref. https://docs.nebius.com/cli/compute-vm#create
	if strings.HasPrefix(mountPoint, "/mnt/cloud-metadata") {
		return false
	}

	// in case pod volume mounted on NFS
	if strings.Contains(mountPoint, "/kubelet/pods") {
		return false
	}

	return true
}
