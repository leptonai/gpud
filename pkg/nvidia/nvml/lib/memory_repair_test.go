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

// An older driver lacks these symbols; calling through go-nvml would abort the
// process, so the wrapper must not reach the underlying device.
func TestMemoryRepairSymbolAvailability(t *testing.T) {
	for _, tc := range []struct {
		symbol string
		call   func(nvlibdevice.Device) nvml.Return
	}{
		{"nvmlDeviceGetBankRemapperStatus_v1", func(d nvlibdevice.Device) nvml.Return { _, ret := d.GetBankRemapperStatus_v1(); return ret }},
		{"nvmlDeviceGetRepairStatus", func(d nvlibdevice.Device) nvml.Return { _, ret := d.GetRepairStatus(); return ret }},
		{"nvmlDeviceGetUnrepairableMemoryFlag_v1", func(d nvlibdevice.Device) nvml.Return { _, ret := d.GetUnrepairableMemoryFlag_v1(); return ret }},
	} {
		for _, available := range []bool{false, true} {
			t.Run(tc.symbol+map[bool]string{false: "/old driver", true: "/Rubin driver"}[available], func(t *testing.T) {
				called := false
				lib := &devInterface{
					lookupSymbol: func(name string) error {
						require.Equal(t, tc.symbol, name)
						if !available {
							return errors.New("symbol not found")
						}
						return nil
					},
					devices: []nvlibdevice.Device{testutil.NewMockDevice(&mock.Device{
						GetBankRemapperStatus_v1Func: func() (nvml.EccBankRemapperStatus_v1, nvml.Return) {
							called = true
							return nvml.EccBankRemapperStatus_v1{}, nvml.SUCCESS
						},
						GetRepairStatusFunc: func() (nvml.RepairStatus, nvml.Return) {
							called = true
							return nvml.RepairStatus{}, nvml.SUCCESS
						},
						GetUnrepairableMemoryFlag_v1Func: func() (nvml.UnrepairableMemoryStatus_v1, nvml.Return) {
							called = true
							return nvml.UnrepairableMemoryStatus_v1{}, nvml.SUCCESS
						},
					}, "", "", "", "")},
				}
				devices, err := lib.GetDevices()
				require.NoError(t, err)
				ret := tc.call(devices[0])
				require.Equal(t, available, called)
				if available {
					require.Equal(t, nvml.SUCCESS, ret)
				} else {
					require.Equal(t, nvml.ERROR_FUNCTION_NOT_FOUND, ret)
				}
			})
		}
	}
}
