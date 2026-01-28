package testutil

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
)

func TestMockDeviceClocks(t *testing.T) {
	tests := []struct {
		name            string
		graphicsClock   uint32
		graphicsReturn  nvml.Return
		memClock        uint32
		memReturn       nvml.Return
		uuid            string
		expectedClocks  map[nvml.ClockType]uint32
		expectedReturns map[nvml.ClockType]nvml.Return
	}{
		{
			name:           "successful creation",
			graphicsClock:  1000,
			graphicsReturn: nvml.SUCCESS,
			memClock:       2000,
			memReturn:      nvml.SUCCESS,
			uuid:           "test-uuid",
			expectedClocks: map[nvml.ClockType]uint32{
				nvml.CLOCK_GRAPHICS: 1000,
				nvml.CLOCK_MEM:      2000,
			},
			expectedReturns: map[nvml.ClockType]nvml.Return{
				nvml.CLOCK_GRAPHICS: nvml.SUCCESS,
				nvml.CLOCK_MEM:      nvml.SUCCESS,
			},
		},
		{
			name:           "error states",
			graphicsClock:  500,
			graphicsReturn: nvml.ERROR_NOT_SUPPORTED,
			memClock:       1500,
			memReturn:      nvml.ERROR_NOT_SUPPORTED,
			uuid:           "error-uuid",
			expectedClocks: map[nvml.ClockType]uint32{
				nvml.CLOCK_GRAPHICS: 500,
				nvml.CLOCK_MEM:      1500,
			},
			expectedReturns: map[nvml.ClockType]nvml.Return{
				nvml.CLOCK_GRAPHICS: nvml.ERROR_NOT_SUPPORTED,
				nvml.CLOCK_MEM:      nvml.ERROR_NOT_SUPPORTED,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &MockDevice{
				Device: &mock.Device{
					GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
						switch clockType {
						case nvml.CLOCK_GRAPHICS:
							return tt.graphicsClock, tt.graphicsReturn
						case nvml.CLOCK_MEM:
							return tt.memClock, tt.memReturn
						default:
							return 0, nvml.ERROR_UNKNOWN
						}
					},
					GetUUIDFunc: func() (string, nvml.Return) {
						return tt.uuid, nvml.SUCCESS
					},
				},
			}

			assert.NotNil(t, mockDevice)

			// Test clock info for different clock types
			for clockType, expectedClock := range tt.expectedClocks {
				clock, ret := mockDevice.GetClockInfo(clockType)
				assert.Equal(t, expectedClock, clock)
				assert.Equal(t, tt.expectedReturns[clockType], ret)
			}

			// Test unknown clock type
			clock, ret := mockDevice.GetClockInfo(999)
			assert.Equal(t, uint32(0), clock)
			assert.Equal(t, nvml.ERROR_UNKNOWN, ret)

			// Test UUID
			uuid, ret := mockDevice.GetUUID()
			assert.Equal(t, tt.uuid, uuid)
			assert.Equal(t, nvml.SUCCESS, ret)
		})
	}
}
