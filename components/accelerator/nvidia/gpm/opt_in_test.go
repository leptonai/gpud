package gpm

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

func TestBindingUpgradeDoesNotAutomaticallyEnableGPM(t *testing.T) {
	for _, optIn := range []string{"", "true"} {
		t.Run("enabled="+optIn, func(t *testing.T) {
			t.Setenv(EnvEnableGPM, optIn)
			c, err := New(&components.GPUdInstance{RootCtx: t.Context(), NVMLInstance: &mockNVMLInstance{}})
			require.NoError(t, err)
			defer func() { require.NoError(t, c.Close()) }()
			called := false
			dev := testutil.NewMockDevice(&mock.Device{GpmQueryDeviceSupportFunc: func() (nvml.GpmSupport, nvml.Return) {
				called = true
				return nvml.GpmSupport{IsSupportedDevice: 1}, nvml.SUCCESS
			}}, "hopper", "NVIDIA", "9.0", "")
			supported, err := c.(*component).getGPMSupportedFunc(dev)
			if optIn == "true" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errSamplingDisabled)
			}
			require.Equal(t, optIn == "true", supported)
			require.Equal(t, optIn == "true", called)
			if optIn == "" {
				c.(*component).nvmlInstance = &mockNVMLInstance{devicesFunc: func() map[string]device.Device { return map[string]device.Device{"GPU-0": dev} }}
				result := c.Check().(*checkResult)
				require.Equal(t, errSamplingDisabled.Error(), result.reason)
				require.Nil(t, result.suggestedActions)
				require.False(t, called)
			}
		})
	}
}
