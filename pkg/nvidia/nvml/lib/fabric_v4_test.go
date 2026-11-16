package lib

import (
	"errors"
	"testing"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

func TestFabricV4SymbolGuardThroughDeviceWrapper(t *testing.T) {
	for _, available := range []bool{false, true} {
		t.Run(map[bool]string{false: "older driver", true: "V4 driver"}[available], func(t *testing.T) {
			called := false
			library := &devInterface{
				lookupSymbol: func(name string) error {
					require.Equal(t, "nvmlDeviceGetGpuFabricInfo_v4", name)
					if !available {
						return errors.New("missing symbol")
					}
					return nil
				},
				devices: []nvlibdevice.Device{testutil.NewMockDevice(&mock.Device{
					GetUUIDFunc: func() (string, nvml.Return) { return "GPU-test", nvml.SUCCESS },
					GetGpuFabricInfo_v4Func: func() (nvml.GpuFabricInfo_v4, nvml.Return) {
						called = true
						info := nvml.GpuFabricInfo_v4{NumCliques: 1}
						info.Cliques[0] = nvml.GpuFabricClique_v1{Type: 2, Id: 32766}
						return info, nvml.SUCCESS
					},
					GetGpuFabricInfoFunc: func() (nvml.GpuFabricInfo, nvml.Return) { return nvml.GpuFabricInfo{CliqueId: 65534}, nvml.SUCCESS },
				}, "rubin", "NVIDIA", "", "")},
			}
			devices, err := library.GetDevices()
			require.NoError(t, err)
			// Use the production wrapper path, including capability discovery.
			state, err := device.New(devices[0], "", device.WithRubin(true)).GetFabricState()
			require.NoError(t, err)
			require.Equal(t, available, called)
			if available {
				require.Nil(t, state.LegacyCliqueID())
				require.Len(t, state.Cliques, 1)
			} else {
				require.Equal(t, uint32(65534), *state.LegacyCliqueID())
				require.Nil(t, state.Cliques)
			}
		})
	}
}
