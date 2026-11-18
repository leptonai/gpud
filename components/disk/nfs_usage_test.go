package disk

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/disk"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

func TestBlockedNFSUsagePublishesFreshHealthAndThreshold(t *testing.T) {
	// Only the usage I/O is blocked. Let the native probe validate a real mount.
	const target = "/proc"
	part := disk.Partition{MountPoint: target}
	if runtime.GOOS == "linux" {
		mounts, err := procfs.GetMounts()
		require.NoError(t, err)
		for _, mount := range mounts {
			if mount.MountPoint == target && mount.FSType == "proc" {
				part.Fstype, part.Device = mount.FSType, mount.Source
				break
			}
		}
		require.NotEmpty(t, part.Fstype)
	}

	blocked := make(chan struct{})
	started := make(chan struct{})
	var calls atomic.Int32
	patch := mockey.Mock(disk.GetUsage).To(func(context.Context, string) (*disk.Usage, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-blocked
		return &disk.Usage{TotalBytes: 100, FreeBytes: 50, UsedBytes: 50}, nil
	}).Build()
	defer patch.UnPatch()
	collector := &disk.NFSUsageCollector{}
	mounts := disk.Partitions{part}
	c := createTestComponent(t.Context(), []string{target}, nil)
	defer func() { _ = c.Close() }()
	c.getBlockDevicesFunc = nil
	c.getExt4PartitionsFunc = func(context.Context) (disk.Partitions, error) { return nil, nil }
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs { return pkgnfschecker.Configs{{VolumePath: target}} }
	first := true
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		if first {
			first = false
			timeout, cancel := context.WithCancel(ctx)
			defer cancel()
			result := make(chan disk.Partitions, 1)
			go func() { result <- collector.Collect(timeout, mounts, "") }()
			select {
			case <-started:
			case parts := <-result:
				t.Fatalf("usage probe returned before reaching the blocked operation: %+v", parts)
			case <-time.After(5 * time.Second):
				t.Fatal("usage probe did not reach the blocked operation")
			}
			cancel()
			return <-result, nil
		}
		timeout, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		return collector.Collect(timeout, mounts, ""), nil
	}
	c.findMntFunc = func(context.Context, string) (*disk.FindMntOutput, error) {
		t.Fatal("must not re-probe the blocked NFS target")
		return nil, nil
	}
	now := time.Now()
	c.getTimeNowFunc = func() time.Time { now = now.Add(time.Minute); return now }
	var previous time.Time
	for i := 1; i <= 5; i++ {
		cr := c.Check().(*checkResult)
		require.True(t, cr.ts.After(previous))
		previous = cr.ts
		require.True(t, cr.NFSPartitions[0].StatTimedOut)
		if i < 5 {
			require.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		} else {
			require.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
			require.Contains(t, cr.reason, target+" stat timed out")
			require.Contains(t, cr.reason, "5 checks")
		}
	}
	require.EqualValues(t, 1, calls.Load())
	close(blocked)
	require.Eventually(t, func() bool { return c.Check().HealthStateType() == apiv1.HealthStateTypeHealthy }, time.Second, time.Millisecond)
	require.GreaterOrEqual(t, calls.Load(), int32(2))
}

func TestNFSCollectionErrorCannotReturnHealthy(t *testing.T) {
	c := createTestComponent(t.Context(), nil, nil)
	defer func() { _ = c.Close() }()
	calls := 0
	c.getNFSPartitionsFunc = func(context.Context) (disk.Partitions, error) {
		calls++
		return nil, errors.New("inventory unavailable")
	}
	cr := &checkResult{health: apiv1.HealthStateTypeHealthy, reason: "ok"}
	require.False(t, c.fetchNFSPartitions(cr))
	require.Equal(t, 1, calls)
	require.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
	require.Contains(t, cr.reason, "inventory unavailable")
}

func TestNFSFailureCountsResetForRemovedOrReplacedMount(t *testing.T) {
	c := &component{}
	parts := disk.Partitions{{MountPoint: "/nfs", Device: "a", Fstype: "nfs4"}}
	c.reconcileNFSStatFailures(parts)
	require.Equal(t, 1, c.recordNFSStatTimeout("/nfs", true))
	parts[0].Device = "b"
	c.reconcileNFSStatFailures(parts)
	require.Equal(t, 1, c.recordNFSStatTimeout("/nfs", true))
	c.reconcileNFSStatFailures(nil)
	c.reconcileNFSStatFailures(parts)
	require.Equal(t, 1, c.recordNFSStatTimeout("/nfs", true))
}

func TestNFSTargetUsageReusesBoundedSample(t *testing.T) {
	parts := disk.Partitions{{MountPoint: "/nfs", Device: "server:/export", Fstype: "nfs4", Usage: &disk.Usage{TotalBytes: 100, UsedBytes: 40, FreeBytes: 60}}}
	result, ok := nfsMountTargetUsage("/nfs", parts)
	require.True(t, ok)
	require.EqualValues(t, 100, result.Filesystems[0].SizeBytes)
	require.EqualValues(t, 40, result.Filesystems[0].UsedPercent)
	_, ok = nfsMountTargetUsage("/nfssomething", parts)
	require.False(t, ok)
	parts[0].Usage = nil
	result, ok = nfsMountTargetUsage("/nfs/child", parts)
	require.True(t, ok)
	require.Nil(t, result)
}
