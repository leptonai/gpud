package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

func TestGetBrand(t *testing.T) {
	const testUUID = "test-gpu-uuid"

	tests := []struct {
		name           string
		brandValue     nvml.BrandType
		brandReturn    nvml.Return
		expectedResult string
		expectError    bool
	}{
		{
			name:           "Known brand - Tesla",
			brandValue:     nvml.BRAND_TESLA,
			brandReturn:    nvml.SUCCESS,
			expectedResult: "Tesla",
			expectError:    false,
		},
		{
			name:           "Known brand - GeForce RTX",
			brandValue:     nvml.BRAND_GEFORCE_RTX,
			brandReturn:    nvml.SUCCESS,
			expectedResult: "GeForce RTX",
			expectError:    false,
		},
		{
			name:           "Unknown brand code",
			brandValue:     nvml.BrandType(999),
			brandReturn:    nvml.SUCCESS,
			expectedResult: "UnknownBrand(999)",
			expectError:    false,
		},
		{
			name:           "Unknown brand - explicit unknown",
			brandValue:     nvml.BRAND_UNKNOWN,
			brandReturn:    nvml.SUCCESS,
			expectedResult: "Unknown",
			expectError:    false,
		},
		{
			name:        "Error getting brand",
			brandReturn: nvml.ERROR_NOT_SUPPORTED,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock device
			mockDevice := &testutil.MockDevice{
				Device: &mock.Device{
					GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
						return tc.brandValue, tc.brandReturn
					},
					GetUUIDFunc: func() (string, nvml.Return) {
						return testUUID, nvml.SUCCESS
					},
				},
			}

			// Call the function being tested
			result, err := GetBrand(mockDevice)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}
