package nvml

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia-query/nvml/errors"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetGSPFirmwareMode(t *testing.T) {
	testCases := []struct {
		name                  string
		gspEnabled            bool
		gspSupported          bool
		gspFirmwareRet        nvml.Return
		expectedGSPFirmware   GSPFirmwareMode
		expectError           bool
		expectedErrorContains string
	}{
		{
			name:           "gsp enabled and supported",
			gspEnabled:     true,
			gspSupported:   true,
			gspFirmwareRet: nvml.SUCCESS,
			expectedGSPFirmware: GSPFirmwareMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   true,
				Supported: true,
			},
			expectError: false,
		},
		{
			name:           "gsp disabled but supported",
			gspEnabled:     false,
			gspSupported:   true,
			gspFirmwareRet: nvml.SUCCESS,
			expectedGSPFirmware: GSPFirmwareMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   false,
				Supported: true,
			},
			expectError: false,
		},
		{
			name:           "not supported",
			gspEnabled:     false,
			gspSupported:   false,
			gspFirmwareRet: nvml.ERROR_NOT_SUPPORTED,
			expectedGSPFirmware: GSPFirmwareMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   false,
				Supported: false,
			},
			expectError: false,
		},
		{
			name:                  "error case",
			gspEnabled:            false,
			gspSupported:          false,
			gspFirmwareRet:        nvml.ERROR_UNKNOWN,
			expectError:           true,
			expectedErrorContains: "failed to get gsp firmware mode",
		},
		{
			name:                  "GPU lost error",
			gspEnabled:            false,
			gspSupported:          false,
			gspFirmwareRet:        nvml.ERROR_GPU_IS_LOST,
			expectError:           true,
			expectedErrorContains: "GPU lost",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := testutil.CreateGSPFirmwareDevice(
				"test-uuid",
				tc.gspEnabled,
				tc.gspSupported,
				tc.gspFirmwareRet,
			)

			gspFirmware, err := GetGSPFirmwareMode("test-uuid", mockDevice)

			if tc.expectError {
				assert.Error(t, err)
				if tc.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorContains)
				}
				if tc.gspFirmwareRet == nvml.ERROR_GPU_IS_LOST {
					assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost), "Expected GPU lost error")
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedGSPFirmware, gspFirmware)
			}
		})
	}
}
