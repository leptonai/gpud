package remappedrows

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestPartialMemoryTelemetryIsNotHealthy(t *testing.T) {
	dev := testutil.NewMockDevice(&mock.Device{}, "rubin", "NVIDIA", "", "0000:01:00.0")
	inst := &mockNVMLInstance{
		getDevicesFunc: func() map[string]device.Device {
			return map[string]device.Device{"GPU-0": dev, "GPU-1": dev, "GPU-2": dev, "GPU-3": dev}
		},
		getProductNameFunc: func() string { return "NVIDIA-Graphics-Device" },
		getMemoryErrorManagementCapabilitiesFunc: func() product.MemoryErrorManagementCapabilities {
			return product.CapabilitiesForGPU("NVIDIA-Graphics-Device", "rubin", product.HostLinkUnknown).MemoryManagement
		},
	}
	comp, err := New(&components.GPUdInstance{RootCtx: t.Context(), NVMLInstance: inst})
	require.NoError(t, err)
	defer func() { require.NoError(t, comp.Close()) }()
	c := mustComponent(t, comp)
	queryErr1, queryErr2 := errors.New("GPU-1 query failed"), errors.New("GPU-2 query failed")
	c.getRemappedRowsFunc = func(uuid string, _ device.Device) (RemappedRows, error) {
		switch uuid {
		case "GPU-1":
			return RemappedRows{}, queryErr1
		case "GPU-2":
			return RemappedRows{}, queryErr2
		default:
			return RemappedRows{UUID: uuid, Supported: true}, nil
		}
	}
	result := c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, result.health)
	require.ErrorIs(t, result.err, queryErr1)
	require.ErrorIs(t, result.err, queryErr2)
	require.Len(t, result.RemappedRows, 2)
	require.Nil(t, result.suggestedActions)
}
