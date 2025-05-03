package disk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Equal(t, "found 1 ext4 partitions and 1 block devices", states[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Contains(t, states[0].ExtraInfo, "data")
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

	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "disk", states[0].Name)
	assert.Contains(t, states[0].Error, "failed to get disk data")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.NotContains(t, states[0].ExtraInfo, "data")
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
		assert.Equal(t, "ok", lastCheckResult.reason)
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
		assert.Equal(t, "ok", lastCheckResult.reason)
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
		assert.Equal(t, "ok", lastCheckResult.reason)
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

	states := nilData.HealthStates()
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
	assert.Equal(t, apiv1.HealthStateTypeHealthy, rs.HealthStateType())

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

		assert.Contains(t, result, "GB") // Total size in GB
		assert.Contains(t, result, "MB") // Free and used sizes in MB or GB
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

func TestFindMntRetryLogic(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "gpud-disk-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a file in the directory to ensure it exists and has content
	testFile := tempDir + "/testfile"
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	tests := []struct {
		name              string
		failCount         int
		expectSuccess     bool
		cancelAfter       time.Duration
		expectHealthState apiv1.HealthStateType
	}{
		{
			name:              "succeeds first try",
			failCount:         0,
			expectSuccess:     true,
			expectHealthState: apiv1.HealthStateTypeHealthy,
		},
		{
			name:              "succeeds after 3 retries",
			failCount:         3,
			expectSuccess:     true,
			expectHealthState: apiv1.HealthStateTypeHealthy,
		},
		{
			name:              "fails all 5 attempts",
			failCount:         5,
			expectSuccess:     false,
			expectHealthState: apiv1.HealthStateTypeHealthy, // Component remains healthy even if findMnt fails
		},
		{
			name:              "context canceled during retry",
			failCount:         3,
			cancelAfter:       1 * time.Second,
			expectSuccess:     false,
			expectHealthState: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Track number of function calls
			callCount := 0

			mockFindMntFunc := func(ctx context.Context, target string) (*disk.FindMntOutput, error) {
				callCount++
				if callCount <= tc.failCount {
					return nil, errors.New("mock error")
				}
				return &disk.FindMntOutput{
					Filesystems: []disk.FoundMnt{
						{
							Sources:              []string{"/dev/sda1"},
							SizeHumanized:        "100G",
							AvailableHumanized:   "50G",
							UsedHumanized:        "50G",
							UsedPercentHumanized: "50%",
						},
					},
				}, nil
			}

			// Create component with mock functions
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := &component{
				ctx:         ctx,
				cancel:      cancel,
				findMntFunc: mockFindMntFunc,
				getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
					return disk.Partitions{
						{
							Device:     "/dev/sda1",
							MountPoint: "/",
							Usage: &disk.Usage{
								TotalBytes: 100 * 1024 * 1024 * 1024,
								UsedBytes:  50 * 1024 * 1024 * 1024,
								FreeBytes:  50 * 1024 * 1024 * 1024,
							},
						},
					}, nil
				},
				mountPointsToTrackUsage: map[string]struct{}{
					tempDir: {},
				},
			}

			// Set up cancellation if needed
			if tc.cancelAfter > 0 {
				time.AfterFunc(tc.cancelAfter, cancel)
			}

			// Run the Check method
			result := c.Check()
			cr, ok := result.(*checkResult)
			require.True(t, ok)

			// Verify health state
			assert.Equal(t, tc.expectHealthState, cr.health)

			// Verify call count
			if tc.expectSuccess {
				assert.LessOrEqual(t, tc.failCount+1, callCount, "Expected at least failCount+1 calls")
				assert.NotNil(t, cr.MountTargetUsages)
				assert.Contains(t, cr.MountTargetUsages, tempDir)
			} else if tc.cancelAfter > 0 {
				assert.LessOrEqual(t, callCount, tc.failCount+1, "Expected at most failCount+1 calls due to cancellation")
				assert.Equal(t, context.Canceled, cr.err)
			} else {
				assert.Equal(t, 5, callCount, "Expected exactly 5 calls for complete failure")
				// MountTargetUsages may still be nil or not contain the temp dir
			}
		})
	}
}

func TestMountTargetUsagesInitialization(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "gpud-disk-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a file in the directory to ensure it exists and has content
	testFile := tempDir + "/testfile"
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Test that MountTargetUsages is properly initialized
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &component{
		ctx:    ctx,
		cancel: cancel,
		findMntFunc: func(ctx context.Context, target string) (*disk.FindMntOutput, error) {
			return &disk.FindMntOutput{
				Filesystems: []disk.FoundMnt{
					{
						Sources:              []string{"/dev/sda1"},
						SizeHumanized:        "100G",
						AvailableHumanized:   "50G",
						UsedHumanized:        "50G",
						UsedPercentHumanized: "50%",
					},
				},
			}, nil
		},
		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Usage: &disk.Usage{
						TotalBytes: 100 * 1024 * 1024 * 1024,
						UsedBytes:  50 * 1024 * 1024 * 1024,
						FreeBytes:  50 * 1024 * 1024 * 1024,
					},
				},
			}, nil
		},
		mountPointsToTrackUsage: map[string]struct{}{
			tempDir: {},
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.NotNil(t, cr.MountTargetUsages, "MountTargetUsages should be initialized")
	assert.Contains(t, cr.MountTargetUsages, tempDir, "MountTargetUsages should contain the tempDir")
	assert.Len(t, cr.MountTargetUsages[tempDir].Filesystems, 1, "Should have one filesystem entry")
	assert.Equal(t, "/dev/sda1", cr.MountTargetUsages[tempDir].Filesystems[0].Sources[0])
}

func TestFindMntLogging(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "gpud-disk-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a file in the directory to ensure it exists and has content
	testFile := tempDir + "/testfile"
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Test logging on retry success
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	callCount := 0
	c := &component{
		ctx:    ctx,
		cancel: cancel,
		findMntFunc: func(ctx context.Context, target string) (*disk.FindMntOutput, error) {
			callCount++
			if callCount == 1 {
				return nil, errors.New("mock error")
			}
			return &disk.FindMntOutput{
				Filesystems: []disk.FoundMnt{
					{
						Sources:              []string{"/dev/sda1"},
						SizeHumanized:        "100G",
						AvailableHumanized:   "50G",
						UsedHumanized:        "50G",
						UsedPercentHumanized: "50%",
					},
				},
			}, nil
		},
		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Usage: &disk.Usage{
						TotalBytes: 100 * 1024 * 1024 * 1024,
						UsedBytes:  50 * 1024 * 1024 * 1024,
						FreeBytes:  50 * 1024 * 1024 * 1024,
					},
				},
			}, nil
		},
		mountPointsToTrackUsage: map[string]struct{}{
			tempDir: {},
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, 2, callCount, "Expected 2 calls")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.NotNil(t, cr.MountTargetUsages)
	assert.Contains(t, cr.MountTargetUsages, tempDir)
}
