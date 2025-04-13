package disk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/disk"
)

func TestComponentName(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, []string{}, []string{}).(*component)
	defer c.Close()

	assert.Equal(t, Name, c.Name())
}

func TestNewComponent(t *testing.T) {
	ctx := context.Background()
	mountPoints := []string{"/mnt/test1"}
	mountTargets := []string{"/mnt/test2"}

	c := New(ctx, mountPoints, mountTargets).(*component)
	defer c.Close()

	// Check if mount points are correctly added to the tracking map
	assert.Contains(t, c.mountPointsToTrackUsage, "/mnt/test1")
	assert.Contains(t, c.mountPointsToTrackUsage, "/mnt/test2")
}

func TestComponentStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := New(ctx, []string{}, []string{}).(*component)
	defer c.Close()

	err := c.Start()
	assert.NoError(t, err)
}

func TestComponentClose(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, []string{}, []string{}).(*component)

	err := c.Close()
	assert.NoError(t, err)
}

func TestComponentEvents(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, []string{}, []string{}).(*component)
	defer c.Close()

	events, err := c.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestEmptyDataStates(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, []string{}, []string{}).(*component)
	defer c.Close()

	// No data set yet
	states, err := c.States(ctx)
	require.NoError(t, err)
	assert.Equal(t, "no data yet", states[0].Reason)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.True(t, states[0].DeprecatedHealthy)
}

func TestDataGetStates(t *testing.T) {
	d := &Data{
		ExtPartitions: disk.Partitions{
			{Device: "/dev/sda1", MountPoint: "/mnt/data1"},
		},
		BlockDevices: disk.BlockDevices{
			{Name: "sda", Type: "disk"},
		},

		healthy: true,
		reason:  "found 1 ext4 partitions and 1 block devices",
	}

	states, err := d.getStates()
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Equal(t, "found 1 ext4 partitions and 1 block devices", states[0].Reason)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.True(t, states[0].DeprecatedHealthy)
	assert.Contains(t, states[0].DeprecatedExtraInfo, "data")
	assert.Contains(t, states[0].DeprecatedExtraInfo, "encoding")
}

func TestDataGetError(t *testing.T) {
	// Test with error
	d := &Data{
		err: assert.AnError,
	}
	errStr := d.getError()
	assert.Equal(t, assert.AnError.Error(), errStr)

	// Test without error
	d = &Data{}
	errStr = d.getError()
	assert.Empty(t, errStr)

	// Test with nil data
	var nilData *Data
	errStr = nilData.getError()
	assert.Empty(t, errStr)
}

func TestDataGetStatesWithError(t *testing.T) {
	d := &Data{
		err: errors.New("failed to get disk data"),
	}

	states, err := d.getStates()
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Contains(t, states[0].Error, "failed to get disk data")
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.False(t, states[0].DeprecatedHealthy)
	assert.Contains(t, states[0].DeprecatedExtraInfo, "data")
	assert.Contains(t, states[0].DeprecatedExtraInfo, "encoding")
}

func TestComponentStatesWithError(t *testing.T) {
	ctx := context.Background()
	c := New(ctx, []string{}, []string{}).(*component)
	defer c.Close()

	// Manually set lastData with an error
	c.lastMu.Lock()
	c.lastData = &Data{
		err: errors.New("failed to get disk data"),
	}
	c.lastMu.Unlock()

	states, err := c.States(ctx)
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Contains(t, states[0].Error, "failed to get disk data")
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.False(t, states[0].DeprecatedHealthy)
}

func TestCheckOnce(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "sda",
		Type: "disk",
	}
	mockPartition := disk.Partition{
		Device:     "/dev/sda1",
		MountPoint: "/mnt/data1",
		Usage: &disk.Usage{
			TotalBytes:             1000,
			FreeBytes:              500,
			UsedBytes:              500,
			UsedPercentFloat:       50.0,
			InodesUsedPercentFloat: 20.0,
		},
	}

	t.Run("successful check", func(t *testing.T) {
		c := New(ctx, []string{"/mnt/data1"}, []string{}).(*component)
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{mockPartition}, nil
		}

		c.CheckOnce()

		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)
		assert.True(t, lastData.healthy)
		assert.Equal(t, "found 1 ext4 partition(s) and 1 block device(s)", lastData.reason)
		assert.Len(t, lastData.BlockDevices, 1)
		assert.Len(t, lastData.ExtPartitions, 1)
	})

	t.Run("no block devices", func(t *testing.T) {
		c := New(ctx, []string{}, []string{}).(*component)
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{}, nil
		}

		c.CheckOnce()

		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)
		assert.True(t, lastData.healthy)
		assert.Equal(t, "no block device found", lastData.reason)
	})

	t.Run("no ext4 partitions", func(t *testing.T) {
		c := New(ctx, []string{}, []string{}).(*component)
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		}

		c.CheckOnce()

		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)
		assert.True(t, lastData.healthy)
		assert.Equal(t, "no ext4 partition found", lastData.reason)
	})
}

func TestErrorRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockDevice := disk.BlockDevice{
		Name: "sda",
		Type: "disk",
	}
	mockPartition := disk.Partition{
		Device:     "/dev/sda1",
		MountPoint: "/mnt/data1",
		Usage:      &disk.Usage{},
	}

	t.Run("retry on block device error", func(t *testing.T) {
		c := New(ctx, []string{}, []string{}).(*component)
		defer c.Close()

		callCount := 0
		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			callCount++
			if callCount == 1 {
				return nil, assert.AnError
			}
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{mockPartition}, nil
		}

		c.CheckOnce()

		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)
		assert.True(t, lastData.healthy)
		assert.Equal(t, "found 1 ext4 partition(s) and 1 block device(s)", lastData.reason)
		assert.Equal(t, 2, callCount)
	})

	t.Run("retry on partition error", func(t *testing.T) {
		c := New(ctx, []string{}, []string{}).(*component)
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}

		callCount := 0
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			callCount++
			if callCount == 1 {
				return nil, assert.AnError
			}
			return disk.Partitions{mockPartition}, nil
		}

		c.CheckOnce()

		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)
		assert.True(t, lastData.healthy)
		assert.Equal(t, "found 1 ext4 partition(s) and 1 block device(s)", lastData.reason)
		assert.Equal(t, 2, callCount)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctxWithCancel, ctxCancel := context.WithCancel(context.Background())
		c := New(ctxWithCancel, []string{}, []string{}).(*component)

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			ctxCancel()
			return nil, assert.AnError
		}

		c.CheckOnce()

		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)
		assert.False(t, lastData.healthy)
		assert.NotNil(t, lastData.err)
		assert.Contains(t, lastData.err.Error(), "context canceled")
	})
}

func TestMountTargetUsages(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "sda",
		Type: "disk",
	}
	mockPartition := disk.Partition{
		Device:     "/dev/sda1",
		MountPoint: "/mnt/data1",
		Usage:      &disk.Usage{},
	}
	mockMountOutput := disk.FindMntOutput{
		Target: "/dev/sda1",
		Filesystems: []disk.FoundMnt{
			{
				MountedPoint: "/mnt/data1",
				Fstype:       "ext4",
				Sources:      []string{"/dev/sda1"},
			},
		},
	}

	t.Run("track mount target", func(t *testing.T) {
		// Create a temp dir to use as mount target
		tempDir := t.TempDir()

		c := New(ctx, []string{}, []string{tempDir}).(*component)
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{mockPartition}, nil
		}
		c.findMntFunc = func(ctx context.Context, target string) (*disk.FindMntOutput, error) {
			return &mockMountOutput, nil
		}

		c.CheckOnce()

		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)
		assert.True(t, lastData.healthy)
		assert.Contains(t, lastData.MountTargetUsages, tempDir)
		assert.Equal(t, mockMountOutput, lastData.MountTargetUsages[tempDir])
	})

	t.Run("mount target error handling", func(t *testing.T) {
		// Create a temp dir to use as mount target
		tempDir := t.TempDir()

		c := New(ctx, []string{}, []string{tempDir}).(*component)
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{mockPartition}, nil
		}
		c.findMntFunc = func(ctx context.Context, target string) (*disk.FindMntOutput, error) {
			return nil, assert.AnError
		}

		c.CheckOnce()

		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)
		assert.True(t, lastData.healthy)
		assert.Nil(t, lastData.MountTargetUsages)
	})

	t.Run("non-existent mount target", func(t *testing.T) {
		nonExistentDir := "/path/that/doesnt/exist"

		c := New(ctx, []string{}, []string{nonExistentDir}).(*component)
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{mockPartition}, nil
		}

		c.CheckOnce()

		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)
		assert.True(t, lastData.healthy)
		assert.Nil(t, lastData.MountTargetUsages)
	})
}

// Test nil data handling in the Data type methods
func TestNilDataHandling(t *testing.T) {
	var nilData *Data

	states, err := nilData.getStates()
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Equal(t, "no data yet", states[0].Reason)
	assert.True(t, states[0].DeprecatedHealthy)
}

// Test metrics tracking for mount points
func TestMetricsTracking(t *testing.T) {
	ctx := context.Background()
	mockPartition := disk.Partition{
		Device:     "/dev/sda1",
		MountPoint: "/mnt/data1",
		Usage: &disk.Usage{
			TotalBytes:             1000,
			FreeBytes:              500,
			UsedBytes:              500,
			UsedPercentFloat:       50.0,
			InodesUsedPercentFloat: 20.0,
		},
	}

	c := New(ctx, []string{"/mnt/data1"}, []string{}).(*component)
	defer c.Close()

	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{
			{
				Name: "sda",
				Type: "disk",
			},
		}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{mockPartition}, nil
	}

	c.CheckOnce()

	// Check that the component is tracking the mount point correctly
	assert.Contains(t, c.mountPointsToTrackUsage, "/mnt/data1")

	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()

	// Ensure data was collected
	assert.NotNil(t, lastData)
	assert.True(t, lastData.healthy)
	assert.Len(t, lastData.ExtPartitions, 1)
	assert.Equal(t, mockPartition.MountPoint, lastData.ExtPartitions[0].MountPoint)
}
