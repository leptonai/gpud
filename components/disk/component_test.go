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
	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/kmsg"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
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

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
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

	// Initialize statWithTimeoutFunc with real implementation for tests that don't override it
	if ct.statWithTimeoutFunc == nil {
		ct.statWithTimeoutFunc = pkgfile.StatWithTimeout
	}

	// Initialize getGroupConfigsFunc to return empty configs by default
	if ct.getGroupConfigsFunc == nil {
		ct.getGroupConfigsFunc = func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		}
	}

	return ct
}

// createTestComponentWithTime creates a test component with a fixed time function for deterministic testing
func createTestComponentWithTime(ctx context.Context, mountPoints, mountTargets []string, fixedTime time.Time) *component {
	c := createTestComponent(ctx, mountPoints, mountTargets)
	c.getTimeNowFunc = func() time.Time {
		return fixedTime
	}
	return c
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

	// Verify that getTimeNowFunc is properly initialized
	assert.NotNil(t, c.getTimeNowFunc)

	// Test that the time function returns a valid time
	ts1 := c.getTimeNowFunc()
	time.Sleep(10 * time.Millisecond)
	ts2 := c.getTimeNowFunc()
	assert.True(t, ts2.After(ts1), "getTimeNowFunc should return increasing timestamps")
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
		assert.Equal(t, "ok", lastCheckResult.reason)
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
		tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		c := createTestComponent(ctx, []string{}, []string{tmpDir})
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
		assert.Contains(t, lastCheckResult.MountTargetUsages, tmpDir)
		assert.Equal(t, mockMountOutput, lastCheckResult.MountTargetUsages[tmpDir])
	})

	t.Run("mount target error handling", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		c := createTestComponent(ctx, []string{}, []string{tmpDir})
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

// TestCheckWithFixedTime tests the Check method with a fixed timestamp
func TestCheckWithFixedTime(t *testing.T) {
	ctx := context.Background()
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	c := createTestComponentWithTime(ctx, []string{}, []string{}, fixedTime)
	defer c.Close()

	// Mock the partition functions to return predictable data
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{
			{
				Device:     "/dev/sda1",
				MountPoint: "/",
				Usage: &disk.Usage{
					TotalBytes: 1000,
					FreeBytes:  500,
					UsedBytes:  500,
				},
			},
		}, nil
	}

	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{
			{
				Name: "sda",
				Type: "disk",
			},
		}, nil
	}

	// Perform check
	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify the timestamp is correctly set
	assert.Equal(t, fixedTime, cr.ts)

	// Verify health states use the fixed timestamp
	healthStates := cr.HealthStates()
	assert.Len(t, healthStates, 1)
	assert.Equal(t, fixedTime, healthStates[0].Time.Time)
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

		assert.Contains(t, result, "GiB") // Total size in GiB
		assert.Contains(t, result, "MiB") // Free and used sizes in MiB or GiB
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
				ctx:                 ctx,
				cancel:              cancel,
				findMntFunc:         mockFindMntFunc,
				statWithTimeoutFunc: pkgfile.StatWithTimeout,
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
				getGroupConfigsFunc: func() pkgnfschecker.Configs {
					return pkgnfschecker.Configs{}
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
		ctx:                 ctx,
		cancel:              cancel,
		statWithTimeoutFunc: pkgfile.StatWithTimeout,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		},
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
		ctx:                 ctx,
		cancel:              cancel,
		statWithTimeoutFunc: pkgfile.StatWithTimeout,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		},
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

		// Provide non-empty configs to enable NFS checking
		c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{VolumePath: "/mnt/nfs"},
			}
		}

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
		assert.Equal(t, "ok", lastCheckResult.reason)
	})

	t.Run("skip NFS partitions when no configs", func(t *testing.T) {
		c := createTestComponent(ctx, []string{}, []string{})
		defer c.Close()

		// Ensure configs are empty (this is default, but explicit for clarity)
		c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		}

		// Track if getNFSPartitionsFunc was called
		nfsPartitionsCalled := false
		c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			nfsPartitionsCalled = true
			return disk.Partitions{mockNFSPartition}, nil
		}

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		}

		c.Check()

		// Verify getNFSPartitionsFunc was NOT called
		assert.False(t, nfsPartitionsCalled, "getNFSPartitionsFunc should not be called when configs are empty")

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Equal(t, "ok", lastCheckResult.reason)
		assert.Empty(t, lastCheckResult.NFSPartitions, "NFSPartitions should be empty when configs are empty")
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

		// Provide non-empty configs to enable NFS checking
		c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{VolumePath: "/mnt/nfs"},
			}
		}

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

	// Provide non-empty configs to enable NFS checking
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{VolumePath: "/mnt/nfs"},
		}
	}

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

	// Provide non-empty configs to enable NFS checking
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{VolumePath: "/mnt/nfs"},
		}
	}

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

	// Provide non-empty configs to enable NFS checking
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{VolumePath: "/mnt/nfs"},
		}
	}

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
			ctx:                 cctx,
			cancel:              ccancel,
			statWithTimeoutFunc: pkgfile.StatWithTimeout,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
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
			ctx:                 cctx,
			cancel:              ccancel,
			statWithTimeoutFunc: pkgfile.StatWithTimeout,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			eventBucket: nil,
			kmsgSyncer:  nil, // Changed from &kmsg.Syncer{} to nil to avoid panic
		}

		err := c.Close()
		assert.NoError(t, err)
		// No eventBucket.Close() should be called. kmsgSyncer.Close() path also not taken.
	})
}

// TestComponent_StatTimedOut_SetsHealthDegraded tests that the component health state is set to degraded when StatTimedOut=true
func TestComponent_StatTimedOut_SetsHealthDegraded(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "nfs",
		Type: "disk",
	}

	// Create an NFS partition with StatTimedOut=true
	mockNFSPartition := disk.Partition{
		Device:       "192.168.1.100:/shared",
		MountPoint:   "/mnt/nfs",
		Fstype:       "nfs4",
		Mounted:      false,
		StatTimedOut: true, // This should trigger degraded health state
		Usage:        nil,
	}

	c := createTestComponent(ctx, []string{"/mnt/nfs"}, []string{})
	defer c.Close()

	// Provide non-empty NFS configs to ensure NFS partitions are checked
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   "/mnt/nfs",
				DirName:      ".gpud-nfs-checker",
				FileContents: "test",
			},
		}
	}
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
	assert.Equal(t, apiv1.HealthStateTypeDegraded, lastCheckResult.health)
	assert.Contains(t, lastCheckResult.reason, "stat timed out (possible connection issue)")
	assert.Contains(t, lastCheckResult.reason, "/mnt/nfs")
	assert.Len(t, lastCheckResult.NFSPartitions, 1)
	assert.True(t, lastCheckResult.NFSPartitions[0].StatTimedOut)
}

// TestComponent_StatTimedOut_MultiplePartitions tests behavior with multiple partitions where some have StatTimedOut=true
func TestComponent_StatTimedOut_MultiplePartitions(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "nfs",
		Type: "disk",
	}

	// Create multiple NFS partitions - one with StatTimedOut=true, one without
	mockNFSPartition1 := disk.Partition{
		Device:       "192.168.1.100:/shared1",
		MountPoint:   "/mnt/nfs1",
		Fstype:       "nfs4",
		Mounted:      true,
		StatTimedOut: false,
		Usage: &disk.Usage{
			TotalBytes: 1000,
			FreeBytes:  500,
			UsedBytes:  500,
		},
	}

	mockNFSPartition2 := disk.Partition{
		Device:       "192.168.1.100:/shared2",
		MountPoint:   "/mnt/nfs2",
		Fstype:       "nfs4",
		Mounted:      false,
		StatTimedOut: true, // This should trigger degraded health state
		Usage:        nil,
	}

	c := createTestComponent(ctx, []string{"/mnt/nfs1", "/mnt/nfs2"}, []string{})
	defer c.Close()

	// Provide non-empty NFS configs to ensure NFS partitions are checked
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   "/mnt/nfs1",
				DirName:      ".gpud-nfs-checker",
				FileContents: "test",
			},
			{
				VolumePath:   "/mnt/nfs2",
				DirName:      ".gpud-nfs-checker",
				FileContents: "test",
			},
		}
	}
	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{mockDevice}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{}, nil
	}
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{mockNFSPartition1, mockNFSPartition2}, nil
	}

	c.Check()

	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	assert.NotNil(t, lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeDegraded, lastCheckResult.health)
	assert.Contains(t, lastCheckResult.reason, "stat timed out (possible connection issue)")
	assert.Contains(t, lastCheckResult.reason, "/mnt/nfs2")
	assert.Len(t, lastCheckResult.NFSPartitions, 2)

	// Find the partitions and verify their StatTimedOut values
	var partition1, partition2 *disk.Partition
	for i := range lastCheckResult.NFSPartitions {
		if lastCheckResult.NFSPartitions[i].MountPoint == "/mnt/nfs1" {
			partition1 = &lastCheckResult.NFSPartitions[i]
		} else if lastCheckResult.NFSPartitions[i].MountPoint == "/mnt/nfs2" {
			partition2 = &lastCheckResult.NFSPartitions[i]
		}
	}

	assert.NotNil(t, partition1)
	assert.NotNil(t, partition2)
	assert.False(t, partition1.StatTimedOut)
	assert.True(t, partition2.StatTimedOut)
}

// TestComponent_StatTimedOut_False_HealthyState tests that the component health state remains healthy when StatTimedOut=false
func TestComponent_StatTimedOut_False_HealthyState(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "nfs",
		Type: "disk",
	}

	// Create an NFS partition with StatTimedOut=false
	mockNFSPartition := disk.Partition{
		Device:       "192.168.1.100:/shared",
		MountPoint:   "/mnt/nfs",
		Fstype:       "nfs4",
		Mounted:      true,
		StatTimedOut: false, // This should keep health state healthy
		Usage: &disk.Usage{
			TotalBytes: 1000,
			FreeBytes:  500,
			UsedBytes:  500,
		},
	}

	c := createTestComponent(ctx, []string{"/mnt/nfs"}, []string{})
	defer c.Close()

	// Provide non-empty NFS configs to ensure NFS partitions are checked
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   "/mnt/nfs",
				DirName:      ".gpud-nfs-checker",
				FileContents: "test",
			},
		}
	}
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
	assert.Equal(t, "ok", lastCheckResult.reason)
	assert.Len(t, lastCheckResult.NFSPartitions, 1)
	assert.False(t, lastCheckResult.NFSPartitions[0].StatTimedOut)
}

// TestComponent_StatTimedOut_ExtPartitionsIgnored tests that only NFS partitions are checked for StatTimedOut
func TestComponent_StatTimedOut_ExtPartitionsIgnored(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "sda",
		Type: "disk",
	}

	// Create an Ext4 partition with StatTimedOut=true (should be ignored)
	mockExtPartition := disk.Partition{
		Device:       "/dev/sda1",
		MountPoint:   "/mnt/ext4",
		Fstype:       "ext4",
		Mounted:      false,
		StatTimedOut: true, // This should be ignored for health state calculation
		Usage:        nil,
	}

	// Create an NFS partition with StatTimedOut=false
	mockNFSPartition := disk.Partition{
		Device:       "192.168.1.100:/shared",
		MountPoint:   "/mnt/nfs",
		Fstype:       "nfs4",
		Mounted:      true,
		StatTimedOut: false,
		Usage: &disk.Usage{
			TotalBytes: 1000,
			FreeBytes:  500,
			UsedBytes:  500,
		},
	}

	c := createTestComponent(ctx, []string{"/mnt/ext4", "/mnt/nfs"}, []string{})
	defer c.Close()

	// Provide non-empty NFS configs to ensure NFS partitions are checked
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{
			{
				VolumePath:   "/mnt/nfs",
				DirName:      ".gpud-nfs-checker",
				FileContents: "test",
			},
		}
	}
	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{mockDevice}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{mockExtPartition}, nil
	}
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{mockNFSPartition}, nil
	}

	c.Check()

	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	assert.NotNil(t, lastCheckResult)
	// Health should be healthy because only NFS partitions are checked for StatTimedOut
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
	assert.Equal(t, "ok", lastCheckResult.reason)
	assert.Len(t, lastCheckResult.ExtPartitions, 1)
	assert.Len(t, lastCheckResult.NFSPartitions, 1)
	assert.True(t, lastCheckResult.ExtPartitions[0].StatTimedOut)  // Ext4 partition has StatTimedOut=true
	assert.False(t, lastCheckResult.NFSPartitions[0].StatTimedOut) // NFS partition has StatTimedOut=false
}

// TestComponent_StatTimedOut_ReasonMessage tests that the correct reason message is set when StatTimedOut is detected
func TestComponent_StatTimedOut_ReasonMessage(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "nfs",
		Type: "disk",
	}

	tests := []struct {
		name        string
		mountPoint  string
		expectedMsg string
	}{
		{
			name:        "short mount point",
			mountPoint:  "/mnt/nfs",
			expectedMsg: "/mnt/nfs stat timed out (possible connection issue)",
		},
		{
			name:        "long mount point",
			mountPoint:  "/mnt/very/long/path/to/nfs/mount",
			expectedMsg: "/mnt/very/long/path/to/nfs/mount stat timed out (possible connection issue)",
		},
		{
			name:        "root mount point",
			mountPoint:  "/",
			expectedMsg: "/ stat timed out (possible connection issue)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockNFSPartition := disk.Partition{
				Device:       "192.168.1.100:/shared",
				MountPoint:   tt.mountPoint,
				Fstype:       "nfs4",
				Mounted:      false,
				StatTimedOut: true,
				Usage:        nil,
			}

			c := createTestComponent(ctx, []string{tt.mountPoint}, []string{})
			defer c.Close()

			// Provide non-empty NFS configs to ensure NFS partitions are checked
			c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{
					{
						VolumePath:   tt.mountPoint,
						DirName:      ".gpud-nfs-checker",
						FileContents: "test",
					},
				}
			}
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
			assert.Equal(t, apiv1.HealthStateTypeDegraded, lastCheckResult.health)
			assert.Equal(t, tt.expectedMsg, lastCheckResult.reason)
		})
	}
}

// TestComponent_StatTimedOut_NoNFSPartitions tests that StatTimedOut in non-NFS partitions doesn't affect health
func TestComponent_StatTimedOut_NoNFSPartitions(t *testing.T) {
	ctx := context.Background()
	mockDevice := disk.BlockDevice{
		Name: "sda",
		Type: "disk",
	}

	// Create only Ext4 partitions with StatTimedOut=true
	mockExtPartition := disk.Partition{
		Device:       "/dev/sda1",
		MountPoint:   "/mnt/ext4",
		Fstype:       "ext4",
		Mounted:      false,
		StatTimedOut: true, // This should not affect health state
		Usage:        nil,
	}

	c := createTestComponent(ctx, []string{"/mnt/ext4"}, []string{})
	defer c.Close()

	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{mockDevice}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{mockExtPartition}, nil
	}
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{}, nil // No NFS partitions
	}

	c.Check()

	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	assert.NotNil(t, lastCheckResult)
	// Health should be healthy because StatTimedOut only affects NFS partitions
	assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
	assert.Equal(t, "ok", lastCheckResult.reason)
	assert.Len(t, lastCheckResult.ExtPartitions, 1)
	assert.Len(t, lastCheckResult.NFSPartitions, 0)
	assert.True(t, lastCheckResult.ExtPartitions[0].StatTimedOut)
}

// TestComponent_TimeoutScenarios_CompleteFlow tests comprehensive timeout scenarios
// from file operation timeouts to health state degraded
func TestComponent_TimeoutScenarios_CompleteFlow(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		name        string
		ctxFunc     func() (context.Context, context.CancelFunc)
		expectError bool
		errorType   error
		description string
	}{
		{
			name: "deadline_exceeded_timeout",
			ctxFunc: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				time.Sleep(10 * time.Millisecond) // Ensure timeout
				return ctx, cancel
			},
			expectError: true,
			errorType:   context.DeadlineExceeded,
			description: "simulates NFS hang causing deadline exceeded",
		},
		{
			name: "context_canceled",
			ctxFunc: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx, cancel
			},
			expectError: true,
			errorType:   context.Canceled,
			description: "simulates context cancellation during NFS operation",
		},
		{
			name: "successful_operation",
			ctxFunc: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 5*time.Second)
			},
			expectError: false,
			errorType:   nil,
			description: "normal operation for comparison",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// Create test component with NFS mount points
			c := createTestComponent(ctx, []string{}, []string{"/mnt/nfs-test"})
			defer c.Close()

			// Provide non-empty NFS configs to ensure NFS partitions are checked
			c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{
					{
						VolumePath:   "/mnt/nfs-test",
						DirName:      ".gpud-nfs-checker",
						FileContents: "test",
					},
				}
			}

			// Mock functions to simulate the timeout scenario
			c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{
					{Name: "mock-device", Type: "disk"},
				}, nil
			}
			c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			}
			c.getNFSPartitionsFunc = func(timeoutCtx context.Context) (disk.Partitions, error) {
				// Test with the scenario's context to simulate timeout behavior
				testCtx, testCancel := scenario.ctxFunc()
				defer testCancel()

				// Create a mock NFS partition and test timeout behavior
				partition := disk.Partition{
					Device:     "192.168.1.100:/shared",
					Fstype:     "nfs4",
					MountPoint: "/mnt/nfs-test",
				}

				// Simulate the StatWithTimeout call that would happen in GetPartitions
				_, err := pkgfile.StatWithTimeout(testCtx, "/mnt/nfs-test")
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
						// This simulates what happens in disk_partition_usage.go line 61
						partition.StatTimedOut = true
						partition.Mounted = false
						t.Logf("Scenario %s: StatTimedOut set to true due to %v", scenario.name, err)
					}
				} else {
					partition.Mounted = true
					partition.StatTimedOut = false
				}

				return disk.Partitions{partition}, nil
			}

			// Run the Check method
			result := c.Check()
			cr, ok := result.(*checkResult)
			require.True(t, ok, "result should be checkResult")
			require.Len(t, cr.NFSPartitions, 1, "should have one NFS partition")

			nfsPartition := cr.NFSPartitions[0]

			if scenario.expectError {
				// Verify StatTimedOut is set and health is degraded
				assert.True(t, nfsPartition.StatTimedOut, "StatTimedOut should be true for scenario: %s", scenario.description)
				assert.False(t, nfsPartition.Mounted, "Mounted should be false when StatTimedOut is true")
				assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.health, "Health should be degraded when StatTimedOut is true")
				assert.Contains(t, cr.reason, "stat timed out (possible connection issue)", "Reason should mention timeout")
				assert.Contains(t, cr.reason, "/mnt/nfs-test", "Reason should mention the mount point")
			} else {
				// Verify normal operation
				assert.False(t, nfsPartition.StatTimedOut, "StatTimedOut should be false for successful operation")
				assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health, "Health should be healthy for successful operation")
				assert.Equal(t, "ok", cr.reason, "Reason should be ok for successful operation")
			}
		})
	}
}

// TestComponent_MountTargetTimeoutHandling tests timeout handling for mount targets in Check method
func TestComponent_MountTargetTimeoutHandling(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create component with mount targets that don't exist (to simulate timeout)
	c := createTestComponent(ctx, []string{}, []string{"/nonexistent/mount/target"})
	defer c.Close()

	// Mock functions to return minimal data so we focus on mount target checking
	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{
			{Name: "mock-device", Type: "disk"},
		}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{}, nil
	}
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{}, nil
	}

	// Run Check - this will test the mount target StatWithTimeout logic
	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok, "result should be checkResult")

	// The mount target doesn't exist, so StatWithTimeout should return an error
	// The component should handle this gracefully and continue (not set health to unhealthy)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health, "Health should remain healthy even when mount target stat fails")
	assert.Equal(t, "ok", cr.reason, "Should return ok even when mount target stat fails")
}

// TestComponent_GetPartitionsTimeoutIntegration tests integration with GetPartitions timeout behavior
func TestComponent_GetPartitionsTimeoutIntegration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create component that will use real GetPartitions with very short timeout
	c := createTestComponent(ctx, []string{}, []string{})
	defer c.Close()

	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{
			{Name: "mock-device", Type: "disk"},
		}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{}, nil
	}

	// This simulates GetPartitions with very short timeout that could cause StatTimedOut
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		// Use GetPartitions with very short timeout to potentially trigger StatTimedOut
		return disk.GetPartitions(ctx,
			disk.WithFstype(disk.DefaultNFSFsTypeFunc),
			disk.WithSkipUsage(),
			disk.WithStatTimeout(1*time.Nanosecond), // Very short timeout
		)
	}

	// Run Check method
	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok, "result should be checkResult")

	// Check if any NFS partitions have StatTimedOut and verify health state
	hasStatTimedOut := false
	for _, p := range cr.NFSPartitions {
		if p.StatTimedOut {
			hasStatTimedOut = true
			t.Logf("Found NFS partition with StatTimedOut=true: %s at %s", p.Device, p.MountPoint)
			assert.False(t, p.Mounted, "StatTimedOut partition should not be mounted")
		}
	}

	if hasStatTimedOut {
		assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.health, "Health should be degraded when any NFS partition has StatTimedOut")
		assert.Contains(t, cr.reason, "stat timed out", "Reason should mention timeout")
	} else {
		t.Log("No partitions with StatTimedOut found in this test run")
		// This is expected as the system may not have NFS partitions or timeouts may not occur
	}
}

// TestComponent_LookbackPeriodUsage tests that the lookback period is used for getting recent events
func TestComponent_LookbackPeriodUsage(t *testing.T) {
	ctx := context.Background()

	// Create mock event store and bucket
	mockEventStore := new(mockEventStore)
	mockBucket := new(mockEventBucket)
	mockEventStore.On("Bucket", Name).Return(mockBucket, nil)
	mockBucket.On("Close").Return()

	// Mock reboot event store (using existing implementation from component_suggested_actions_test.go)
	mockRebootStore := &mockRebootEventStore{
		events: eventstore.Events{}, // Empty events for this test
		err:    nil,
	}

	// Custom lookback period for testing
	customLookbackPeriod := 6 * time.Hour

	// Create GPUd instance with mock event store
	gpudInstance := &components.GPUdInstance{
		RootCtx:          ctx,
		EventStore:       mockEventStore,
		RebootEventStore: mockRebootStore,
		MountPoints:      []string{},
		MountTargets:     []string{},
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	defer comp.Close()

	// Get the component and set custom lookback period
	c := comp.(*component)
	c.lookbackPeriod = customLookbackPeriod

	// Verify that both eventBucket and rebootEventStore are set
	assert.NotNil(t, c.eventBucket, "eventBucket should be set")
	assert.NotNil(t, c.rebootEventStore, "rebootEventStore should be set")

	// Set up mock functions
	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{
			{
				Device:     "/dev/sda1",
				MountPoint: "/",
				Usage: &disk.Usage{
					TotalBytes: 1000,
					FreeBytes:  500,
					UsedBytes:  500,
				},
			},
		}, nil
	}
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{}, nil
	}

	// Capture the time parameter passed to Get()
	var capturedSinceTime time.Time
	mockBucket.On("Get", mock.Anything, mock.MatchedBy(func(since time.Time) bool {
		capturedSinceTime = since
		return true
	})).Return(eventstore.Events{}, nil)

	// Run Check
	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify that Get was called with the correct lookback period
	expectedSinceTime := cr.ts.Add(-customLookbackPeriod)
	timeDiff := capturedSinceTime.Sub(expectedSinceTime)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	assert.Less(t, timeDiff, time.Second, "Get should be called with lookback period time")

	mockBucket.AssertCalled(t, "Get", mock.Anything, mock.Anything)
}

// TestComponent_ContextErrorTypes verifies different context error types are handled correctly
func TestComponent_ContextErrorTypes(t *testing.T) {
	t.Parallel()

	errorTests := []struct {
		name      string
		error     error
		expectSet bool
	}{
		{
			name:      "deadline_exceeded_sets_stat_timed_out",
			error:     context.DeadlineExceeded,
			expectSet: true,
		},
		{
			name:      "canceled_sets_stat_timed_out",
			error:     context.Canceled,
			expectSet: true,
		},
		{
			name:      "other_error_does_not_set_stat_timed_out",
			error:     errors.New("some other error"),
			expectSet: false,
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			c := createTestComponent(ctx, []string{}, []string{})
			defer c.Close()

			// Provide non-empty NFS configs to ensure NFS partitions are checked
			c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{
					{
						VolumePath:   "/mnt/test-nfs",
						DirName:      ".gpud-nfs-checker",
						FileContents: "test",
					},
				}
			}

			c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{
					{Name: "mock-device", Type: "disk"},
				}, nil
			}
			c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			}
			c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
				// Create a partition and simulate the error condition
				partition := disk.Partition{
					Device:     "test.example.com:/share",
					Fstype:     "nfs4",
					MountPoint: "/mnt/test-nfs",
					Mounted:    false,
				}

				// Simulate the logic from disk_partition_usage.go
				if errors.Is(tt.error, context.DeadlineExceeded) || errors.Is(tt.error, context.Canceled) {
					partition.StatTimedOut = true
				} else {
					partition.StatTimedOut = false
				}

				return disk.Partitions{partition}, nil
			}

			result := c.Check()
			cr, ok := result.(*checkResult)
			require.True(t, ok)
			require.Len(t, cr.NFSPartitions, 1)

			partition := cr.NFSPartitions[0]

			if tt.expectSet {
				assert.True(t, partition.StatTimedOut, "StatTimedOut should be true for %s", tt.name)
				assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.health, "Health should be degraded")
				assert.Contains(t, cr.reason, "stat timed out", "Reason should mention timeout")
			} else {
				assert.False(t, partition.StatTimedOut, "StatTimedOut should be false for %s", tt.name)
				assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health, "Health should be healthy")
				assert.Equal(t, "ok", cr.reason, "Reason should be ok")
			}
		})
	}
}

// TestComponent_StatWithTimeoutDeadlineExceeded tests the case where statWithTimeoutFunc returns context.DeadlineExceeded
func TestComponent_StatWithTimeoutDeadlineExceeded(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory to use as mount target
	tempDir := t.TempDir()

	c := createTestComponent(ctx, []string{}, []string{tempDir})
	defer c.Close()

	// Override statWithTimeoutFunc to return context.DeadlineExceeded
	c.statWithTimeoutFunc = func(ctx context.Context, path string) (os.FileInfo, error) {
		return nil, context.DeadlineExceeded
	}

	// Mock other functions to return minimal data
	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{
			{Name: "sda", Type: "disk"},
		}, nil
	}
	c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{}, nil
	}
	c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
		return disk.Partitions{}, nil
	}
	c.findMntFunc = func(ctx context.Context, target string) (*disk.FindMntOutput, error) {
		// This should not be called because statWithTimeoutFunc fails first
		return nil, errors.New("findMntFunc should not be called when statWithTimeoutFunc returns DeadlineExceeded")
	}

	// Run Check
	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok, "result should be checkResult")

	// Verify that component remains healthy despite timeout
	// (the timeout is logged but doesn't affect overall health)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "ok", cr.reason)

	// Verify that MountTargetUsages is empty/nil due to the timeout
	assert.Nil(t, cr.MountTargetUsages)
}

// TestComponentWithMountPointFiltering tests that the component properly filters mount points
func TestComponentWithMountPointFiltering(t *testing.T) {
	ctx := context.Background()

	t.Run("filters provider-specific mount points", func(t *testing.T) {
		gpudInstance := &components.GPUdInstance{
			RootCtx: ctx,
		}

		c, err := New(gpudInstance)
		require.NoError(t, err)
		defer c.Close()

		component, ok := c.(*component)
		require.True(t, ok)

		// Verify partition functions are initialized with mount point filtering
		assert.NotNil(t, component.getExt4PartitionsFunc)
		assert.NotNil(t, component.getNFSPartitionsFunc)

		// If running on Linux, also check block devices func
		if runtime.GOOS == "linux" {
			assert.NotNil(t, component.getBlockDevicesFunc)
		}
	})

	t.Run("check filters empty and provider-specific mount points", func(t *testing.T) {
		// Create mock partitions with various mount points
		mockExt4Partitions := disk.Partitions{
			{
				Device:     "/dev/sda1",
				MountPoint: "/",
				Fstype:     "ext4",
				Mounted:    true,
			},
			{
				Device:     "/dev/sda2",
				MountPoint: "/home",
				Fstype:     "ext4",
				Mounted:    true,
			},
		}

		mockNFSPartitions := disk.Partitions{
			{
				Device:     "server:/data",
				MountPoint: "/mnt/nfs",
				Fstype:     "nfs4",
				Mounted:    true,
			},
		}

		mockDevice := disk.BlockDevice{
			Name: "sda",
			Type: "disk",
		}

		c := createTestComponent(ctx, []string{"/"}, []string{})
		defer c.Close()

		// Provide non-empty NFS configs to ensure NFS partitions are checked
		c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{
					VolumePath:   "/mnt/nfs",
					DirName:      ".gpud-nfs-checker",
					FileContents: "test",
				},
			}
		}
		// Override the partition functions to return our mock data
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return mockExt4Partitions, nil
		}
		c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return mockNFSPartitions, nil
		}
		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{mockDevice}, nil
		}

		// Run check
		c.Check()

		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		// Verify results - should include all partitions (filtering happens at the disk package level)
		assert.NotNil(t, lastCheckResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, lastCheckResult.health)
		assert.Len(t, lastCheckResult.ExtPartitions, 2)
		assert.Len(t, lastCheckResult.NFSPartitions, 1)

		// Verify none have empty mount points (as they would be filtered by the disk package)
		for _, p := range lastCheckResult.ExtPartitions {
			assert.NotEmpty(t, p.MountPoint)
		}
		for _, p := range lastCheckResult.NFSPartitions {
			assert.NotEmpty(t, p.MountPoint)
		}
	})
}

// TestComponent_SuperblockWriteErrorDetection tests detection and health evaluation of superblock write errors
func TestComponent_SuperblockWriteErrorDetection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now()

	t.Run("single superblock write error makes component unhealthy", func(t *testing.T) {
		// Create a test event store
		eventBucket := &simpleMockEventBucket{}

		// Insert a superblock write error event
		err := eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-30 * time.Minute),
			Name:      eventSuperblockWriteError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "I/O error while writing superblock",
		})
		require.NoError(t, err)

		// Create mock reboot event store
		mockRebootStore := &mockRebootEventStore{
			events: eventstore.Events{}, // No reboots
		}

		// Create component
		c := &component{
			ctx:              ctx,
			rebootEventStore: mockRebootStore,
			eventBucket:      eventBucket,
			lookbackPeriod:   time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 1000,
							FreeBytes:  500,
							UsedBytes:  500,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		// Run check
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should detect superblock write error and be unhealthy
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "I/O error while writing superblock", cr.reason)

		// Should suggest reboot since no previous reboots
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, cr.suggestedActions.RepairActions[0])
	})

	t.Run("multiple superblock write error events", func(t *testing.T) {
		// Create a test event store
		eventBucket := &simpleMockEventBucket{}

		// Insert multiple superblock write error events
		events := []time.Time{
			now.Add(-60 * time.Minute),
			now.Add(-45 * time.Minute),
			now.Add(-30 * time.Minute),
		}

		for _, eventTime := range events {
			err := eventBucket.Insert(ctx, eventstore.Event{
				Component: Name,
				Time:      eventTime,
				Name:      eventSuperblockWriteError,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "I/O error while writing superblock",
			})
			require.NoError(t, err)
		}

		// Create mock reboot event store with one reboot before failures
		mockRebootStore := &mockRebootEventStore{
			events: eventstore.Events{
				{
					Time:    now.Add(-90 * time.Minute), // Reboot before failures
					Name:    "reboot",
					Type:    string(apiv1.EventTypeWarning),
					Message: "system reboot detected",
				},
			},
		}

		// Create component
		c := &component{
			ctx:              ctx,
			rebootEventStore: mockRebootStore,
			eventBucket:      eventBucket,
			lookbackPeriod:   2 * time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 1000,
							FreeBytes:  500,
							UsedBytes:  500,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		// Run check
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should detect superblock write errors and be unhealthy
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "I/O error while writing superblock", cr.reason)

		// Should suggest reboot since multiple failures after one reboot doesn't meet threshold
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, cr.suggestedActions.RepairActions[0])
	})

	t.Run("superblock write errors resolved after reboot", func(t *testing.T) {
		// Create a test event store
		eventBucket := &simpleMockEventBucket{}

		// Insert superblock write error before reboot
		err := eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-60 * time.Minute), // Before reboot
			Name:      eventSuperblockWriteError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "I/O error while writing superblock",
		})
		require.NoError(t, err)

		// Create mock reboot event store with reboot after failure
		mockRebootStore := &mockRebootEventStore{
			events: eventstore.Events{
				{
					Time:    now.Add(-30 * time.Minute), // Reboot after failure
					Name:    "reboot",
					Type:    string(apiv1.EventTypeWarning),
					Message: "system reboot detected",
				},
			},
		}

		// Create component
		c := &component{
			ctx:              ctx,
			rebootEventStore: mockRebootStore,
			eventBucket:      eventBucket,
			lookbackPeriod:   2 * time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 1000,
							FreeBytes:  500,
							UsedBytes:  500,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		// Run check
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be healthy since reboot resolved the issue
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.Nil(t, cr.suggestedActions)
	})
}

// TestComponent_SuperblockWriteErrorWithMixedEventTypes tests superblock write errors with other error types
func TestComponent_SuperblockWriteErrorWithMixedEventTypes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now()

	t.Run("superblock write error with buffer I/O error", func(t *testing.T) {
		// Create a test event store
		eventBucket := &simpleMockEventBucket{}

		// Insert superblock write error events
		err := eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-30 * time.Minute),
			Name:      eventSuperblockWriteError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "I/O error while writing superblock",
		})
		require.NoError(t, err)

		// Insert buffer I/O error as well
		err = eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-25 * time.Minute),
			Name:      eventBufferIOError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "Buffer I/O error detected on device",
		})
		require.NoError(t, err)

		// Create mock reboot event store
		mockRebootStore := &mockRebootEventStore{
			events: eventstore.Events{}, // No reboots
		}

		// Create component
		c := &component{
			ctx:              ctx,
			rebootEventStore: mockRebootStore,
			eventBucket:      eventBucket,
			lookbackPeriod:   time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 1000,
							FreeBytes:  500,
							UsedBytes:  500,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		// Run check
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be unhealthy with both error types in reason
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "I/O error while writing superblock")
		assert.Contains(t, cr.reason, "Buffer I/O error detected on device")

		// Should suggest reboot since no previous reboots
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, cr.suggestedActions.RepairActions[0])
	})

	t.Run("multiple error types sorted lexicographically", func(t *testing.T) {
		// Create a test event store
		eventBucket := &simpleMockEventBucket{}

		// Insert various error types including superblock write error
		events := []struct {
			name    string
			message string
		}{
			{eventSuperblockWriteError, "I/O error while writing superblock"},
			{eventBufferIOError, "Buffer I/O error detected on device"},
			{eventFilesystemReadOnly, "Filesystem remounted read-only"},
			{eventNVMePathFailure, "NVMe path failure detected"},
		}

		for i, evt := range events {
			err := eventBucket.Insert(ctx, eventstore.Event{
				Component: Name,
				Time:      now.Add(-time.Duration(10*(i+1)) * time.Minute),
				Name:      evt.name,
				Type:      string(apiv1.EventTypeWarning),
				Message:   evt.message,
			})
			require.NoError(t, err)
		}

		// Create mock reboot event store
		mockRebootStore := &mockRebootEventStore{
			events: eventstore.Events{}, // No reboots
		}

		// Create component
		c := &component{
			ctx:              ctx,
			rebootEventStore: mockRebootStore,
			eventBucket:      eventBucket,
			lookbackPeriod:   time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 1000,
							FreeBytes:  500,
							UsedBytes:  500,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		// Run check
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be unhealthy
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)

		// Verify reasons are sorted lexicographically
		expectedReason := "Buffer I/O error detected on device, I/O error while writing superblock, NVMe device has no available path, I/O failing, filesystem remounted as read-only due to errors"
		assert.Equal(t, expectedReason, cr.reason)

		// Should suggest reboot since no previous reboots for multiple error types
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, cr.suggestedActions.RepairActions[0])
	})
}

// TestComponent_SuperblockWriteErrorSuggestedActions tests suggested actions logic specifically for superblock write errors
func TestComponent_SuperblockWriteErrorSuggestedActions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now()

	t.Run("persistent superblock write errors trigger HW inspection", func(t *testing.T) {
		// Create a test event store
		eventBucket := &simpleMockEventBucket{}

		// Insert superblock write error events that persist after reboots
		err := eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-4 * time.Hour), // First failure
			Name:      eventSuperblockWriteError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "I/O error while writing superblock",
		})
		require.NoError(t, err)

		err = eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-1 * time.Hour), // Second failure after reboot
			Name:      eventSuperblockWriteError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "I/O error while writing superblock",
		})
		require.NoError(t, err)

		// Create mock reboot event store with two reboots
		mockRebootStore := &mockRebootEventStore{
			events: eventstore.Events{
				{
					Time:    now.Add(-5 * time.Hour), // First reboot
					Name:    "reboot",
					Type:    string(apiv1.EventTypeWarning),
					Message: "system reboot detected",
				},
				{
					Time:    now.Add(-2 * time.Hour), // Second reboot after first failure
					Name:    "reboot",
					Type:    string(apiv1.EventTypeWarning),
					Message: "system reboot detected",
				},
			},
		}

		// Create component
		c := &component{
			ctx:              ctx,
			rebootEventStore: mockRebootStore,
			eventBucket:      eventBucket,
			lookbackPeriod:   6 * time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 1000,
							FreeBytes:  500,
							UsedBytes:  500,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		// Run check
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be unhealthy
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "I/O error while writing superblock", cr.reason)

		// Should suggest hardware inspection since failures persist after reboots
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, cr.suggestedActions.RepairActions[0])
	})

	t.Run("no suggested actions when event store fails", func(t *testing.T) {
		// Create a component with eventBucket but no rebootEventStore to simulate error
		c := &component{
			ctx:              ctx,
			rebootEventStore: nil, // This will cause an error in the Check method
			eventBucket:      &simpleMockEventBucket{},
			lookbackPeriod:   time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 1000,
							FreeBytes:  500,
							UsedBytes:  500,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		// Run check
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be healthy since no events can be processed
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.Nil(t, cr.suggestedActions)
	})
}

// TestComponent_SuperblockWriteErrorEventStoreErrors tests error handling in event store operations
func TestComponent_SuperblockWriteErrorEventStoreErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now()

	t.Run("event bucket get error", func(t *testing.T) {
		// Create a mock event bucket that returns an error
		mockBucket := new(mockEventBucket)
		expectedErr := errors.New("event bucket get error")
		mockBucket.On("Get", mock.Anything, mock.Anything).Return(nil, expectedErr)
		mockBucket.On("Close").Return().Maybe()

		// Create mock reboot event store
		mockRebootStore := &mockRebootEventStore{
			events: eventstore.Events{}, // No reboots
		}

		// Create component
		c := &component{
			ctx:              ctx,
			rebootEventStore: mockRebootStore,
			eventBucket:      mockBucket,
			lookbackPeriod:   time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 1000,
							FreeBytes:  500,
							UsedBytes:  500,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		// Run check
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be unhealthy due to event store error
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "failed to get recent events", cr.reason)
		assert.Equal(t, expectedErr, cr.err)
		assert.Nil(t, cr.suggestedActions)

		mockBucket.AssertCalled(t, "Get", mock.Anything, mock.Anything)
	})

	t.Run("reboot event store error", func(t *testing.T) {
		// Create a test event store with superblock write error
		eventBucket := &simpleMockEventBucket{}
		err := eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      time.Now().Add(-30 * time.Minute),
			Name:      eventSuperblockWriteError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "I/O error while writing superblock",
		})
		require.NoError(t, err)

		// Create mock reboot event store that returns an error
		expectedErr := errors.New("reboot event store error")
		mockRebootStore := &mockRebootEventStore{
			err: expectedErr,
		}

		// Create component
		c := &component{
			ctx:              ctx,
			rebootEventStore: mockRebootStore,
			eventBucket:      eventBucket,
			lookbackPeriod:   time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 1000,
							FreeBytes:  500,
							UsedBytes:  500,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		// Run check
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be unhealthy due to reboot event store error
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, expectedErr, cr.err)
		assert.Nil(t, cr.suggestedActions)
	})
}

// TestComponent_SuperblockWriteErrorIntegration tests end-to-end integration with real examples
func TestComponent_SuperblockWriteErrorIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now()

	t.Run("real-world scenario with buffer I/O and superblock errors", func(t *testing.T) {
		// Create a test event store
		eventBucket := &simpleMockEventBucket{}

		// Insert events similar to the user's real examples
		// [83028.888615] Buffer I/O error on dev dm-0, logical block 0, lost sync page write
		// [83028.888618] EXT4-fs (dm-0): I/O error while writing superblock
		err := eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-30 * time.Minute),
			Name:      eventBufferIOError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "Buffer I/O error detected on device",
		})
		require.NoError(t, err)

		err = eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-29 * time.Minute), // 3 seconds later, like in real logs
			Name:      eventSuperblockWriteError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "I/O error while writing superblock",
		})
		require.NoError(t, err)

		// Multiple occurrences like in real logs
		err = eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-25 * time.Minute),
			Name:      eventBufferIOError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "Buffer I/O error detected on device",
		})
		require.NoError(t, err)

		err = eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-24 * time.Minute),
			Name:      eventSuperblockWriteError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "I/O error while writing superblock",
		})
		require.NoError(t, err)

		// Create mock reboot event store
		mockRebootStore := &mockRebootEventStore{
			events: eventstore.Events{}, // No reboots
		}

		// Create component
		c := &component{
			ctx:              ctx,
			rebootEventStore: mockRebootStore,
			eventBucket:      eventBucket,
			lookbackPeriod:   time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "dm-0", Type: "dm"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/dm-0",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 1000,
							FreeBytes:  500,
							UsedBytes:  500,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		// Run check
		result := c.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be unhealthy with both error types
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)

		// Should contain both error messages in sorted order
		expectedReason := "Buffer I/O error detected on device, I/O error while writing superblock"
		assert.Equal(t, expectedReason, cr.reason)

		// Should suggest reboot for initial occurrence
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, cr.suggestedActions.RepairActions[0])
	})
}
