package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetArchitecture(t *testing.T) {
	const testUUID = "test-gpu-uuid"

	tests := []struct {
		name           string
		archValue      nvml.DeviceArchitecture
		archReturn     nvml.Return
		expectedResult string
		expectError    bool
	}{
		{
			name:           "Known architecture - Ampere",
			archValue:      7, // NVML_DEVICE_ARCH_AMPERE
			archReturn:     nvml.SUCCESS,
			expectedResult: "Ampere",
			expectError:    false,
		},
		{
			name:           "Known architecture - Hopper",
			archValue:      9, // NVML_DEVICE_ARCH_HOPPER
			archReturn:     nvml.SUCCESS,
			expectedResult: "Hopper",
			expectError:    false,
		},
		{
			name:           "Unknown architecture code",
			archValue:      123,
			archReturn:     nvml.SUCCESS,
			expectedResult: "UnknownArchitecture(123)",
			expectError:    false,
		},
		{
			name:           "Unknown architecture - explicit unknown",
			archValue:      0xffffffff,
			archReturn:     nvml.SUCCESS,
			expectedResult: "Unknown",
			expectError:    false,
		},
		{
			name:        "Error getting architecture",
			archReturn:  nvml.ERROR_NOT_SUPPORTED,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock device
			mockDevice := &testutil.MockDevice{
				Device: &mock.Device{
					GetArchitectureFunc: func() (nvml.DeviceArchitecture, nvml.Return) {
						return tc.archValue, tc.archReturn
					},
					GetUUIDFunc: func() (string, nvml.Return) {
						return testUUID, nvml.SUCCESS
					},
				},
			}

			// Call the function being tested
			result, err := GetArchitecture(mockDevice)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}
