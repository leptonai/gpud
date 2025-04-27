package disk

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/disk"
)

// createTestComponent creates a test component with the given mount points and targets
func createTestComponent(ctx context.Context, mountPoints, mountTargets []string) *component {
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		MountPoints:  mountPoints,
		MountTargets: mountTargets,
	}
	c, _ := New(gpudInstance)
	return c.(*component)
}

func TestComponentName(t *testing.T) {
	ctx := context.Background()
	c := createTestComponent(ctx, []string{}, []string{})
	defer c.Close()

	assert.Equal(t, Name, c.Name())
}

func TestNewComponent(t *testing.T) {
	ctx := context.Background()
	mountPoints := []string{"/mnt/test1"}
	mountTargets := []string{"/mnt/test2"}

	c := createTestComponent(ctx, mountPoints, mountTargets)
	defer c.Close()

	// Check if mount points are correctly added to the tracking map
	assert.Contains(t, c.mountPointsToTrackUsage, "/mnt/test1")
	assert.Contains(t, c.mountPointsToTrackUsage, "/mnt/test2")
}

func TestComponentStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := createTestComponent(ctx, []string{}, []string{})
	defer c.Close()

	err := c.Start()
	assert.NoError(t, err)
}

func TestComponentClose(t *testing.T) {
	ctx := context.Background()
	c := createTestComponent(ctx, []string{}, []string{})

	err := c.Close()
	assert.NoError(t, err)
}

func TestComponentEvents(t *testing.T) {
	ctx := context.Background()
	c := createTestComponent(ctx, []string{}, []string{})
	defer c.Close()

	events, err := c.Events(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestEmptyDataStates(t *testing.T) {
	ctx := context.Background()
	c := createTestComponent(ctx, []string{}, []string{})
	defer c.Close()

	// No data set yet
	states := c.LastHealthStates()
	assert.Equal(t, "no data yet", states[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

func TestDataGetStates(t *testing.T) {
	cr := &checkResult{
		ExtPartitions: disk.Partitions{
			{Device: "/dev/sda1", MountPoint: "/mnt/data1"},
		},
		BlockDevices: disk.FlattenedBlockDevices{
			{Name: "sda", Type: "disk"},
		},

		health: apiv1.HealthStateTypeHealthy,
		reason: "found 1 ext4 partitions and 1 block devices",
	}

	states := cr.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Equal(t, "found 1 ext4 partitions and 1 block devices", states[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Contains(t, states[0].ExtraInfo, "data")
	assert.Contains(t, states[0].ExtraInfo, "encoding")
}

func TestDataGetError(t *testing.T) {
	// Test with error
	cr := &checkResult{
		err: assert.AnError,
	}
	errStr := cr.getError()
	assert.Equal(t, assert.AnError.Error(), errStr)

	// Test without error
	cr = &checkResult{}
	errStr = cr.getError()
	assert.Empty(t, errStr)

	// Test with nil data
	var nilData *checkResult
	errStr = nilData.getError()
	assert.Empty(t, errStr)
}

func TestDataGetStatesWithError(t *testing.T) {
	cr := &checkResult{
		err:    errors.New("failed to get disk data"),
		health: apiv1.HealthStateTypeUnhealthy,
	}

	states := cr.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Contains(t, states[0].Error, "failed to get disk data")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].ExtraInfo, "data")
	assert.Contains(t, states[0].ExtraInfo, "encoding")
}

func TestComponentStatesWithError(t *testing.T) {
	ctx := context.Background()
	c := createTestComponent(ctx, []string{}, []string{})
	defer c.Close()

	// Manually set lastCheckResult with an error
	c.lastMu.Lock()
	c.lastCheckResult = &checkResult{
		err:    errors.New("failed to get disk data"),
		health: apiv1.HealthStateTypeUnhealthy,
	}
	c.lastMu.Unlock()

	states := c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Contains(t, states[0].Error, "failed to get disk data")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
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
			TotalBytes: 1000,
			FreeBytes:  500,
			UsedBytes:  500,
		},
	}

	t.Run("successful check", func(t *testing.T) {
		c := createTestComponent(ctx, []string{"/mnt/data1"}, []string{})
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{mockPartition}, nil
		}

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Equal(t, "found 1 ext4 partition(s) and 1 block device(s)", lastCheckResult.reason)
		assert.Len(t, lastCheckResult.BlockDevices, 1)
		assert.Len(t, lastCheckResult.ExtPartitions, 1)
	})

	t.Run("no block devices", func(t *testing.T) {
		c := createTestComponent(ctx, []string{}, []string{})
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{}, nil
		}

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Equal(t, "no block device found", lastCheckResult.reason)
	})

	t.Run("no ext4 partitions", func(t *testing.T) {
		c := createTestComponent(ctx, []string{}, []string{})
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		}

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Equal(t, "no ext4 partition found", lastCheckResult.reason)
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
		c := createTestComponent(ctx, []string{}, []string{})
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

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Equal(t, "found 1 ext4 partition(s) and 1 block device(s)", lastCheckResult.reason)
		assert.Equal(t, 2, callCount)
	})

	t.Run("retry on partition error", func(t *testing.T) {
		c := createTestComponent(ctx, []string{}, []string{})
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

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Equal(t, "found 1 ext4 partition(s) and 1 block device(s)", lastCheckResult.reason)
		assert.Equal(t, 2, callCount)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctxWithCancel, ctxCancel := context.WithCancel(context.Background())
		c := createTestComponent(ctxWithCancel, []string{}, []string{})

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			ctxCancel()
			return nil, assert.AnError
		}

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, lastCheckResult.health)
		assert.NotNil(t, lastCheckResult.err)
		assert.Contains(t, lastCheckResult.err.Error(), "context canceled")
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

		c := createTestComponent(ctx, []string{}, []string{tempDir})
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

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Contains(t, lastCheckResult.MountTargetUsages, tempDir)
		assert.Equal(t, mockMountOutput, lastCheckResult.MountTargetUsages[tempDir])
	})

	t.Run("mount target error handling", func(t *testing.T) {
		// Create a temp dir to use as mount target
		tempDir := t.TempDir()

		c := createTestComponent(ctx, []string{}, []string{tempDir})
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

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Nil(t, lastCheckResult.MountTargetUsages)
	})

	t.Run("non-existent mount target", func(t *testing.T) {
		nonExistentDir := "/path/that/doesnt/exist"

		c := createTestComponent(ctx, []string{}, []string{nonExistentDir})
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{mockPartition}, nil
		}

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Nil(t, lastCheckResult.MountTargetUsages)
	})
}

// Test nil data handling in the Data type methods
func TestNilDataHandling(t *testing.T) {
	var nilData *checkResult

	states := nilData.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Equal(t, "no data yet", states[0].Reason)
}

// Test metrics tracking for mount points
func TestMetricsTracking(t *testing.T) {
	ctx := context.Background()
	mockPartition := disk.Partition{
		Device:     "/dev/sda1",
		MountPoint: "/mnt/data1",
		Usage: &disk.Usage{
			TotalBytes: 1000,
			FreeBytes:  500,
			UsedBytes:  500,
		},
	}

	c := createTestComponent(ctx, []string{"/mnt/data1"}, []string{})
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

	c.Check()

	// Check that the component is tracking the mount point correctly
	assert.Contains(t, c.mountPointsToTrackUsage, "/mnt/data1")

	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	// Ensure data was collected
	assert.NotNil(t, lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
	assert.Len(t, lastCheckResult.ExtPartitions, 1)
	assert.Equal(t, mockPartition.MountPoint, lastCheckResult.ExtPartitions[0].MountPoint)
}

func TestCheck(t *testing.T) {
	comp, err := New(&components.GPUdInstance{
		RootCtx: context.Background(),
	})
	assert.NoError(t, err)

	rs := comp.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, rs.HealthState())

	fmt.Println(rs.String())
}

// TestCheckResultString tests the String method of checkResult
func TestCheckResultString(t *testing.T) {
	t.Run("nil checkResult", func(t *testing.T) {
		var cr *checkResult
		result := cr.String()
		assert.Equal(t, "", result)
	})

	t.Run("empty ExtPartitions", func(t *testing.T) {
		cr := &checkResult{
			ExtPartitions: disk.Partitions{},
		}
		result := cr.String()
		assert.Equal(t, "", result)
	})

	t.Run("ExtPartitions with nil Usage", func(t *testing.T) {
		cr := &checkResult{
			ExtPartitions: disk.Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/mnt/data1",
					Usage:      nil,
				},
			},
		}
		result := cr.String()
		assert.NotContains(t, result, "/mnt/data1")
	})

	t.Run("ExtPartitions with valid Usage", func(t *testing.T) {
		cr := &checkResult{
			ExtPartitions: disk.Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/mnt/data1",
					Usage: &disk.Usage{
						TotalBytes: 1024 * 1024 * 1024,
						FreeBytes:  512 * 1024 * 1024,
						UsedBytes:  512 * 1024 * 1024,
					},
				},
				{
					Device:     "/dev/sda2",
					MountPoint: "/mnt/data2",
					Usage: &disk.Usage{
						TotalBytes: 2 * 1024 * 1024 * 1024,
						FreeBytes:  1 * 1024 * 1024 * 1024,
						UsedBytes:  1 * 1024 * 1024 * 1024,
					},
				},
			},
		}
		result := cr.String()

		// Verify table contains both mount points
		assert.Contains(t, result, "/mnt/data1")
		assert.Contains(t, result, "/mnt/data2")

		// Verify header exists in uppercase
		assert.Contains(t, result, "MOUNT POINT")
		assert.Contains(t, result, "TOTAL")
		assert.Contains(t, result, "FREE")
		assert.Contains(t, result, "USED")
		assert.Contains(t, result, "USED %")

		// Verify data values - use more flexible contains checks instead of exact matches
		// since the exact formatting may vary
		assert.Contains(t, result, "GB")     // Total size in GB
		assert.Contains(t, result, "MB")     // Free and used sizes in MB or GB
		assert.Contains(t, result, "50.0 %") // Usage percentage
	})

	t.Run("mixed valid and nil Usage", func(t *testing.T) {
		cr := &checkResult{
			ExtPartitions: disk.Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/mnt/data1",
					Usage: &disk.Usage{
						TotalBytes: 1024 * 1024 * 1024,
						FreeBytes:  512 * 1024 * 1024,
						UsedBytes:  512 * 1024 * 1024,
					},
				},
				{
					Device:     "/dev/sda2",
					MountPoint: "/mnt/data2",
					Usage:      nil,
				},
			},
		}
		result := cr.String()

		// Should contain the valid mount point but not the nil one
		assert.Contains(t, result, "/mnt/data1")
		assert.NotContains(t, result, "/mnt/data2")
	})
}
