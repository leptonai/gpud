package nvml

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetClockSpeed(t *testing.T) {
	testUUID := "GPU-12345678"

	tests := []struct {
		name                string
		graphicsClock       uint32
		graphicsReturn      nvml.Return
		memoryClock         uint32
		memoryReturn        nvml.Return
		expectedClockSpeed  ClockSpeed
		expectError         bool
		expectedErrorString string
	}{
		{
			name:           "success case - both clocks supported",
			graphicsClock:  1200,
			graphicsReturn: nvml.SUCCESS,
			memoryClock:    5000,
			memoryReturn:   nvml.SUCCESS,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            1200,
				MemoryMHz:              5000,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   true,
			},
			expectError: false,
		},
		{
			name:           "graphics clock not supported but memory clock supported",
			graphicsClock:  0,
			graphicsReturn: nvml.ERROR_NOT_SUPPORTED,
			memoryClock:    5000,
			memoryReturn:   nvml.SUCCESS,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            0,
				MemoryMHz:              5000,
				ClockGraphicsSupported: false,
				ClockMemorySupported:   true,
			},
			expectError: false,
		},
		{
			name:           "graphics clock supported but memory clock not supported",
			graphicsClock:  1200,
			graphicsReturn: nvml.SUCCESS,
			memoryClock:    0,
			memoryReturn:   nvml.ERROR_NOT_SUPPORTED,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            1200,
				MemoryMHz:              0,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   false,
			},
			expectError: false,
		},
		{
			name:           "both clocks not supported",
			graphicsClock:  0,
			graphicsReturn: nvml.ERROR_NOT_SUPPORTED,
			memoryClock:    0,
			memoryReturn:   nvml.ERROR_NOT_SUPPORTED,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            0,
				MemoryMHz:              0,
				ClockGraphicsSupported: false,
				ClockMemorySupported:   false,
			},
			expectError: false,
		},
		{
			name:           "graphics clock error",
			graphicsClock:  0,
			graphicsReturn: nvml.ERROR_UNKNOWN,
			memoryClock:    0,
			memoryReturn:   nvml.SUCCESS,
			expectedClockSpeed: ClockSpeed{
				UUID: testUUID,
			},
			expectError:         true,
			expectedErrorString: "failed to get device clock info for nvml.CLOCK_GRAPHICS",
		},
		{
			name:           "memory clock error",
			graphicsClock:  1200,
			graphicsReturn: nvml.SUCCESS,
			memoryClock:    0,
			memoryReturn:   nvml.ERROR_UNKNOWN,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            1200,
				ClockGraphicsSupported: true,
			},
			expectError:         true,
			expectedErrorString: "failed to get device clock info for nvml.CLOCK_MEM",
		},
		{
			name:           "zero clock values but still supported",
			graphicsClock:  0,
			graphicsReturn: nvml.SUCCESS,
			memoryClock:    0,
			memoryReturn:   nvml.SUCCESS,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            0,
				MemoryMHz:              0,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   true,
			},
			expectError: false,
		},
		{
			name:           "GPU lost error on graphics clock",
			graphicsClock:  0,
			graphicsReturn: nvml.ERROR_GPU_IS_LOST,
			memoryClock:    0,
			memoryReturn:   nvml.SUCCESS,
			expectedClockSpeed: ClockSpeed{
				UUID: testUUID,
			},
			expectError:         true,
			expectedErrorString: "GPU lost",
		},
		{
			name:           "GPU lost error on memory clock",
			graphicsClock:  1200,
			graphicsReturn: nvml.SUCCESS,
			memoryClock:    0,
			memoryReturn:   nvml.ERROR_GPU_IS_LOST,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            1200,
				ClockGraphicsSupported: true,
			},
			expectError:         true,
			expectedErrorString: "GPU lost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock device using the testutil helper
			mockDevice := testutil.CreateClockSpeedDevice(
				tt.graphicsClock, tt.graphicsReturn,
				tt.memoryClock, tt.memoryReturn,
				testUUID,
			)

			// Call the function being tested
			clockSpeed, err := GetClockSpeed(testUUID, mockDevice)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorString != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorString)
				}
				if tt.graphicsReturn == nvml.ERROR_GPU_IS_LOST || tt.memoryReturn == nvml.ERROR_GPU_IS_LOST {
					assert.True(t, errors.Is(err, ErrGPULost), "Expected GPU lost error")
				}
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedClockSpeed.UUID, clockSpeed.UUID)
			assert.Equal(t, tt.expectedClockSpeed.GraphicsMHz, clockSpeed.GraphicsMHz)
			assert.Equal(t, tt.expectedClockSpeed.MemoryMHz, clockSpeed.MemoryMHz)
			assert.Equal(t, tt.expectedClockSpeed.ClockGraphicsSupported, clockSpeed.ClockGraphicsSupported)
			assert.Equal(t, tt.expectedClockSpeed.ClockMemorySupported, clockSpeed.ClockMemorySupported)
		})
	}
}

// TestClockSpeedWithCustomNotSupportedMessage tests that the error handling correctly identifies
// various forms of "not supported" error messages from the NVML library
func TestClockSpeedWithCustomNotSupportedMessage(t *testing.T) {
	testUUID := "GPU-ABCDEF"

	// Test case with a custom "not supported" message
	t.Run("custom not supported message", func(t *testing.T) {
		// Override the GetClockInfo function to return a custom error
		originalErrorString := nvml.ErrorString
		defer func() { nvml.ErrorString = originalErrorString }()

		// We'll create a custom Return value that will be transformed into a "not supported" string
		customNotSupportedReturn := nvml.Return(1000)

		nvml.ErrorString = func(ret nvml.Return) string {
			if ret == customNotSupportedReturn {
				return "The operation is not supported on this device"
			}
			return originalErrorString(ret)
		}

		// Create mock device that returns our custom error for graphics and real error for memory
		mockDevice := &testutil.MockDevice{
			Device: &mock.Device{
				GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
					if clockType == nvml.CLOCK_GRAPHICS {
						return 0, customNotSupportedReturn
					}
					return 0, nvml.ERROR_NOT_SUPPORTED // standard not supported for memory
				},
			},
		}

		// Call the function
		clockSpeed, err := GetClockSpeed(testUUID, mockDevice)

		// Verify results - should have recognized both errors as "not supported"
		assert.NoError(t, err)
		assert.Equal(t, testUUID, clockSpeed.UUID)
		assert.False(t, clockSpeed.ClockGraphicsSupported)
		assert.False(t, clockSpeed.ClockMemorySupported)
		assert.Equal(t, uint32(0), clockSpeed.GraphicsMHz)
		assert.Equal(t, uint32(0), clockSpeed.MemoryMHz)
	})
}
