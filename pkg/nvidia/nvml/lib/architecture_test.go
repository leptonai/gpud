package lib

import (
	"errors"
	"testing"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

func TestArchitectureSymbolAvailability(t *testing.T) {
	for _, available := range []bool{false, true} {
		t.Run(map[bool]string{false: "old driver", true: "Rubin driver"}[available], func(t *testing.T) {
			called := false
			lib := &devInterface{
				lookupSymbol: func(name string) error {
					require.Equal(t, "nvmlDeviceGetArchitecture", name)
					if !available {
						return errors.New("symbol not found")
					}
					return nil
				},
				devices: []nvlibdevice.Device{testutil.NewMockDevice(&mock.Device{
					GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
						called = true
						return nvml.DEVICE_ARCH_RUBIN, nvml.SUCCESS
					},
				}, "", "", "", "")},
			}
			devices, err := lib.GetDevices()
			require.NoError(t, err)
			arch, ret := devices[0].GetArchitecture()
			require.Equal(t, available, called)
			if available {
				require.Equal(t, nvml.SUCCESS, ret)
				require.Equal(t, nvml.DeviceArchitecture(nvml.DEVICE_ARCH_RUBIN), arch)
			} else {
				require.Equal(t, nvml.ERROR_FUNCTION_NOT_FOUND, ret)
			}
		})
	}
}
