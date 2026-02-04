package cpu

import (
	"context"
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
)

func TestComponentCheck_TimeStatErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("Check returns error when time stats fail", t, func() {
		mockey.Mock(getTimeStatForAllCPUs).To(func(ctx context.Context) (cpu.TimesStat, error) {
			return cpu.TimesStat{}, errors.New("times failed")
		}).Build()

		comp, err := New(&components.GPUdInstance{RootCtx: context.Background()})
		require.NoError(t, err)

		result := comp.Check()
		assert.Equal(t, "error calculating CPU usage", result.Summary())
	})
}

func TestComponentCheck_LoadAverageErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("Check returns error when load average fails", t, func() {
		mockey.Mock(getTimeStatForAllCPUs).To(func(ctx context.Context) (cpu.TimesStat, error) {
			return cpu.TimesStat{User: 1, System: 1, Idle: 1}, nil
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
