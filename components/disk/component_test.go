package disk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
)

// mockEventStore implements a mock for eventstore.Store
type mockEventStore struct {
	mock.Mock
}

func (m *mockEventStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	args := m.Called(name) // Do not pass opts to m.Called for simplicity unless needed
	var bucket eventstore.Bucket
	if args.Get(0) != nil {
		bucket = args.Get(0).(eventstore.Bucket)
	}
	return bucket, args.Error(1)
}

// mockEventBucket implements a mock for eventstore.Bucket
type mockEventBucket struct {
	mock.Mock
}

func (m *mockEventBucket) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockEventBucket) Insert(ctx context.Context, event eventstore.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *mockEventBucket) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	args := m.Called(ctx, event)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*eventstore.Event), args.Error(1)
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time, opts ...eventstore.OpOption) (eventstore.Events, error) {
	args := m.Called(ctx, since)
	var events eventstore.Events
	if args.Get(0) != nil {
		events = args.Get(0).(eventstore.Events)
	}
	return events, args.Error(1)
}

func (m *mockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*eventstore.Event), args.Error(1)
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	args := m.Called(ctx, beforeTimestamp)
	return args.Int(0), args.Error(1)
}

func (m *mockEventBucket) Close() {
	m.Called()
}

// MockKmsgSyncer implements a mock for kmsg.Syncer's Close method
type MockKmsgSyncer struct {
	mock.Mock
	kmsg.Syncer // Embed Syncer if it has other methods needed, or define them
}

func (m *MockKmsgSyncer) Close() error {
	args := m.Called()
	return args.Error(0)
}

// createTestComponent creates a test component with the given mount points and targets
func createTestComponent(ctx context.Context, mountPoints, mountTargets []string) *component {
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		MountPoints:  mountPoints,
		MountTargets: mountTargets,
	}
	c, _ := New(gpudInstance)
	ct := c.(*component)
	ct.retryInterval = 0
	return ct
}

func TestComponentName(t *testing.T) {
	ctx := context.Background()
	c := createTestComponent(ctx, []string{}, []string{})
	defer c.Close()

	assert.Equal(t, Name, c.Name())
}

func TestTags(t *testing.T) {
	ctx := context.Background()
	c := createTestComponent(ctx, []string{}, []string{})
	defer c.Close()

	expectedTags := []string{
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 1, "Component should return exactly 1 tag")
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
		c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		}

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Equal(t, "no ext4/nfs partition found", lastCheckResult.reason)
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

		// The table will contain the mount point, but with n/a values
		assert.Contains(t, result, "/mnt/data1")
		assert.Contains(t, result, "n/a")
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

		// Both mount points will be in the output, but one with real values and one with n/a
		assert.Contains(t, result, "/mnt/data1")
		assert.Contains(t, result, "/mnt/data2")
		assert.Contains(t, result, "n/a")
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
				getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
					return disk.Partitions{}, nil
				},
				getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
					return disk.BlockDevices{
						{
							Name: "sda",
							Type: "disk",
						},
					}, nil
				},
				mountPointsToTrackUsage: map[string]struct{}{
					tempDir: {},
				},
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
		getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		},
		getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{
				{
					Name: "sda",
					Type: "disk",
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
		getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		},
		getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{
				{
					Name: "sda",
					Type: "disk",
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

// TestNFSPartitionsRetrieval tests the getNFSPartitionsFunc functionality
func TestNFSPartitionsRetrieval(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "nfs",
		Type: "disk",
	}
	mockNFSPartition := disk.Partition{
		Device:     "192.168.1.100:/shared",
		MountPoint: "/mnt/nfs",
		Fstype:     "fuse.juicefs",
		Usage: &disk.Usage{
			TotalBytes: 2000,
			FreeBytes:  1000,
			UsedBytes:  1000,
		},
	}

	t.Run("successful NFS partitions retrieval", func(t *testing.T) {
		c := createTestComponent(ctx, []string{"/mnt/nfs"}, []string{})
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{mockNFSPartition}, nil
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
		assert.Equal(t, "ok", lastCheckResult.reason)
		assert.Len(t, lastCheckResult.NFSPartitions, 1)
		assert.Equal(t, mockNFSPartition.Device, lastCheckResult.NFSPartitions[0].Device)
		assert.Equal(t, mockNFSPartition.MountPoint, lastCheckResult.NFSPartitions[0].MountPoint)
	})

	t.Run("no NFS partitions", func(t *testing.T) {
		c := createTestComponent(ctx, []string{}, []string{})
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
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
		assert.Equal(t, "no ext4/nfs partition found", lastCheckResult.reason)
	})
}

// TestNFSPartitionsErrorRetry tests error handling and retry logic for NFS partitions
func TestNFSPartitionsErrorRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockDevice := disk.BlockDevice{
		Name: "nfs",
		Type: "disk",
	}
	mockNFSPartition := disk.Partition{
		Device:     "192.168.1.100:/shared",
		MountPoint: "/mnt/nfs",
		Fstype:     "fuse.juicefs",
		Usage: &disk.Usage{
			TotalBytes: 2000,
			FreeBytes:  1000,
			UsedBytes:  1000,
		},
	}

	t.Run("retry on NFS partition error", func(t *testing.T) {
		c := createTestComponent(ctx, []string{"/mnt/nfs"}, []string{})
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		}

		callCount := 0
		c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			callCount++
			if callCount == 1 {
				return nil, assert.AnError
			}
			return disk.Partitions{mockNFSPartition}, nil
		}

		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Equal(t, "ok", lastCheckResult.reason)
		assert.Equal(t, 2, callCount)
		assert.Len(t, lastCheckResult.NFSPartitions, 1)
	})

	t.Run("context cancellation during NFS partitions", func(t *testing.T) {
		ctxWithCancel, ctxCancel := context.WithCancel(context.Background())
		c := createTestComponent(ctxWithCancel, []string{"/mnt/nfs"}, []string{})
		defer c.Close()

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		}

		var contextCanceled bool
		c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			if !contextCanceled {
				ctxCancel()
				contextCanceled = true
			}
			return nil, context.Canceled
		}

		checkDone := make(chan struct{})
		go func() {
			c.Check()
			close(checkDone)
		}()
		select {
		case <-checkDone:
		case <-time.After(time.Second):
			assert.Fail(t, "Check() did not complete within timeout")
		}

		// Ensure context cancellation was detected
		assert.True(t, contextCanceled, "getNFSPartitionsFunc should have been called")

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, lastCheckResult.health)
		if assert.NotNil(t, lastCheckResult.err) {
			assert.Contains(t, lastCheckResult.err.Error(), "context canceled")
		}
	})
}

// TestNFSMetricsTracking tests metrics tracking for NFS mount points
func TestNFSMetricsTracking(t *testing.T) {
	ctx := context.Background()
	mockNFSPartition := disk.Partition{
		Device:     "192.168.1.100:/shared",
		MountPoint: "/mnt/nfs",
		Fstype:     "fuse.juicefs",
		Usage: &disk.Usage{
			TotalBytes: 2000,
			FreeBytes:  1000,
			UsedBytes:  1000,
		},
	}

	c := createTestComponent(ctx, []string{"/mnt/nfs"}, []string{})
	defer c.Close()

	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{
			{
				Name: "nfs",
				Type: "disk",
			},
		}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{}, nil
	}
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{mockNFSPartition}, nil
	}

	c.Check()

	// Check that the component is tracking the mount point correctly
	assert.Contains(t, c.mountPointsToTrackUsage, "/mnt/nfs")

	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	// Ensure data was collected
	assert.NotNil(t, lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
	assert.Len(t, lastCheckResult.NFSPartitions, 1)
	assert.Equal(t, mockNFSPartition.MountPoint, lastCheckResult.NFSPartitions[0].MountPoint)
	assert.Equal(t, mockNFSPartition.Device, lastCheckResult.NFSPartitions[0].Device)
}

// TestCombinedPartitions tests the case when both ext4 and NFS partitions are present
func TestCombinedPartitions(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "sda",
		Type: "disk",
	}
	mockExt4Partition := disk.Partition{
		Device:     "/dev/sda1",
		MountPoint: "/mnt/data1",
		Fstype:     "ext4",
		Usage: &disk.Usage{
			TotalBytes: 1000,
			FreeBytes:  500,
			UsedBytes:  500,
		},
	}
	mockNFSPartition := disk.Partition{
		Device:     "192.168.1.100:/shared",
		MountPoint: "/mnt/nfs",
		Fstype:     "fuse.juicefs",
		Usage: &disk.Usage{
			TotalBytes: 2000,
			FreeBytes:  1000,
			UsedBytes:  1000,
		},
	}

	c := createTestComponent(ctx, []string{"/mnt/data1", "/mnt/nfs"}, []string{})
	defer c.Close()

	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{mockDevice}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{mockExt4Partition}, nil
	}
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{mockNFSPartition}, nil
	}

	c.Check()

	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	assert.NotNil(t, lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
	assert.Equal(t, "ok", lastCheckResult.reason)
	assert.Len(t, lastCheckResult.ExtPartitions, 1)
	assert.Len(t, lastCheckResult.NFSPartitions, 1)
	assert.Equal(t, mockExt4Partition.MountPoint, lastCheckResult.ExtPartitions[0].MountPoint)
	assert.Equal(t, mockNFSPartition.MountPoint, lastCheckResult.NFSPartitions[0].MountPoint)
}

// TestDeviceUsagesWithNFS tests the device usages calculation with NFS partitions
func TestDeviceUsagesWithNFS(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "sda",
		Type: "disk",
	}
	mockNFSPartition := disk.Partition{
		Device:     "192.168.1.100:/shared",
		MountPoint: "/mnt/nfs",
		Fstype:     "fuse.juicefs",
		Usage: &disk.Usage{
			TotalBytes: 2000,
			FreeBytes:  1000,
			UsedBytes:  1000,
		},
	}

	c := createTestComponent(ctx, []string{"/mnt/nfs"}, []string{})
	defer c.Close()

	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{mockDevice}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{}, nil
	}
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{mockNFSPartition}, nil
	}

	c.Check()

	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	assert.NotNil(t, lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
	assert.Len(t, lastCheckResult.DeviceUsages, 1)
	assert.Equal(t, mockNFSPartition.Device, lastCheckResult.DeviceUsages[0].DeviceName)
	assert.Equal(t, mockNFSPartition.MountPoint, lastCheckResult.DeviceUsages[0].MountPoint)
	assert.Equal(t, mockNFSPartition.Usage.TotalBytes, lastCheckResult.DeviceUsages[0].TotalBytes)
	assert.Equal(t, mockNFSPartition.Usage.FreeBytes, lastCheckResult.DeviceUsages[0].FreeBytes)
	assert.Equal(t, mockNFSPartition.Usage.UsedBytes, lastCheckResult.DeviceUsages[0].UsedBytes)
}

// TestNFSPartitionsDeviceUsages tests that DeviceUsages are properly created from NFSPartitions
func TestNFSPartitionsDeviceUsages(t *testing.T) {
	mockNFSPartition := disk.Partition{
		Device:     "192.168.1.100:/shared",
		MountPoint: "/mnt/nfs",
		Fstype:     "fuse.juicefs",
		Usage: &disk.Usage{
			TotalBytes: 2000,
			FreeBytes:  1000,
			UsedBytes:  1000,
		},
	}

	// Create a check result with only NFS partitions
	cr := &checkResult{
		NFSPartitions: disk.Partitions{mockNFSPartition},
		BlockDevices:  disk.FlattenedBlockDevices{{Name: "nfs", Type: "disk"}},
	}

	// Manually process NFSPartitions into DeviceUsages similar to component.Check()
	if len(cr.NFSPartitions) > 0 {
		for _, p := range cr.NFSPartitions {
			usage := p.Usage
			if usage == nil {
				continue
			}

			cr.DeviceUsages = append(cr.DeviceUsages, disk.DeviceUsage{
				DeviceName: p.Device,
				MountPoint: p.MountPoint,
				TotalBytes: usage.TotalBytes,
				FreeBytes:  usage.FreeBytes,
				UsedBytes:  usage.UsedBytes,
			})
		}
	}

	// Verify the result
	assert.Len(t, cr.DeviceUsages, 1)
	assert.Equal(t, mockNFSPartition.Device, cr.DeviceUsages[0].DeviceName)
	assert.Equal(t, mockNFSPartition.MountPoint, cr.DeviceUsages[0].MountPoint)
	assert.Equal(t, mockNFSPartition.Usage.TotalBytes, cr.DeviceUsages[0].TotalBytes)
	assert.Equal(t, mockNFSPartition.Usage.FreeBytes, cr.DeviceUsages[0].FreeBytes)
	assert.Equal(t, mockNFSPartition.Usage.UsedBytes, cr.DeviceUsages[0].UsedBytes)
}

// TestNFSPartitionsStringMethod tests the String() method of checkResult when it contains NFSPartitions
func TestNFSPartitionsStringMethod(t *testing.T) {
	// The String() method checks for len(ExtPartitions) == 0 and returns an empty string if true
	// So we need to have at least one ExtPartition to see the NFSPartitions output
	cr := &checkResult{
		ExtPartitions: disk.Partitions{
			{
				Device:     "/dev/sda1",
				MountPoint: "/mnt/data1",
				Fstype:     "ext4",
				Usage: &disk.Usage{
					TotalBytes: 1024 * 1024 * 1024,
					FreeBytes:  512 * 1024 * 1024,
					UsedBytes:  512 * 1024 * 1024,
				},
			},
		},
		NFSPartitions: disk.Partitions{
			{
				Device:     "192.168.1.100:/shared",
				MountPoint: "/mnt/nfs1",
				Fstype:     "fuse.juicefs",
				Usage: &disk.Usage{
					TotalBytes: 2 * 1024 * 1024 * 1024,
					FreeBytes:  1 * 1024 * 1024 * 1024,
					UsedBytes:  1 * 1024 * 1024 * 1024,
				},
			},
		},
	}

	result := cr.String()

	// Should contain both ext4 and NFS mount points
	assert.Contains(t, result, "/mnt/data1")
	assert.Contains(t, result, "/mnt/nfs1")
	assert.Contains(t, result, "/dev/sda1")
	assert.Contains(t, result, "192.168.1.100:/shared")

	// Test just the NFSPartitions.RenderTable directly
	buf := bytes.NewBuffer(nil)
	cr.NFSPartitions.RenderTable(buf)
	nfsOutput := buf.String()

	assert.Contains(t, nfsOutput, "/mnt/nfs1")
	assert.Contains(t, nfsOutput, "192.168.1.100:/shared")
	assert.Contains(t, nfsOutput, "MOUNT POINT")
	assert.Contains(t, nfsOutput, "TOTAL")
	assert.Contains(t, nfsOutput, "FREE")
	assert.Contains(t, nfsOutput, "USED")
}

// TestComponentIsSupported tests the IsSupported method
func TestComponentIsSupported(t *testing.T) {
	ctx := context.Background()
	c := createTestComponent(ctx, []string{}, []string{})
	defer c.Close()

	isSupported := c.IsSupported()
	assert.True(t, isSupported, "disk component should be supported")
}

// TestComponentName tests the ComponentName method
func TestComponentNameMethod(t *testing.T) {
	cr := &checkResult{}
	name := cr.ComponentName()
	assert.Equal(t, Name, name)
}

// TestSummary tests the Summary method
func TestSummary(t *testing.T) {
	t.Run("nil checkResult", func(t *testing.T) {
		var cr *checkResult
		summary := cr.Summary()
		assert.Equal(t, "", summary)
	})

	t.Run("with reason", func(t *testing.T) {
		cr := &checkResult{
			reason: "test reason",
		}
		summary := cr.Summary()
		assert.Equal(t, "test reason", summary)
	})

	t.Run("empty reason", func(t *testing.T) {
		cr := &checkResult{}
		summary := cr.Summary()
		assert.Equal(t, "", summary)
	})
}

// TestHealthStateType tests the HealthStateType method
func TestHealthStateType(t *testing.T) {
	t.Run("nil checkResult", func(t *testing.T) {
		var cr *checkResult
		health := cr.HealthStateType()
		assert.Equal(t, apiv1.HealthStateType(""), health)
	})

	t.Run("with health state", func(t *testing.T) {
		cr := &checkResult{
			health: apiv1.HealthStateTypeHealthy,
		}
		health := cr.HealthStateType()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, health)
	})

	t.Run("empty health state", func(t *testing.T) {
		cr := &checkResult{}
		health := cr.HealthStateType()
		assert.Equal(t, apiv1.HealthStateType(""), health)
	})
}

// Additional test cases for String() method to improve coverage
func TestCheckResultStringMoreCases(t *testing.T) {
	t.Run("with all types of data", func(t *testing.T) {
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
			},
			NFSPartitions: disk.Partitions{
				{
					Device:     "192.168.1.100:/shared",
					MountPoint: "/mnt/nfs",
					Usage: &disk.Usage{
						TotalBytes: 2 * 1024 * 1024 * 1024,
						FreeBytes:  1 * 1024 * 1024 * 1024,
						UsedBytes:  1 * 1024 * 1024 * 1024,
					},
				},
			},
			BlockDevices: disk.FlattenedBlockDevices{
				{
					Name: "sda",
					Type: "disk",
				},
			},
			DeviceUsages: disk.DeviceUsages{
				{
					DeviceName: "/dev/sda1",
					MountPoint: "/mnt/data1",
					TotalBytes: 1024 * 1024 * 1024,
					FreeBytes:  512 * 1024 * 1024,
					UsedBytes:  512 * 1024 * 1024,
				},
			},
			MountTargetUsages: map[string]disk.FindMntOutput{
				"/mnt/target": {
					Target: "/mnt/target",
					Filesystems: []disk.FoundMnt{
						{
							MountedPoint:         "/mnt/target",
							Sources:              []string{"/dev/sda1"},
							SizeHumanized:        "1G",
							AvailableHumanized:   "512M",
							UsedHumanized:        "512M",
							UsedPercentHumanized: "50%",
						},
					},
				},
			},
		}

		result := cr.String()

		// Verify all sections are included
		assert.Contains(t, result, "/mnt/data1")
		assert.Contains(t, result, "/mnt/nfs")
		assert.Contains(t, result, "sda")
		assert.Contains(t, result, "/dev/sda1")
		assert.Contains(t, result, "/mnt/target")

		// Tables should contain appropriate headers
		assert.Contains(t, result, "MOUNT POINT")
		assert.Contains(t, result, "DEVICE")
		assert.Contains(t, result, "TOTAL")
		assert.Contains(t, result, "NAME")
		assert.Contains(t, result, "TYPE")

		// For MountTargetUsages table
		assert.Contains(t, result, "USED %")
	})
}

// Test the New function more extensively
func TestNewComponentMoreCases(t *testing.T) {
	// Test with specific mount points and targets
	ctx := context.Background()
	mountPoints := []string{"/mnt/data1", "/mnt/data2"}
	mountTargets := []string{"/mnt/target1", "/mnt/target2"}

	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		MountPoints:  mountPoints,
		MountTargets: mountTargets,
	}

	c, err := New(gpudInstance)
	require.NoError(t, err)
	defer c.Close()

	// Check component type
	component, ok := c.(*component)
	require.True(t, ok)

	// Check if mount points are correctly tracked
	assert.Contains(t, component.mountPointsToTrackUsage, "/mnt/data1")
	assert.Contains(t, component.mountPointsToTrackUsage, "/mnt/data2")
	assert.Contains(t, component.mountPointsToTrackUsage, "/mnt/target1")
	assert.Contains(t, component.mountPointsToTrackUsage, "/mnt/target2")

	// Check if function fields are properly initialized
	assert.NotNil(t, component.getExt4PartitionsFunc)
	assert.NotNil(t, component.getNFSPartitionsFunc)
	assert.NotNil(t, component.findMntFunc)

	// On Linux, getBlockDevicesFunc should be set
	if runtime.GOOS == "linux" {
		assert.NotNil(t, component.getBlockDevicesFunc)
	}
}

// TestNew_EventStoreHandling tests the New function's handling of EventStore.
func TestNew_EventStoreHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("eventStoreIsNil", func(t *testing.T) {
		gpudInstance := &components.GPUdInstance{
			RootCtx:    ctx,
			EventStore: nil, // Explicitly nil
		}
		comp, err := New(gpudInstance)
		require.NoError(t, err)
		defer comp.Close()

		diskComp, ok := comp.(*component)
		require.True(t, ok)
		assert.Nil(t, diskComp.eventBucket, "eventBucket should be nil if EventStore is nil")
		assert.Nil(t, diskComp.kmsgSyncer, "kmsgSyncer should be nil if EventStore is nil")
	})

	t.Run("eventStoreBucketSuccess", func(t *testing.T) {
		mockEventStore := new(mockEventStore)
		mockBucket := new(mockEventBucket)

		// Set up the mock for Bucket method - this is called regardless of OS
		mockEventStore.On("Bucket", Name).Return(mockBucket, nil)
		// The bucket's Close method will be called when the component is closed
		mockBucket.On("Close").Return()

		gpudInstance := &components.GPUdInstance{
			RootCtx:    ctx,
			EventStore: mockEventStore,
		}

		comp, err := New(gpudInstance)
		require.NoError(t, err)
		require.NotNil(t, comp)
		defer comp.Close()

		diskComp, ok := comp.(*component)
		require.True(t, ok)

		// Event bucket should be created when EventStore is provided
		assert.NotNil(t, diskComp.eventBucket, "eventBucket should be initialized when EventStore is provided")
		assert.Equal(t, mockBucket, diskComp.eventBucket)
		mockEventStore.AssertCalled(t, "Bucket", Name)

		// kmsgSyncer is only created on Linux when running as root
		if runtime.GOOS == "linux" && os.Geteuid() == 0 {
			// kmsgSyncer might be set on Linux as root
			// We can't easily test this without mocking kmsg.NewSyncer
		} else {
			assert.Nil(t, diskComp.kmsgSyncer, "kmsgSyncer should be nil when not on Linux as root")
		}
	})

	t.Run("eventStoreBucketError", func(t *testing.T) {
		mockEventStore := new(mockEventStore)
		expectedErr := errors.New("bucket error")

		// Set up the mock to return an error
		mockEventStore.On("Bucket", Name).Return(nil, expectedErr)

		gpudInstance := &components.GPUdInstance{
			RootCtx:    ctx,
			EventStore: mockEventStore,
		}

		comp, err := New(gpudInstance)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, comp)
		mockEventStore.AssertCalled(t, "Bucket", Name)
	})

	// Note: Testing kmsg.NewSyncer success/failure path requires runtime.GOOS == "linux"
	// and os.Geteuid() == 0, which is hard to control portably in tests without
	// modifying the source code for dependency injection of kmsg.NewSyncer itself or os.Geteuid.
	// The coverage for those lines might be 0 if tests are not run in such an environment.
}

// TestComponentEvents_WithEventBucket tests the Events method with a focus on eventBucket.
func TestComponentEvents_WithEventBucket(t *testing.T) {
	ctx := context.Background()

	t.Run("eventBucketIsNil", func(t *testing.T) {
		// This scenario is implicitly tested by createTestComponent when no EventStore is set up,
		// leading to c.eventBucket being nil.
		c := createTestComponent(ctx, []string{}, []string{}) // This component will have c.eventBucket = nil
		defer c.Close()

		evs, err := c.Events(ctx, time.Now())
		assert.NoError(t, err)
		assert.Nil(t, evs)
	})

	t.Run("eventBucketGetError", func(t *testing.T) {
		mockBucket := new(mockEventBucket)
		expectedErr := errors.New("get error")
		sinceTime := time.Now().Add(-1 * time.Hour)
		// Use mock.Anything for the context to avoid type mismatches with unexported context types
		mockBucket.On("Get", mock.Anything, sinceTime).Return(nil, expectedErr)
		// If eventBucket is set and comp.Close() is called, bucket.Close() will be invoked.
		mockBucket.On("Close").Return().Maybe()

		// Create component and manually set the eventBucket
		c := createTestComponent(ctx, []string{}, []string{})
		defer c.Close()
		c.eventBucket = mockBucket // Manually set mock bucket

		evs, err := c.Events(ctx, sinceTime)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, evs)
		mockBucket.AssertCalled(t, "Get", mock.Anything, sinceTime)
	})

	t.Run("eventBucketGetSuccess", func(t *testing.T) {
		mockBucket := new(mockEventBucket)
		expectedEvents := eventstore.Events{
			{Time: time.Now(), Name: "test_event", Type: "type1", Message: "message1", Component: Name},
		}
		sinceTime := time.Now().Add(-1 * time.Hour)
		// Use mock.Anything for the context
		mockBucket.On("Get", mock.Anything, sinceTime).Return(expectedEvents, nil)
		// If eventBucket is set and comp.Close() is called, bucket.Close() will be invoked.
		mockBucket.On("Close").Return().Maybe()

		c := createTestComponent(ctx, []string{}, []string{})
		defer c.Close()
		c.eventBucket = mockBucket // Manually set mock bucket

		evs, err := c.Events(ctx, sinceTime)
		assert.NoError(t, err)
		require.NotNil(t, evs)
		assert.Equal(t, expectedEvents.Events(), evs) // Compare apiv1.Events
		mockBucket.AssertCalled(t, "Get", mock.Anything, sinceTime)
	})
}

// TestComponentClose_EventHandling tests the Close method's handling of eventBucket and kmsgSyncer.
func TestComponentClose_EventHandling(t *testing.T) {
	ctx := context.Background()

	// Renamed from closeWithEventBucketAndKmsgSyncerNotNil
	t.Run("closeWithEventBucketSetAndKmsgSyncerNil", func(t *testing.T) {
		mockBucket := new(mockEventBucket)
		mockBucket.On("Close").Return()

		cctx, ccancel := context.WithCancel(ctx)
		c := &component{
			ctx:         cctx,
			cancel:      ccancel,
			eventBucket: mockBucket,
			kmsgSyncer:  nil, // Changed from &kmsg.Syncer{} to nil to avoid panic
		}

		err := c.Close()
		assert.NoError(t, err)
		mockBucket.AssertCalled(t, "Close")
	})

	// The subtest "closeWithEventBucketOnly_KmsgSyncerNil" is identical to the one above after the change.
	// It can be removed or kept if preferred for explicitness, but for consolidation, we rely on the above.

	// Renamed from closeWithKmsgSyncerNotNilOnly_EventBucketNil
	// This also becomes similar to "closeWithBothEventBucketAndKmsgSyncerNil"
	t.Run("closeWithEventBucketNilAndKmsgSyncerNil", func(t *testing.T) {
		cctx, ccancel := context.WithCancel(ctx)
		c := &component{
			ctx:         cctx,
			cancel:      ccancel,
			eventBucket: nil,
			kmsgSyncer:  nil, // Changed from &kmsg.Syncer{} to nil to avoid panic
		}

		err := c.Close()
		assert.NoError(t, err)
		// No eventBucket.Close() should be called. kmsgSyncer.Close() path also not taken.
	})

	// The subtest "closeWithBothEventBucketAndKmsgSyncerNil" is identical to the one above.
	// It can be removed for consolidation.
}
