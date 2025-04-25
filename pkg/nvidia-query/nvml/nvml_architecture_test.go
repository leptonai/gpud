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
			name:           "Known architecture - Kepler",
			archValue:      nvml.DEVICE_ARCH_KEPLER,
			archReturn:     nvml.SUCCESS,
			expectedResult: "Kepler",
			expectError:    false,
		},
		{
			name:           "Known architecture - Maxwell",
			archValue:      nvml.DEVICE_ARCH_MAXWELL,
			archReturn:     nvml.SUCCESS,
			expectedResult: "Maxwell",
			expectError:    false,
		},
		{
			name:           "Known architecture - Pascal",
			archValue:      nvml.DEVICE_ARCH_PASCAL,
			archReturn:     nvml.SUCCESS,
			expectedResult: "Pascal",
			expectError:    false,
		},
		{
			name:           "Known architecture - Volta",
			archValue:      nvml.DEVICE_ARCH_VOLTA,
			archReturn:     nvml.SUCCESS,
			expectedResult: "Volta",
			expectError:    false,
		},
		{
			name:           "Known architecture - Turing",
			archValue:      nvml.DEVICE_ARCH_TURING,
			archReturn:     nvml.SUCCESS,
			expectedResult: "Turing",
			expectError:    false,
		},
		{
			name:           "Known architecture - Ampere",
			archValue:      nvml.DEVICE_ARCH_AMPERE,
			archReturn:     nvml.SUCCESS,
			expectedResult: "Ampere",
			expectError:    false,
		},
		{
			name:           "Known architecture - Ada",
			archValue:      nvml.DEVICE_ARCH_ADA,
			archReturn:     nvml.SUCCESS,
			expectedResult: "Ada",
			expectError:    false,
		},
		{
			name:           "Known architecture - Hopper",
			archValue:      nvml.DEVICE_ARCH_HOPPER,
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
			archValue:      nvml.DEVICE_ARCH_UNKNOWN,
			archReturn:     nvml.SUCCESS,
			expectedResult: "Unknown",
			expectError:    false,
		},
		{
			name:        "Error getting architecture",
			archReturn:  nvml.ERROR_NOT_SUPPORTED,
			expectError: true,
		},
		{
			name:        "Error getting architecture - generic error",
			archReturn:  nvml.ERROR_UNKNOWN,
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

func TestGetArchFamily(t *testing.T) {
	const testUUID = "test-gpu-uuid"

	tests := []struct {
		name           string
		computeMajor   int
		computeMinor   int
		computeReturn  nvml.Return
		expectedResult string
		expectError    bool
	}{
		{
			name:           "Tesla architecture",
			computeMajor:   1,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "tesla",
			expectError:    false,
		},
		{
			name:           "Fermi architecture",
			computeMajor:   2,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "fermi",
			expectError:    false,
		},
		{
			name:           "Kepler architecture",
			computeMajor:   3,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "kepler",
			expectError:    false,
		},
		{
			name:           "Maxwell architecture",
			computeMajor:   5,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "maxwell",
			expectError:    false,
		},
		{
			name:           "Pascal architecture",
			computeMajor:   6,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "pascal",
			expectError:    false,
		},
		{
			name:           "Volta architecture",
			computeMajor:   7,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "volta",
			expectError:    false,
		},
		{
			name:           "Turing architecture",
			computeMajor:   7,
			computeMinor:   5,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "turing",
			expectError:    false,
		},
		{
			name:           "Ampere architecture",
			computeMajor:   8,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "ampere",
			expectError:    false,
		},
		{
			name:           "Ada Lovelace architecture",
			computeMajor:   8,
			computeMinor:   9,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "ada-lovelace",
			expectError:    false,
		},
		{
			name:           "Hopper architecture",
			computeMajor:   9,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "hopper",
			expectError:    false,
		},
		{
			name:           "Blackwell architecture - variant 10",
			computeMajor:   10,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "blackwell",
			expectError:    false,
		},
		{
			name:           "Blackwell architecture - variant 12",
			computeMajor:   12,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "blackwell",
			expectError:    false,
		},
		{
			name:           "Undefined architecture",
			computeMajor:   20,
			computeMinor:   0,
			computeReturn:  nvml.SUCCESS,
			expectedResult: "undefined",
			expectError:    false,
		},
		{
			name:          "Error getting compute capability",
			computeReturn: nvml.ERROR_NOT_SUPPORTED,
			expectError:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock device
			mockDevice := &testutil.MockDevice{
				Device: &mock.Device{
					GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
						return tc.computeMajor, tc.computeMinor, tc.computeReturn
					},
					GetUUIDFunc: func() (string, nvml.Return) {
						return testUUID, nvml.SUCCESS
					},
				},
			}

			// Call the function being tested
			result, err := GetArchFamily(mockDevice)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestGetArchFamilyHelper(t *testing.T) {
	tests := []struct {
		name           string
		computeMajor   int
		computeMinor   int
		expectedResult string
	}{
		{"Tesla", 1, 0, "tesla"},
		{"Fermi", 2, 0, "fermi"},
		{"Kepler", 3, 0, "kepler"},
		{"Maxwell", 5, 0, "maxwell"},
		{"Pascal", 6, 0, "pascal"},
		{"Volta", 7, 0, "volta"},
		{"Turing", 7, 5, "turing"},
		{"Ampere", 8, 0, "ampere"},
		{"Ada Lovelace", 8, 9, "ada-lovelace"},
		{"Hopper", 9, 0, "hopper"},
		{"Blackwell 10.0", 10, 0, "blackwell"},
		{"Blackwell 12.0", 12, 0, "blackwell"},
		{"Undefined", 20, 0, "undefined"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getArchFamily(tc.computeMajor, tc.computeMinor)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
