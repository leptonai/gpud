package nvml

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia-query/nvml/errors"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetTemperature(t *testing.T) {
	testUUID := "GPU-12345678"

	tests := []struct {
		name                string
		currentTemp         uint32
		currentTempReturn   nvml.Return
		shutdownTemp        uint32
		shutdownTempReturn  nvml.Return
		slowdownTemp        uint32
		slowdownTempReturn  nvml.Return
		memMaxTemp          uint32
		memMaxTempReturn    nvml.Return
		gpuMaxTemp          uint32
		gpuMaxTempReturn    nvml.Return
		expectedTemperature Temperature
	}{
		{
			name:               "success case all thresholds available",
			currentTemp:        70,
			currentTempReturn:  nvml.SUCCESS,
			shutdownTemp:       100,
			shutdownTempReturn: nvml.SUCCESS,
			slowdownTemp:       90,
			slowdownTempReturn: nvml.SUCCESS,
			memMaxTemp:         95,
			memMaxTempReturn:   nvml.SUCCESS,
			gpuMaxTemp:         85,
			gpuMaxTempReturn:   nvml.SUCCESS,
			expectedTemperature: Temperature{
				UUID:                     testUUID,
				CurrentCelsiusGPUCore:    70,
				ThresholdCelsiusShutdown: 100,
				ThresholdCelsiusSlowdown: 90,
				ThresholdCelsiusMemMax:   95,
				ThresholdCelsiusGPUMax:   85,
				UsedPercentShutdown:      "70.00",
				UsedPercentSlowdown:      "77.78",
				UsedPercentMemMax:        "73.68",
				UsedPercentGPUMax:        "82.35",
			},
		},
		{
			name:               "success case with zero shutdown threshold",
			currentTemp:        70,
			currentTempReturn:  nvml.SUCCESS,
			shutdownTemp:       0,
			shutdownTempReturn: nvml.SUCCESS,
			slowdownTemp:       90,
			slowdownTempReturn: nvml.SUCCESS,
			memMaxTemp:         95,
			memMaxTempReturn:   nvml.SUCCESS,
			gpuMaxTemp:         85,
			gpuMaxTempReturn:   nvml.SUCCESS,
			expectedTemperature: Temperature{
				UUID:                     testUUID,
				CurrentCelsiusGPUCore:    70,
				ThresholdCelsiusShutdown: 0,
				ThresholdCelsiusSlowdown: 90,
				ThresholdCelsiusMemMax:   95,
				ThresholdCelsiusGPUMax:   85,
				UsedPercentShutdown:      "0.0",
				UsedPercentSlowdown:      "77.78",
				UsedPercentMemMax:        "73.68",
				UsedPercentGPUMax:        "82.35",
			},
		},
		{
			name:               "error getting current temperature",
			currentTemp:        0,
			currentTempReturn:  nvml.ERROR_UNKNOWN,
			shutdownTemp:       100,
			shutdownTempReturn: nvml.SUCCESS,
			slowdownTemp:       90,
			slowdownTempReturn: nvml.SUCCESS,
			memMaxTemp:         95,
			memMaxTempReturn:   nvml.SUCCESS,
			gpuMaxTemp:         85,
			gpuMaxTempReturn:   nvml.SUCCESS,
			expectedTemperature: Temperature{
				UUID:                     testUUID,
				CurrentCelsiusGPUCore:    0,
				ThresholdCelsiusShutdown: 100,
				ThresholdCelsiusSlowdown: 90,
				ThresholdCelsiusMemMax:   95,
				ThresholdCelsiusGPUMax:   85,
				UsedPercentShutdown:      "0.00",
				UsedPercentSlowdown:      "0.00",
				UsedPercentMemMax:        "0.00",
				UsedPercentGPUMax:        "0.00",
			},
		},
		{
			name:               "error getting shutdown threshold",
			currentTemp:        70,
			currentTempReturn:  nvml.SUCCESS,
			shutdownTemp:       0,
			shutdownTempReturn: nvml.ERROR_UNKNOWN,
			slowdownTemp:       90,
			slowdownTempReturn: nvml.SUCCESS,
			memMaxTemp:         95,
			memMaxTempReturn:   nvml.SUCCESS,
			gpuMaxTemp:         85,
			gpuMaxTempReturn:   nvml.SUCCESS,
			expectedTemperature: Temperature{
				UUID:                     testUUID,
				CurrentCelsiusGPUCore:    70,
				ThresholdCelsiusShutdown: 0,
				ThresholdCelsiusSlowdown: 90,
				ThresholdCelsiusMemMax:   95,
				ThresholdCelsiusGPUMax:   85,
				UsedPercentShutdown:      "0.0",
				UsedPercentSlowdown:      "77.78",
				UsedPercentMemMax:        "73.68",
				UsedPercentGPUMax:        "82.35",
			},
		},
		{
			name:               "error getting slowdown threshold",
			currentTemp:        70,
			currentTempReturn:  nvml.SUCCESS,
			shutdownTemp:       100,
			shutdownTempReturn: nvml.SUCCESS,
			slowdownTemp:       0,
			slowdownTempReturn: nvml.ERROR_UNKNOWN,
			memMaxTemp:         95,
			memMaxTempReturn:   nvml.SUCCESS,
			gpuMaxTemp:         85,
			gpuMaxTempReturn:   nvml.SUCCESS,
			expectedTemperature: Temperature{
				UUID:                     testUUID,
				CurrentCelsiusGPUCore:    70,
				ThresholdCelsiusShutdown: 100,
				ThresholdCelsiusSlowdown: 0,
				ThresholdCelsiusMemMax:   95,
				ThresholdCelsiusGPUMax:   85,
				UsedPercentShutdown:      "70.00",
				UsedPercentSlowdown:      "0.0",
				UsedPercentMemMax:        "73.68",
				UsedPercentGPUMax:        "82.35",
			},
		},
		{
			name:               "error getting memory max threshold",
			currentTemp:        70,
			currentTempReturn:  nvml.SUCCESS,
			shutdownTemp:       100,
			shutdownTempReturn: nvml.SUCCESS,
			slowdownTemp:       90,
			slowdownTempReturn: nvml.SUCCESS,
			memMaxTemp:         0,
			memMaxTempReturn:   nvml.ERROR_UNKNOWN,
			gpuMaxTemp:         85,
			gpuMaxTempReturn:   nvml.SUCCESS,
			expectedTemperature: Temperature{
				UUID:                     testUUID,
				CurrentCelsiusGPUCore:    70,
				ThresholdCelsiusShutdown: 100,
				ThresholdCelsiusSlowdown: 90,
				ThresholdCelsiusMemMax:   0,
				ThresholdCelsiusGPUMax:   85,
				UsedPercentShutdown:      "70.00",
				UsedPercentSlowdown:      "77.78",
				UsedPercentMemMax:        "0.0",
				UsedPercentGPUMax:        "82.35",
			},
		},
		{
			name:               "error getting GPU max threshold",
			currentTemp:        70,
			currentTempReturn:  nvml.SUCCESS,
			shutdownTemp:       100,
			shutdownTempReturn: nvml.SUCCESS,
			slowdownTemp:       90,
			slowdownTempReturn: nvml.SUCCESS,
			memMaxTemp:         95,
			memMaxTempReturn:   nvml.SUCCESS,
			gpuMaxTemp:         0,
			gpuMaxTempReturn:   nvml.ERROR_UNKNOWN,
			expectedTemperature: Temperature{
				UUID:                     testUUID,
				CurrentCelsiusGPUCore:    70,
				ThresholdCelsiusShutdown: 100,
				ThresholdCelsiusSlowdown: 90,
				ThresholdCelsiusMemMax:   95,
				ThresholdCelsiusGPUMax:   0,
				UsedPercentShutdown:      "70.00",
				UsedPercentSlowdown:      "77.78",
				UsedPercentMemMax:        "73.68",
				UsedPercentGPUMax:        "0.0",
			},
		},
		{
			name:               "all thresholds not supported",
			currentTemp:        70,
			currentTempReturn:  nvml.SUCCESS,
			shutdownTemp:       0,
			shutdownTempReturn: nvml.ERROR_NOT_SUPPORTED,
			slowdownTemp:       0,
			slowdownTempReturn: nvml.ERROR_NOT_SUPPORTED,
			memMaxTemp:         0,
			memMaxTempReturn:   nvml.ERROR_NOT_SUPPORTED,
			gpuMaxTemp:         0,
			gpuMaxTempReturn:   nvml.ERROR_NOT_SUPPORTED,
			expectedTemperature: Temperature{
				UUID:                     testUUID,
				CurrentCelsiusGPUCore:    70,
				ThresholdCelsiusShutdown: 0,
				ThresholdCelsiusSlowdown: 0,
				ThresholdCelsiusMemMax:   0,
				ThresholdCelsiusGPUMax:   0,
				UsedPercentShutdown:      "0.0",
				UsedPercentSlowdown:      "0.0",
				UsedPercentMemMax:        "0.0",
				UsedPercentGPUMax:        "0.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock device
			mockDevice := &testutil.MockDevice{
				Device: &mock.Device{
					GetTemperatureFunc: func(sensor nvml.TemperatureSensors) (uint32, nvml.Return) {
						if sensor == nvml.TEMPERATURE_GPU {
							return tt.currentTemp, tt.currentTempReturn
						}
						return 0, nvml.ERROR_INVALID_ARGUMENT
					},
					GetTemperatureThresholdFunc: func(thresholdType nvml.TemperatureThresholds) (uint32, nvml.Return) {
						switch thresholdType {
						case nvml.TEMPERATURE_THRESHOLD_SHUTDOWN:
							return tt.shutdownTemp, tt.shutdownTempReturn
						case nvml.TEMPERATURE_THRESHOLD_SLOWDOWN:
							return tt.slowdownTemp, tt.slowdownTempReturn
						case nvml.TEMPERATURE_THRESHOLD_MEM_MAX:
							return tt.memMaxTemp, tt.memMaxTempReturn
						case nvml.TEMPERATURE_THRESHOLD_GPU_MAX:
							return tt.gpuMaxTemp, tt.gpuMaxTempReturn
						default:
							return 0, nvml.ERROR_INVALID_ARGUMENT
						}
					},
				},
			}

			// Call the function being tested
			temperature, err := GetTemperature(testUUID, mockDevice)

			// We don't expect errors from GetTemperature
			assert.NoError(t, err)

			// Check all temperature fields
			assert.Equal(t, tt.expectedTemperature.UUID, temperature.UUID)
			assert.Equal(t, tt.expectedTemperature.CurrentCelsiusGPUCore, temperature.CurrentCelsiusGPUCore)
			assert.Equal(t, tt.expectedTemperature.ThresholdCelsiusShutdown, temperature.ThresholdCelsiusShutdown)
			assert.Equal(t, tt.expectedTemperature.ThresholdCelsiusSlowdown, temperature.ThresholdCelsiusSlowdown)
			assert.Equal(t, tt.expectedTemperature.ThresholdCelsiusMemMax, temperature.ThresholdCelsiusMemMax)
			assert.Equal(t, tt.expectedTemperature.ThresholdCelsiusGPUMax, temperature.ThresholdCelsiusGPUMax)
			assert.Equal(t, tt.expectedTemperature.UsedPercentShutdown, temperature.UsedPercentShutdown)
			assert.Equal(t, tt.expectedTemperature.UsedPercentSlowdown, temperature.UsedPercentSlowdown)
			assert.Equal(t, tt.expectedTemperature.UsedPercentMemMax, temperature.UsedPercentMemMax)
			assert.Equal(t, tt.expectedTemperature.UsedPercentGPUMax, temperature.UsedPercentGPUMax)
		})
	}
}

// TestGetUsedPercentMethods tests the helper methods for parsing percentage strings
func TestGetUsedPercentMethods(t *testing.T) {
	// Create a Temperature struct with known values
	temp := Temperature{
		UsedPercentShutdown: "75.50",
		UsedPercentSlowdown: "80.25",
		UsedPercentMemMax:   "90.75",
		UsedPercentGPUMax:   "95.00",
	}

	// Test GetUsedPercentShutdown
	shutdown, err := temp.GetUsedPercentShutdown()
	assert.NoError(t, err)
	assert.Equal(t, 75.50, shutdown)

	// Test GetUsedPercentSlowdown
	slowdown, err := temp.GetUsedPercentSlowdown()
	assert.NoError(t, err)
	assert.Equal(t, 80.25, slowdown)

	// Test GetUsedPercentMemMax
	memMax, err := temp.GetUsedPercentMemMax()
	assert.NoError(t, err)
	assert.Equal(t, 90.75, memMax)

	// Test GetUsedPercentGPUMax
	gpuMax, err := temp.GetUsedPercentGPUMax()
	assert.NoError(t, err)
	assert.Equal(t, 95.00, gpuMax)

	// Test with invalid values
	invalidTemp := Temperature{
		UsedPercentShutdown: "not-a-number",
	}
	_, err = invalidTemp.GetUsedPercentShutdown()
	assert.Error(t, err)
}

// TestGetTemperatureWithNilDevice tests the behavior of GetTemperature when passed a nil device.
func TestGetTemperatureWithNilDevice(t *testing.T) {
	var nilDevice device.Device = nil
	testUUID := "GPU-NILTEST"

	// We expect the function to panic with a nil device
	assert.Panics(t, func() {
		// Call the function with a nil device
		_, _ = GetTemperature(testUUID, nilDevice)
	}, "Expected panic when calling GetTemperature with nil device")
}

// TestGetTemperatureEdgeCases tests edge cases for the temperature function
func TestGetTemperatureEdgeCases(t *testing.T) {
	testUUID := "GPU-EDGECASES"

	// Test case: Zero temperature
	t.Run("zero temperature", func(t *testing.T) {
		mockDevice := &testutil.MockDevice{
			Device: &mock.Device{
				GetTemperatureFunc: func(sensor nvml.TemperatureSensors) (uint32, nvml.Return) {
					return 0, nvml.SUCCESS
				},
				GetTemperatureThresholdFunc: func(thresholdType nvml.TemperatureThresholds) (uint32, nvml.Return) {
					return 100, nvml.SUCCESS
				},
			},
		}

		temperature, err := GetTemperature(testUUID, mockDevice)
		assert.NoError(t, err)
		assert.Equal(t, uint32(0), temperature.CurrentCelsiusGPUCore)
		assert.Equal(t, "0.00", temperature.UsedPercentShutdown)
	})

	// Test case: Very high temperature (close to threshold)
	t.Run("high temperature", func(t *testing.T) {
		mockDevice := &testutil.MockDevice{
			Device: &mock.Device{
				GetTemperatureFunc: func(sensor nvml.TemperatureSensors) (uint32, nvml.Return) {
					return 99, nvml.SUCCESS
				},
				GetTemperatureThresholdFunc: func(thresholdType nvml.TemperatureThresholds) (uint32, nvml.Return) {
					return 100, nvml.SUCCESS
				},
			},
		}

		temperature, err := GetTemperature(testUUID, mockDevice)
		assert.NoError(t, err)
		assert.Equal(t, uint32(99), temperature.CurrentCelsiusGPUCore)
		assert.Equal(t, "99.00", temperature.UsedPercentShutdown)
	})

	// Test case: Temperature exactly at threshold
	t.Run("temperature at threshold", func(t *testing.T) {
		mockDevice := &testutil.MockDevice{
			Device: &mock.Device{
				GetTemperatureFunc: func(sensor nvml.TemperatureSensors) (uint32, nvml.Return) {
					return 100, nvml.SUCCESS
				},
				GetTemperatureThresholdFunc: func(thresholdType nvml.TemperatureThresholds) (uint32, nvml.Return) {
					return 100, nvml.SUCCESS
				},
			},
		}

		temperature, err := GetTemperature(testUUID, mockDevice)
		assert.NoError(t, err)
		assert.Equal(t, uint32(100), temperature.CurrentCelsiusGPUCore)
		assert.Equal(t, "100.00", temperature.UsedPercentShutdown)
	})
}

// TestGetTemperatureWithGPULostError tests that GetTemperature correctly handles GPU lost errors
func TestGetTemperatureWithGPULostError(t *testing.T) {
	testUUID := "GPU-LOST"

	// Create a mock device that returns GPU_IS_LOST error
	mockDevice := &testutil.MockDevice{
		Device: &mock.Device{
			GetTemperatureFunc: func(sensor nvml.TemperatureSensors) (uint32, nvml.Return) {
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetTemperatureThresholdFunc: func(thresholdType nvml.TemperatureThresholds) (uint32, nvml.Return) {
				// This function needs to be mocked but its return values don't matter
				// since the first call to GetTemperature will fail
				return 0, nvml.ERROR_UNKNOWN
			},
		},
	}

	// Call the function
	_, err := GetTemperature(testUUID, mockDevice)

	// Check error handling
	assert.Error(t, err)
	assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost), "Expected GPU lost error")
}

// TestGetTemperatureWithGPULostErrorCases tests all cases where the temperature functions can return GPU lost errors
func TestGetTemperatureWithGPULostErrorCases(t *testing.T) {
	testCases := []struct {
		name               string
		currentTempRet     nvml.Return
		shutdownThreshRet  nvml.Return
		slowdownThreshRet  nvml.Return
		memMaxThreshRet    nvml.Return
		gpuMaxThreshRet    nvml.Return
		expectedErrorMatch bool
	}{
		{
			name:               "GPU lost in current temperature",
			currentTempRet:     nvml.ERROR_GPU_IS_LOST,
			shutdownThreshRet:  nvml.SUCCESS,
			slowdownThreshRet:  nvml.SUCCESS,
			memMaxThreshRet:    nvml.SUCCESS,
			gpuMaxThreshRet:    nvml.SUCCESS,
			expectedErrorMatch: true,
		},
		{
			name:               "GPU lost in shutdown threshold",
			currentTempRet:     nvml.SUCCESS,
			shutdownThreshRet:  nvml.ERROR_GPU_IS_LOST,
			slowdownThreshRet:  nvml.SUCCESS,
			memMaxThreshRet:    nvml.SUCCESS,
			gpuMaxThreshRet:    nvml.SUCCESS,
			expectedErrorMatch: true,
		},
		{
			name:               "GPU lost in slowdown threshold",
			currentTempRet:     nvml.SUCCESS,
			shutdownThreshRet:  nvml.SUCCESS,
			slowdownThreshRet:  nvml.ERROR_GPU_IS_LOST,
			memMaxThreshRet:    nvml.SUCCESS,
			gpuMaxThreshRet:    nvml.SUCCESS,
			expectedErrorMatch: true,
		},
		{
			name:               "GPU lost in memory max threshold",
			currentTempRet:     nvml.SUCCESS,
			shutdownThreshRet:  nvml.SUCCESS,
			slowdownThreshRet:  nvml.SUCCESS,
			memMaxThreshRet:    nvml.ERROR_GPU_IS_LOST,
			gpuMaxThreshRet:    nvml.SUCCESS,
			expectedErrorMatch: true,
		},
		{
			name:               "GPU lost in GPU max threshold",
			currentTempRet:     nvml.SUCCESS,
			shutdownThreshRet:  nvml.SUCCESS,
			slowdownThreshRet:  nvml.SUCCESS,
			memMaxThreshRet:    nvml.SUCCESS,
			gpuMaxThreshRet:    nvml.ERROR_GPU_IS_LOST,
			expectedErrorMatch: true,
		},
		{
			name:               "No GPU lost errors",
			currentTempRet:     nvml.SUCCESS,
			shutdownThreshRet:  nvml.SUCCESS,
			slowdownThreshRet:  nvml.SUCCESS,
			memMaxThreshRet:    nvml.SUCCESS,
			gpuMaxThreshRet:    nvml.SUCCESS,
			expectedErrorMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testUUID := "GPU-LOST-TEST"

			// Create a mock device with specified returns for each call
			mockDevice := &testutil.MockDevice{
				Device: &mock.Device{
					GetTemperatureFunc: func(sensor nvml.TemperatureSensors) (uint32, nvml.Return) {
						return 0, tc.currentTempRet
					},
					GetTemperatureThresholdFunc: func(thresholdType nvml.TemperatureThresholds) (uint32, nvml.Return) {
						switch thresholdType {
						case nvml.TEMPERATURE_THRESHOLD_SHUTDOWN:
							return 0, tc.shutdownThreshRet
						case nvml.TEMPERATURE_THRESHOLD_SLOWDOWN:
							return 0, tc.slowdownThreshRet
						case nvml.TEMPERATURE_THRESHOLD_MEM_MAX:
							return 0, tc.memMaxThreshRet
						case nvml.TEMPERATURE_THRESHOLD_GPU_MAX:
							return 0, tc.gpuMaxThreshRet
						default:
							return 0, nvml.ERROR_INVALID_ARGUMENT
						}
					},
				},
			}

			// Call the function
			_, err := GetTemperature(testUUID, mockDevice)

			// Verify results
			if tc.expectedErrorMatch {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost), "Expected GPU lost error for case: %s", tc.name)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
