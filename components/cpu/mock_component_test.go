package cpu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	gopscpu "github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
)

func TestComponentCheck_TimeStatErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("Check returns error when time stats fail", t, func() {
		mockey.Mock(getTimeStatForAllCPUs).To(func(ctx context.Context) (gopscpu.TimesStat, error) {
			return gopscpu.TimesStat{}, errors.New("times failed")
		}).Build()

		comp, err := New(&components.GPUdInstance{RootCtx: context.Background()})
		require.NoError(t, err)

		result := comp.Check()
		assert.Equal(t, "error calculating CPU usage", result.Summary())
	})
}

func TestComponentCheck_LoadAverageErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("Check returns error when load average fails", t, func() {
		mockey.Mock(getTimeStatForAllCPUs).To(func(ctx context.Context) (gopscpu.TimesStat, error) {
			return gopscpu.TimesStat{User: 1, System: 1, Idle: 1}, nil
		}).Build()
		mockey.Mock(getUsedPercentForAllCPUs).To(func(ctx context.Context) (float64, error) {
			return 10.0, nil
		}).Build()
		mockey.Mock(load.AvgWithContext).To(func(ctx context.Context) (*load.AvgStat, error) {
			return nil, errors.New("load failed")
		}).Build()

		comp, err := New(&components.GPUdInstance{RootCtx: context.Background()})
		require.NoError(t, err)

		result := comp.Check()
		assert.Equal(t, "error calculating load average", result.Summary())
	})
}

type staticBucketStore struct {
	bucket eventstore.Bucket
	err    error
}

func (s *staticBucketStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.bucket, nil
}

func TestNew_EventBucketErrorWithMockey(t *testing.T) {
	comp, err := New(&components.GPUdInstance{
		RootCtx: context.Background(),
		EventStore: &staticBucketStore{
			err: errors.New("bucket failed"),
		},
	})
	require.Error(t, err)
	assert.Nil(t, comp)
}

func TestGetTimeStatForAllCPUs_WithMockey(t *testing.T) {
	t.Run("times error", func(t *testing.T) {
		mockey.PatchConvey("getTimeStatForAllCPUs returns error from gopsutil", t, func() {
			mockey.Mock(gopscpu.TimesWithContext).To(func(ctx context.Context, percpu bool) ([]gopscpu.TimesStat, error) {
				return nil, errors.New("times error")
			}).Build()
			_, err := getTimeStatForAllCPUs(context.Background())
			require.Error(t, err)
		})
	})

	t.Run("invalid result size", func(t *testing.T) {
		mockey.PatchConvey("getTimeStatForAllCPUs validates result count", t, func() {
			mockey.Mock(gopscpu.TimesWithContext).To(func(ctx context.Context, percpu bool) ([]gopscpu.TimesStat, error) {
				return []gopscpu.TimesStat{
					{CPU: "cpu0"},
					{CPU: "cpu1"},
				}, nil
			}).Build()
			_, err := getTimeStatForAllCPUs(context.Background())
			require.Error(t, err)
		})
	})

	t.Run("success", func(t *testing.T) {
		mockey.PatchConvey("getTimeStatForAllCPUs returns single stat", t, func() {
			mockey.Mock(gopscpu.TimesWithContext).To(func(ctx context.Context, percpu bool) ([]gopscpu.TimesStat, error) {
				return []gopscpu.TimesStat{
					{CPU: "cpu-total", User: 1},
				}, nil
			}).Build()
			st, err := getTimeStatForAllCPUs(context.Background())
			require.NoError(t, err)
			assert.Equal(t, "cpu-total", st.CPU)
		})
	})
}

func TestGetUsedPercentForAllCPUs_WithMockey(t *testing.T) {
	t.Run("percent error", func(t *testing.T) {
		mockey.PatchConvey("getUsedPercentForAllCPUs returns error from gopsutil", t, func() {
			mockey.Mock(gopscpu.PercentWithContext).To(func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error) {
				return nil, errors.New("percent error")
			}).Build()
			_, err := getUsedPercentForAllCPUs(context.Background())
			require.Error(t, err)
		})
	})

	t.Run("invalid result size", func(t *testing.T) {
		mockey.PatchConvey("getUsedPercentForAllCPUs validates result count", t, func() {
			mockey.Mock(gopscpu.PercentWithContext).To(func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error) {
				return []float64{1.0, 2.0}, nil
			}).Build()
			_, err := getUsedPercentForAllCPUs(context.Background())
			require.Error(t, err)
		})
	})

	t.Run("success", func(t *testing.T) {
		mockey.PatchConvey("getUsedPercentForAllCPUs returns one value", t, func() {
			mockey.Mock(gopscpu.PercentWithContext).To(func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error) {
				return []float64{42.5}, nil
			}).Build()
			v, err := getUsedPercentForAllCPUs(context.Background())
			require.NoError(t, err)
			assert.Equal(t, 42.5, v)
		})
	})
}

func TestComponentMethods_IsSupportedAndComponentName(t *testing.T) {
	c := &component{}
	assert.True(t, c.IsSupported())

	cr := &checkResult{}
	assert.Equal(t, Name, cr.ComponentName())
}
