package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGPMSupportedByDevice(t *testing.T) {
	testCases := []struct {
		name                  string
		gpmDeviceSupport      nvml.GpmSupport
		gpmDeviceSupportRet   nvml.Return
		expectedSupported     bool
		expectError           bool
		expectedErrorContains string
	}{
		{
			name: "supported",
			gpmDeviceSupport: nvml.GpmSupport{
				IsSupportedDevice: 1,
			},
			gpmDeviceSupportRet: nvml.SUCCESS,
			expectedSupported:   true,
			expectError:         false,
		},
		{
			name: "not supported",
			gpmDeviceSupport: nvml.GpmSupport{
				IsSupportedDevice: 0,
			},
			gpmDeviceSupportRet: nvml.SUCCESS,
			expectedSupported:   false,
			expectError:         false,
		},
		{
			name:                "not supported - API error",
			gpmDeviceSupport:    nvml.GpmSupport{},
			gpmDeviceSupportRet: nvml.ERROR_NOT_SUPPORTED,
			expectedSupported:   false,
			expectError:         false,
		},
		{
			name:                  "not supported - version mismatch",
			gpmDeviceSupport:      nvml.GpmSupport{},
			gpmDeviceSupportRet:   nvml.ERROR_INVALID_ARGUMENT,
			expectedSupported:     false,
			expectError:           true,
			expectedErrorContains: "could not query GPM support: ERROR_INVALID_ARGUMENT",
		},
		{
			name:                  "error case",
			gpmDeviceSupport:      nvml.GpmSupport{},
			gpmDeviceSupportRet:   nvml.ERROR_UNKNOWN,
			expectError:           true,
			expectedErrorContains: "could not query GPM support",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := testutil.CreateGPMSupportedDevice(
				"test-uuid",
				tc.gpmDeviceSupport,
				tc.gpmDeviceSupportRet,
			)

			supported, err := GPMSupportedByDevice(mockDevice)

			if tc.expectError {
				assert.Error(t, err)
				if tc.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedSupported, supported)
			}
		})
	}
}
