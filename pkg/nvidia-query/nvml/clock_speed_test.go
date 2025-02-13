package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetClockSpeed(t *testing.T) {
	testUUID := "GPU-123456"

	tests := []struct {
		name               string
		graphicsClockRet   nvml.Return
		graphicsClock      uint32
		memClockRet        nvml.Return
		memClock           uint32
		expectedClockSpeed ClockSpeed
		expectError        bool
	}{
		{
			name:             "Success - Both clocks supported",
			graphicsClockRet: nvml.SUCCESS,
			graphicsClock:    1500,
			memClockRet:      nvml.SUCCESS,
			memClock:         2000,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            1500,
				MemoryMHz:              2000,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   true,
			},
			expectError: false,
		},
		{
			name:             "Graphics clock not supported",
			graphicsClockRet: nvml.ERROR_NOT_SUPPORTED,
			graphicsClock:    0,
			memClockRet:      nvml.SUCCESS,
			memClock:         2000,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            0,
				MemoryMHz:              2000,
				ClockGraphicsSupported: false,
				ClockMemorySupported:   true,
			},
			expectError: false,
		},
		{
			name:             "Memory clock not supported",
			graphicsClockRet: nvml.SUCCESS,
			graphicsClock:    1500,
			memClockRet:      nvml.ERROR_NOT_SUPPORTED,
			memClock:         0,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            1500,
				MemoryMHz:              0,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   false,
			},
			expectError: false,
		},
		{
			name:             "Graphics clock error",
			graphicsClockRet: nvml.ERROR_UNKNOWN,
			graphicsClock:    0,
			memClockRet:      nvml.SUCCESS,
			memClock:         2000,
			expectedClockSpeed: ClockSpeed{
				UUID: testUUID,
			},
			expectError: true,
		},
		{
			name:             "Memory clock error",
			graphicsClockRet: nvml.SUCCESS,
			graphicsClock:    1500,
			memClockRet:      nvml.ERROR_UNKNOWN,
			memClock:         0,
			expectedClockSpeed: ClockSpeed{
				UUID:                   testUUID,
				GraphicsMHz:            1500,
				ClockGraphicsSupported: true,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock device using testutil
			dev := testutil.CreateClockSpeedDevice(tt.graphicsClock, tt.graphicsClockRet, tt.memClock, tt.memClockRet, testUUID)

			// Call GetClockSpeed
			clockSpeed, err := GetClockSpeed(testUUID, dev)

			if tt.expectError {
				assert.Error(t, err)
				if tt.graphicsClockRet == nvml.SUCCESS {
					assert.Equal(t, tt.expectedClockSpeed.GraphicsMHz, clockSpeed.GraphicsMHz)
					assert.Equal(t, tt.expectedClockSpeed.ClockGraphicsSupported, clockSpeed.ClockGraphicsSupported)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedClockSpeed, clockSpeed)
			}
		})
	}
}
