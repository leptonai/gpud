package disk

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"

	"github.com/leptonai/gpud/pkg/disk"
)

func nfsMountTargetUsage(target string, parts disk.Partitions) (*disk.FindMntOutput, bool) {
	var selected *disk.Partition
	target = filepath.Clean(target)
	for i := range parts {
		p := &parts[i]
		mount := filepath.Clean(p.MountPoint)
		if target == mount || strings.HasPrefix(target, strings.TrimRight(mount, "/")+"/") {
			if selected == nil || len(mount) > len(selected.MountPoint) {
				selected = p
			}
		}
	}
	if selected == nil {
		return nil, false
	}
	// Reuse the bounded collection, including failures; stat/findmnt --df here
	// would re-enter the same blocked filesystem after the collector timed out.
	// A subtree may contain an overmount or may not exist. Do not fabricate a
	// findmnt result without traversing it; the mount-level sample stays available.
	if target != filepath.Clean(selected.MountPoint) || selected.Usage == nil {
		return nil, true
	}
	u := selected.Usage
	percent := float64(0)
	if u.TotalBytes != 0 {
		percent = 100 * float64(u.UsedBytes) / float64(u.TotalBytes)
	}
	return &disk.FindMntOutput{Target: target, Filesystems: []disk.FoundMnt{{
		MountedPoint: selected.MountPoint, Sources: []string{selected.Device}, Fstype: selected.Fstype,
		SizeBytes: u.TotalBytes, UsedBytes: u.UsedBytes, AvailableBytes: u.FreeBytes,
		SizeHumanized: humanize.IBytes(u.TotalBytes), UsedHumanized: humanize.IBytes(u.UsedBytes), AvailableHumanized: humanize.IBytes(u.FreeBytes),
		UsedPercent: percent, UsedPercentHumanized: fmt.Sprintf("%.0f%%", percent),
	}}}, true
}

func (c *component) reconcileNFSStatFailures(parts disk.Partitions) {
	c.nfsStatTimeoutMu.Lock()
	defer c.nfsStatTimeoutMu.Unlock()
	identities := make(map[string]string, len(parts))
	for _, p := range parts {
		id := p.Device + "\x00" + p.Fstype
		identities[p.MountPoint] = id
		if previous, ok := c.nfsStatIdentities[p.MountPoint]; ok && previous != id {
			delete(c.nfsStatTimeoutCounts, p.MountPoint)
		}
	}
	// Removed mounts must not carry failures into a later mount at the same path.
	for path := range c.nfsStatTimeoutCounts {
		if _, ok := identities[path]; !ok {
			delete(c.nfsStatTimeoutCounts, path)
		}
	}
	c.nfsStatIdentities = identities
}
