package testutil

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

func TestCreatePersistenceModeDevice(t *testing.T) {
	tests := []struct {
		name               string
		uuid               string
		persistenceMode    nvml.EnableState
		persistenceModeRet nvml.Return
	}{
		{
			name:               "Persistence mode enabled",
			uuid:               "test-uuid-1",
			persistenceMode:    nvml.FEATURE_ENABLED,
			persistenceModeRet: nvml.SUCCESS,
		},
		{
			name:               "Persistence mode disabled",
			uuid:               "test-uuid-2",
			persistenceMode:    nvml.FEATURE_DISABLED,
			persistenceModeRet: nvml.SUCCESS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := CreatePersistenceModeDevice(tt.uuid, tt.persistenceMode, tt.persistenceModeRet)
			require.NotNil(t, device)

			uuid, ret := device.GetUUID()
			require.Equal(t, nvml.SUCCESS, ret)
			require.Equal(t, tt.uuid, uuid)
		})
	}
}
