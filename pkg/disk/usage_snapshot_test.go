package disk

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNFSUsageSnapshot(t *testing.T) {
	previous := latestNFSUsage.Swap(nil)
	t.Cleanup(func() { latestNFSUsage.Store(previous) })
	part := Partition{Device: "server:/export", MountPoint: "/remote", Fstype: "nfs4", Mounted: true, Usage: &Usage{TotalBytes: 1000, UsedBytes: 400}}
	assert.Nil(t, CachedNFSUsage("host-df", part))
	PublishNFSUsage("host-df", Partitions{part})
	require.Equal(t, part.Usage, CachedNFSUsage("host-df", part))
	part.Usage.TotalBytes = 2000
	got := CachedNFSUsage("host-df", part)
	require.NotNil(t, got)
	assert.Equal(t, uint64(1000), got.TotalBytes, "publication owns its copy")
	got.UsedBytes = 999
	assert.Equal(t, uint64(400), CachedNFSUsage("host-df", part).UsedBytes, "readers own their copies")
	assert.Nil(t, CachedNFSUsage("container-df", part))
	for _, changed := range []Partition{
		{Device: "other:/export", MountPoint: part.MountPoint, Fstype: part.Fstype},
		{Device: part.Device, MountPoint: "/other", Fstype: part.Fstype},
		{Device: part.Device, MountPoint: part.MountPoint, Fstype: "nfs"},
	} {
		assert.Nil(t, CachedNFSUsage("host-df", changed))
	}

	key := usageKey{part.MountPoint, part.Device, part.Fstype}
	sample := latestNFSUsage.Load().usage[key]
	part.Mounted, part.StatTimedOut, part.Usage = false, true, nil
	PublishNFSUsage("host-df", Partitions{part})
	assert.Equal(t, sample, latestNFSUsage.Load().usage[key], "a failed probe preserves the original timestamp")
	sample.at = time.Now().Add(-nfsUsageMaxAge)
	latestNFSUsage.Store(&usageSnapshot{command: "host-df", usage: map[usageKey]usageSample{key: sample}})
	assert.Nil(t, CachedNFSUsage("host-df", part), "stale samples are not reported")
	PublishNFSUsage("host-df", Partitions{part})
	assert.Empty(t, latestNFSUsage.Load().usage, "stale failed probes are evicted")

	part.Mounted, part.StatTimedOut, part.Usage = true, false, &Usage{TotalBytes: 3000}
	PublishNFSUsage("host-df", Partitions{part})
	PublishNFSUsage("host-df", nil)
	assert.Nil(t, CachedNFSUsage("host-df", part), "successful scans remove disappeared mounts")
}

func TestNFSUsageSnapshotConcurrentReaders(t *testing.T) {
	previous := latestNFSUsage.Swap(nil)
	t.Cleanup(func() { latestNFSUsage.Store(previous) })
	part := Partition{Device: "server:/export", MountPoint: "/remote", Fstype: "nfs4", Mounted: true, Usage: &Usage{TotalBytes: 1000}}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				PublishNFSUsage("df", Partitions{part})
				if usage := CachedNFSUsage("df", part); usage != nil {
					assert.Equal(t, uint64(1000), usage.TotalBytes)
				}
			}
		})
	}
	wg.Wait()
}
