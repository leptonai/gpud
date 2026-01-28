package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
)

func TestComponentCheck_VirtualMemoryErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("Check returns error when VirtualMemory fails", t, func() {
		mockey.Mock(mem.VirtualMemoryWithContext).To(func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
			return nil, errors.New("vmem failed")
		}).Build()

		comp, err := New(&components.GPUdInstance{RootCtx: context.Background()})
		require.NoError(t, err)

		result := comp.Check()
		assert.Equal(t, "error getting virtual memory", result.Summary())
	})
}

func TestComponentCheck_BPFJITErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("Check returns error when BPF JIT buffer lookup fails", t, func() {
		mockey.Mock(mem.VirtualMemoryWithContext).To(func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{
				Total:        1024,
				Available:    512,
				Used:         512,
				Free:         128,
				UsedPercent:  50.0,
				VmallocTotal: 0,
				VmallocUsed:  0,
			}, nil
		}).Build()
		mockey.Mock(getCurrentBPFJITBufferBytes).To(func() (uint64, error) {
			return 0, errors.New("bpf failed")
		}).Build()

		comp, err := New(&components.GPUdInstance{RootCtx: context.Background()})
		require.NoError(t, err)

		result := comp.Check()
		assert.Equal(t, "error getting bpf jit buffer bytes", result.Summary())
	})
}

func TestComponentCheck_LowAvailableThresholdWithMockey(t *testing.T) {
	mockey.PatchConvey("Check completes when available memory is low", t, func() {
		mockey.Mock(mem.VirtualMemoryWithContext).To(func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{
				Total:       1024,
				Available:   1,
				Used:        1023,
				Free:        1,
				UsedPercent: 99.9,
			}, nil
		}).Build()

		comp, err := New(&components.GPUdInstance{RootCtx: context.Background()})
		require.NoError(t, err)

		// Force a very high threshold to ensure the low-available branch is hit.
		if c, ok := comp.(*component); ok {
			c.availableThresholdBytes = 2
			c.getTimeNowFunc = func() time.Time { return time.Unix(0, 0).UTC() }
		}

		result := comp.Check()
		assert.Equal(t, "ok", result.Summary())
	})
}

type stubEventBucket struct {
	lastPurgeBefore int64
	purgeCount      int
}

func (s *stubEventBucket) Name() string { return "memory" }
func (s *stubEventBucket) Insert(ctx context.Context, ev eventstore.Event) error {
	return nil
}
func (s *stubEventBucket) Find(ctx context.Context, ev eventstore.Event) (*eventstore.Event, error) {
	return nil, nil
}
func (s *stubEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	return nil, nil
}
func (s *stubEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) { return nil, nil }
func (s *stubEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	s.lastPurgeBefore = beforeTimestamp
	s.purgeCount = 1
	return 1, nil
}
func (s *stubEventBucket) Close() {}

func TestComponentMethodsAndSetHealthy(t *testing.T) {
	now := time.Unix(123, 0).UTC()
	bucket := &stubEventBucket{}

	comp := &component{
		ctx:            context.Background(),
		cancel:         func() {},
		eventBucket:    bucket,
		getTimeNowFunc: func() time.Time { return now },
	}

	assert.True(t, comp.IsSupported())
	assert.NoError(t, comp.SetHealthy())
	assert.Equal(t, now.Unix(), bucket.lastPurgeBefore)
	assert.Equal(t, 1, bucket.purgeCount)

	cr := &checkResult{reason: "ok", health: apiv1.HealthStateTypeHealthy}
	assert.Equal(t, Name, cr.ComponentName())
	assert.Equal(t, "ok", cr.Summary())
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
}
