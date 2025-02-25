package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetUtilization(t *testing.T) {
	testUUID := "GPU-12345678"

	tests := []struct {
		name                string
		gpuUtilization      uint32
		memoryUtilization   uint32
		mockReturn          nvml.Return
		expectedUtilization Utilization
		expectError         bool
	}{
		{
			name:              "success case",
			gpuUtilization:    75,
			memoryUtilization: 50,
			mockReturn:        nvml.SUCCESS,
			expectedUtilization: Utilization{
				UUID:              testUUID,
				GPUUsedPercent:    75,
				MemoryUsedPercent: 50,
				Supported:         true,
			},
			expectError: false,
		},
		{
			name:              "not supported case",
			gpuUtilization:    0,
			memoryUtilization: 0,
			mockReturn:        nvml.ERROR_NOT_SUPPORTED,
			expectedUtilization: Utilization{
				UUID:      testUUID,
				Supported: false,
			},
			expectError: false,
		},
		{
			name:              "error case",
			gpuUtilization:    0,
			memoryUtilization: 0,
			mockReturn:        nvml.ERROR_UNKNOWN,
			expectedUtilization: Utilization{
				UUID:      testUUID,
				Supported: true,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock device
			mockDevice := &testutil.MockDevice{
				Device: &mock.Device{
					GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
						return nvml.Utilization{
							Gpu:    tt.gpuUtilization,
							Memory: tt.memoryUtilization,
						}, tt.mockReturn
					},
				},
			}

			// Call the function being tested
			utilization, err := GetUtilization(testUUID, mockDevice)

			// Check results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUtilization, utilization)
			}
		})
	}
}

// TestUtilizationWithCustomMockDevice creates a custom mock device implementation
// to test the GetUtilization function with more control over the mocked behavior.
func TestUtilizationWithCustomMockDevice(t *testing.T) {
	testUUID := "GPU-ABCDEF"

	// Test case: device returns partial data before error
	t.Run("custom error handling", func(t *testing.T) {
		// Custom mock device with testutil.MockDevice wrapper
		mockDevice := &testutil.MockDevice{
			Device: &mock.Device{
				GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
					// Return a non-success, non-not-supported error
					return nvml.Utilization{}, nvml.ERROR_INVALID_ARGUMENT
				},
			},
		}

		// Call the function being tested
		utilization, err := GetUtilization(testUUID, mockDevice)

		// Verify error is returned and UUID is preserved
		assert.Error(t, err)
		assert.Equal(t, testUUID, utilization.UUID)
		assert.True(t, utilization.Supported) // Should be true before error check
	})

	// Test case: zero utilization values
	t.Run("zero utilization values", func(t *testing.T) {
		mockDevice := &testutil.MockDevice{
			Device: &mock.Device{
				GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
					return nvml.Utilization{
						Gpu:    0,
						Memory: 0,
					}, nvml.SUCCESS
				},
			},
		}

		utilization, err := GetUtilization(testUUID, mockDevice)

		assert.NoError(t, err)
		assert.Equal(t, testUUID, utilization.UUID)
		assert.Equal(t, uint32(0), utilization.GPUUsedPercent)
		assert.Equal(t, uint32(0), utilization.MemoryUsedPercent)
		assert.True(t, utilization.Supported)
	})

	// Test case: max utilization values
	t.Run("max utilization values", func(t *testing.T) {
		mockDevice := &testutil.MockDevice{
			Device: &mock.Device{
				GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
					return nvml.Utilization{
						Gpu:    100,
						Memory: 100,
					}, nvml.SUCCESS
				},
			},
		}

		utilization, err := GetUtilization(testUUID, mockDevice)

		assert.NoError(t, err)
		assert.Equal(t, testUUID, utilization.UUID)
		assert.Equal(t, uint32(100), utilization.GPUUsedPercent)
		assert.Equal(t, uint32(100), utilization.MemoryUsedPercent)
		assert.True(t, utilization.Supported)
	})
}

// TestGetUtilizationWithNilDevice tests the behavior of GetUtilization when passed a nil device.
// This is a defensive test to ensure proper error handling in case of nil devices.
func TestGetUtilizationWithNilDevice(t *testing.T) {
	var nilDevice device.Device = nil
	testUUID := "GPU-NILTEST"

	// We expect the function to panic with a nil device
	assert.Panics(t, func() {
		// Call the function with a nil device
		_, _ = GetUtilization(testUUID, nilDevice)
	}, "Expected panic when calling GetUtilization with nil device")
}
