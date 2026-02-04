package testutil

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

func TestCreateGPMSupportedDevice(t *testing.T) {
	tests := []struct {
		name                string
		uuid                string
		gpmDeviceSupport    nvml.GpmSupport
		gpmDeviceSupportRet nvml.Return
	}{
		{
			name: "GPM supported device",
			uuid: "test-uuid-1",
			gpmDeviceSupport: nvml.GpmSupport{
				IsSupportedDevice: 1,
			},
			gpmDeviceSupportRet: nvml.SUCCESS,
		},
		{
			name: "GPM not supported device",
			uuid: "test-uuid-2",
			gpmDeviceSupport: nvml.GpmSupport{
				IsSupportedDevice: 0,
			},
			gpmDeviceSupportRet: nvml.SUCCESS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := CreateGPMSupportedDevice(tt.uuid, tt.gpmDeviceSupport, tt.gpmDeviceSupportRet)
			require.NotNil(t, device)

			uuid, ret := device.GetUUID()
			require.Equal(t, nvml.SUCCESS, ret)
			require.Equal(t, tt.uuid, uuid)
		})
	}
}

func TestCreateGPMSampleDevice(t *testing.T) {
	tests := []struct {
		name         string
		uuid         string
		sampleGetRet nvml.Return
	}{
		{
			name:         "Successful sample get",
			uuid:         "test-uuid-1",
			sampleGetRet: nvml.SUCCESS,
		},
		{
			name:         "Failed sample get",
			uuid:         "test-uuid-2",
			sampleGetRet: nvml.ERROR_NOT_SUPPORTED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := CreateGPMSampleDevice(tt.uuid, tt.sampleGetRet)
			require.NotNil(t, device)

			uuid, ret := device.GetUUID()
			require.Equal(t, nvml.SUCCESS, ret)
			require.Equal(t, tt.uuid, uuid)
		})
	}
}
