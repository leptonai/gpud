package infiniband

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

type nativeGB300Instance struct{ mockNVMLInstance }

func (*nativeGB300Instance) Devices() map[string]device.Device {
	return map[string]device.Device{"GPU-a": nil}
}

func TestGB300DefaultThroughNativeSysfsCollector(t *testing.T) {
	root := copyTestClassDir(t, testClassDir)
	keep := map[string]bool{"mlx5_0": true, "mlx5_1": true, "mlx5_8": true, "mlx5_9": true}
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	for _, entry := range entries {
		if !keep[entry.Name()] {
			require.NoError(t, os.RemoveAll(filepath.Join(root, entry.Name())))
		}
	}
	for name := range keep {
		updatePortState(t, root, name, 1, "Active")
		updatePortPhysState(t, root, name, 1, "LinkUp")
		updatePortRate(t, root, name, 1, "800 Gb/sec (4X XDR)")
	}
	enabled := true
	profile := types.ExpectedPortStates{Enabled: &enabled}
	c := &component{
		ctx:               context.Background(),
		nvmlInstance:      &nativeGB300Instance{mockNVMLInstance{exists: true, productName: "NVIDIA GB300"}},
		getTimeNowFunc:    time.Now,
		getThresholdsFunc: func() types.ExpectedPortStates { return profile },
		ignoreFiles:       make(map[string]struct{}),
		getClassDevicesFunc: func(ignore map[string]struct{}) (infinibandclass.Devices, error) {
			return infinibandclass.LoadDevices(root, infinibandclass.WithIgnoreFiles(ignore))
		},
	}
	state := c.Check().HealthStates()[0]
	require.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	require.True(t, *state.Evaluated)
	updatePortRate(t, root, "mlx5_0", 1, "400 Gb/sec (4X NDR)")
	state = c.Check().HealthStates()[0]
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	require.Contains(t, state.Reason, "expect >=4 port(s)")
	require.True(t, *state.Evaluated)
	profile.AtLeastPorts, profile.AtLeastRate = 4, 400
	state = c.Check().HealthStates()[0]
	require.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	enabled = false
	state = c.Check().HealthStates()[0]
	require.False(t, *state.Evaluated)
	require.Equal(t, apiv1.NotEvaluatedPortStates, state.NotEvaluatedReason)
}
