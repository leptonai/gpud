package disk

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/prometheus/procfs"
	"golang.org/x/sys/unix"
)

// openNFSUsageMount pins the visible mount without triggering a terminal autofs
// mount. fdinfo identifies that mount even when mountinfo lists a stack.
func openNFSUsageMount(part Partition) (*os.File, string, error) {
	file, err := os.OpenFile(part.MountPoint, unix.O_PATH, 0)
	if err != nil {
		return nil, "", err
	}
	identity, err := readNFSUsageIdentity(part, file.Fd())
	if err != nil {
		_ = file.Close()
		return nil, "", err
	}
	return file, identity, nil
}

func readNFSUsageIdentity(part Partition, fd uintptr) (string, error) {
	self, err := procfs.Self()
	if err != nil {
		return "", err
	}
	info, err := self.FDInfo(strconv.FormatUint(uint64(fd), 10))
	if err != nil {
		return "", err
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	return nfsUsageIdentity(f, part, info.MntID)
}

func probeNativeNFSUsage(ctx context.Context, part Partition) (*Usage, error) {
	mount, before, err := openNFSUsageMount(part)
	if err != nil {
		return nil, err
	}
	defer func() { _ = mount.Close() }()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// A concurrent overmount must never substitute another filesystem's capacity.
	usage, err := GetUsage(ctx, fmt.Sprintf("/proc/self/fd/%d", mount.Fd()))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, after, err := openNFSUsageMount(part)
	if err != nil {
		return nil, err
	}
	defer func() { _ = current.Close() }()
	if before != after {
		return nil, fmt.Errorf("filesystem identity changed while collecting %q", part.MountPoint)
	}
	return usage, nil
}
