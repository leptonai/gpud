package disk

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

type fakeKmsgSyncer struct {
	closed bool
}

func (f *fakeKmsgSyncer) Close() {
	f.closed = true
}

func TestNew_DefaultNFSPartitionsFunc_Executes(t *testing.T) {
	comp, err := New(&components.GPUdInstance{
		RootCtx: context.Background(),
	})
	require.NoError(t, err)
	defer func() {
		_ = comp.Close()
	}()

	diskComp := comp.(*component)
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = diskComp.getNFSPartitionsFunc(cctx)
}

func TestNewComponent_InjectableBranches(t *testing.T) {
	t.Run("linux sets getBlockDevicesFunc closure", func(t *testing.T) {
		comp, err := newComponent(&components.GPUdInstance{
			RootCtx: context.Background(),
		}, "linux", 1000, nil)
		require.NoError(t, err)
		defer func() {
			_ = comp.Close()
		}()

		diskComp := comp.(*component)
		require.NotNil(t, diskComp.getBlockDevicesFunc)

		cctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _ = diskComp.getBlockDevicesFunc(cctx)
	})

	t.Run("root kmsg syncer error propagates", func(t *testing.T) {
		mockStore := new(mockEventStore)
		mockBucket := new(mockEventBucket)
		mockStore.On("Bucket", Name).Return(mockBucket, nil)
		mockBucket.On("Close").Return().Maybe()

		expectedErr := errors.New("kmsg syncer failed")
		comp, err := newComponent(&components.GPUdInstance{
			RootCtx:    context.Background(),
			EventStore: mockStore,
		}, "linux", 0, func(ctx context.Context, matchFunc kmsg.MatchFunc, eventBucket eventstore.Bucket, opts ...kmsg.OpOption) (*kmsg.Syncer, error) {
			return nil, expectedErr
		})
		require.ErrorIs(t, err, expectedErr)
		assert.Nil(t, comp)
	})

	t.Run("root kmsg syncer success is stored", func(t *testing.T) {
		mockStore := new(mockEventStore)
		mockBucket := new(mockEventBucket)
		mockStore.On("Bucket", Name).Return(mockBucket, nil)
		mockBucket.On("Close").Return()

		comp, err := newComponent(&components.GPUdInstance{
			RootCtx:    context.Background(),
			EventStore: mockStore,
		}, "linux", 0, func(ctx context.Context, matchFunc kmsg.MatchFunc, eventBucket eventstore.Bucket, opts ...kmsg.OpOption) (*kmsg.Syncer, error) {
			return &kmsg.Syncer{}, nil
		})
		require.NoError(t, err)

		diskComp := comp.(*component)
		assert.NotNil(t, diskComp.kmsgSyncer)

		// Avoid calling Close on the zero-value kmsg.Syncer from this test.
		diskComp.kmsgSyncer = nil
		require.NoError(t, comp.Close())
	})
}

func TestComponent_Close_ClosesKmsgSyncer(t *testing.T) {
	syncer := &fakeKmsgSyncer{}
	mockBucket := new(mockEventBucket)
	mockBucket.On("Close").Return()

	cctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:         cctx,
		cancel:      cancel,
		kmsgSyncer:  syncer,
		eventBucket: mockBucket,
	}

	require.NoError(t, c.Close())
	assert.True(t, syncer.closed)
	mockBucket.AssertCalled(t, "Close")
}

func TestComponent_fetchBlockDevices_ContextCanceled_AppendsReason(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := createTestComponent(ctx, nil, nil)
	defer func() {
		_ = c.Close()
	}()
	c.retryInterval = 10 * time.Millisecond
	cancel()

	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return nil, errors.New("lsblk failed")
	}

	cr := &checkResult{
		health: apiv1.HealthStateTypeHealthy,
		reason: "existing reason",
	}
	ok := c.fetchBlockDevices(cr)
	assert.False(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.ErrorIs(t, cr.err, context.Canceled)
	assert.Equal(t, "existing reason; failed to get block devices -- took too long", cr.reason)
}

func TestComponent_fetchBlockDevices_ContextCanceled_FromOKReason(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := createTestComponent(ctx, nil, nil)
	defer func() {
		_ = c.Close()
	}()
	c.retryInterval = 10 * time.Millisecond
	cancel()

	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return nil, errors.New("lsblk failed")
	}

	cr := &checkResult{
		health: apiv1.HealthStateTypeHealthy,
		reason: "ok",
	}
	ok := c.fetchBlockDevices(cr)
	assert.False(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.ErrorIs(t, cr.err, context.Canceled)
	assert.Equal(t, "failed to get block devices -- took too long", cr.reason)
}

func TestComponent_fetchBlockDevices_NoDevices_AppendsReasonAndSetsDefaultHealth(t *testing.T) {
	c := createTestComponent(context.Background(), nil, nil)
	defer func() {
		_ = c.Close()
	}()

	c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
		return disk.BlockDevices{}, nil
	}

	cr := &checkResult{
		reason: "existing reason",
	}
	ok := c.fetchBlockDevices(cr)
	assert.False(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "existing reason; no block device found", cr.reason)
}

func TestComponent_fetchPartitions_ContextCanceled_AppendsReason(t *testing.T) {
	tests := []struct {
		name   string
		call   func(c *component, cr *checkResult) bool
		suffix string
	}{
		{
			name: "ext4",
			call: func(c *component, cr *checkResult) bool {
				c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
					return nil, errors.New("ext4 failed")
				}
				return c.fetchExt4Partitions(cr)
			},
			suffix: "failed to get ext4 partitions -- took too long",
		},
		{
			name: "nfs",
			call: func(c *component, cr *checkResult) bool {
				c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
					return nil, errors.New("nfs failed")
				}
				return c.fetchNFSPartitions(cr)
			},
			suffix: "failed to get nfs partitions -- took too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			c := createTestComponent(ctx, nil, nil)
			defer func() {
				_ = c.Close()
			}()
			c.retryInterval = 10 * time.Millisecond
			cancel()

			cr := &checkResult{
				health: apiv1.HealthStateTypeHealthy,
				reason: "existing reason",
			}

			ok := tt.call(c, cr)
			assert.False(t, ok)
			assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
			assert.ErrorIs(t, cr.err, context.Canceled)
			assert.Equal(t, "existing reason; "+tt.suffix, cr.reason)
		})
	}
}

func TestComponent_recordNFSStatTimeout_EdgeBranches(t *testing.T) {
	c := &component{}

	assert.Equal(t, 0, c.recordNFSStatTimeout("", true))

	// Covers nil-map initialization path.
	assert.Equal(t, 1, c.recordNFSStatTimeout("/mnt/nfs", true))
	assert.Equal(t, 2, c.recordNFSStatTimeout("/mnt/nfs", true))
	assert.Equal(t, 0, c.recordNFSStatTimeout("/mnt/nfs", false))
}

func TestComponent_Check_MountTargetStatAndFindMntBranches(t *testing.T) {
	t.Run("stat generic error skips findmnt and stays healthy", func(t *testing.T) {
		targetDir := t.TempDir()
		c := createTestComponent(context.Background(), nil, []string{targetDir})
		defer func() {
			_ = c.Close()
		}()

		findMntCalled := false
		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		}
		c.statWithTimeoutFunc = func(ctx context.Context, path string) (os.FileInfo, error) {
			return nil, errors.New("permission denied")
		}
		c.findMntFunc = func(ctx context.Context, target string) (*disk.FindMntOutput, error) {
			findMntCalled = true
			return nil, errors.New("should not be called")
		}

		result := c.Check()
		cr := result.(*checkResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.False(t, findMntCalled)
	})

	t.Run("findmnt error returns unhealthy when component context is canceled", func(t *testing.T) {
		targetDir := t.TempDir()

		ctx, cancel := context.WithCancel(context.Background())
		c := createTestComponent(ctx, nil, []string{targetDir})
		defer func() {
			_ = c.Close()
		}()
		c.retryInterval = 10 * time.Millisecond

		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		}
		c.statWithTimeoutFunc = func(ctx context.Context, path string) (os.FileInfo, error) {
			return os.Stat(targetDir)
		}
		c.findMntFunc = func(ctx context.Context, target string) (*disk.FindMntOutput, error) {
			return nil, errors.New("findmnt failed")
		}

		cancel()

		result := c.Check()
		cr := result.(*checkResult)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.ErrorIs(t, cr.err, context.Canceled)
	})
}

func TestComponent_Check_AdditionalEventBranchCoverage(t *testing.T) {
	t.Run("NVMe timeout/device-disabled/beyond-end events aggregate suggested actions", func(t *testing.T) {
		ctx := context.Background()
		now := time.Now()

		eventBucket := &simpleMockEventBucket{}
		eventNames := []string{
			eventNVMeTimeout,
			eventNVMeDeviceDisabled,
			eventBeyondEndOfDevice,
		}
		for i, eventName := range eventNames {
			err := eventBucket.Insert(ctx, eventstore.Event{
				Component: Name,
				Time:      now.Add(time.Duration(-10+i) * time.Minute),
				Name:      eventName,
				Type:      string(apiv1.EventTypeWarning),
				Message:   eventName,
			})
			require.NoError(t, err)
		}

		c := &component{
			ctx:              ctx,
			rebootEventStore: &mockRebootEventStore{events: eventstore.Events{}},
			eventBucket:      eventBucket,
			lookbackPeriod:   time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			freeSpaceThresholdBytesDegraded: defaultFreeSpaceThresholdBytesDegraded,
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 100 * 1024 * 1024 * 1024,
							FreeBytes:  90 * 1024 * 1024 * 1024,
							UsedBytes:  10 * 1024 * 1024 * 1024,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		cr := c.Check().(*checkResult)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.reason, messageNVMeTimeout)
		assert.Contains(t, cr.reason, messageNVMeDeviceDisabled)
		assert.Contains(t, cr.reason, messageBeyondEndOfDevice)
	})

	t.Run("degraded reason appends to existing disk-failure reason", func(t *testing.T) {
		ctx := context.Background()
		now := time.Now()

		eventBucket := &simpleMockEventBucket{}
		err := eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      now.Add(-10 * time.Minute),
			Name:      eventRAIDArrayFailure,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "raid failure",
		})
		require.NoError(t, err)

		c := &component{
			ctx:              ctx,
			rebootEventStore: &mockRebootEventStore{events: eventstore.Events{}},
			eventBucket:      eventBucket,
			lookbackPeriod:   time.Hour,
			getTimeNowFunc: func() time.Time {
				return now
			},
			getGroupConfigsFunc: func() pkgnfschecker.Configs {
				return pkgnfschecker.Configs{}
			},
			freeSpaceThresholdBytesDegraded: 20 * 1024 * 1024 * 1024, // 20 GiB
			getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
				return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{
					{
						Device:     "/dev/sda1",
						MountPoint: "/",
						Usage: &disk.Usage{
							TotalBytes: 100 * 1024 * 1024 * 1024,
							FreeBytes:  10 * 1024 * 1024 * 1024,
							UsedBytes:  90 * 1024 * 1024 * 1024,
						},
					},
				}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		cr := c.Check().(*checkResult)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, messageRAIDArrayFailure)
		assert.Contains(t, cr.reason, "ext4 partition /: free space 10 GiB is below 20 GiB threshold")
		assert.Contains(t, cr.reason, "; ")
	})

	t.Run("NFS partition usage on untracked mount point is skipped for metrics", func(t *testing.T) {
		ctx := context.Background()
		c := createTestComponent(ctx, nil, nil)
		defer func() {
			_ = c.Close()
		}()

		c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{
				{VolumePath: "/mnt/untracked"},
			}
		}
		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.BlockDevices{{Name: "sda", Type: "disk"}}, nil
		}
		c.getExt4PartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		}
		c.getNFSPartitionsFunc = func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{
				{
					Device:     "10.0.0.1:/share",
					MountPoint: "/mnt/untracked",
					Fstype:     "nfs4",
					Usage: &disk.Usage{
						TotalBytes: 100,
						FreeBytes:  90,
						UsedBytes:  10,
					},
				},
			}, nil
		}

		cr := c.Check().(*checkResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
	})
}
