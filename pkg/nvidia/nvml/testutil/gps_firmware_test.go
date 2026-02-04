package testutil

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

func TestCreateGSPFirmwareDevice(t *testing.T) {
	tests := []struct {
		name         string
		uuid         string
		gspEnabled   bool
		gspSupported bool
		returnCode   nvml.Return
	}{
		{
			name:         "GSP enabled and supported",
			uuid:         "test-uuid-1",
			gspEnabled:   true,
			gspSupported: true,
			returnCode:   nvml.SUCCESS,
		},
		{
			name:         "GSP disabled but supported",
			uuid:         "test-uuid-2",
			gspEnabled:   false,
			gspSupported: true,
			returnCode:   nvml.SUCCESS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := CreateGSPFirmwareDevice(tt.uuid, tt.gspEnabled, tt.gspSupported, tt.returnCode)
			require.NotNil(t, device)

			uuid, ret := device.GetUUID()
			require.Equal(t, nvml.SUCCESS, ret)
			require.Equal(t, tt.uuid, uuid)
		})
	}
}
