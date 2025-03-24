package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
)

// Mock device implementation
type mockDevice struct {
	device.Device
	nvLinkState           nvml.EnableState
	nvLinkStateErr        nvml.Return
	replayErrors          uint64
	replayErrorsErr       nvml.Return
	recoveryErrors        uint64
	recoveryErrorsErr     nvml.Return
	crcErrors             uint64
	crcErrorsErr          nvml.Return
	fieldValuesErr        nvml.Return
	rawRxBytes            uint64
	rawTxBytes            uint64
	utilizationCounterErr nvml.Return
}

func (m *mockDevice) GetNvLinkState(link int) (nvml.EnableState, nvml.Return) {
	return m.nvLinkState, m.nvLinkStateErr
}

func (m *mockDevice) GetNvLinkErrorCounter(link int, counter nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
	switch counter {
	case nvml.NVLINK_ERROR_DL_REPLAY:
		return m.replayErrors, m.replayErrorsErr
	case nvml.NVLINK_ERROR_DL_RECOVERY:
		return m.recoveryErrors, m.recoveryErrorsErr
	case nvml.NVLINK_ERROR_DL_CRC_FLIT:
		return m.crcErrors, m.crcErrorsErr
	default:
		return 0, nvml.ERROR_UNKNOWN
	}
}

func (m *mockDevice) GetFieldValues(values []nvml.FieldValue) nvml.Return {
	if m.fieldValuesErr != nvml.SUCCESS {
		return m.fieldValuesErr
	}

	for i := range values {
		switch values[i].FieldId {
		case nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX:
			// Mock the byte slice value - assuming little endian
			for j := 0; j < 8; j++ {
				values[i].Value[j] = byte((m.rawTxBytes >> (j * 8)) & 0xff)
			}
		case nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX:
			// Mock the byte slice value - assuming little endian
			for j := 0; j < 8; j++ {
				values[i].Value[j] = byte((m.rawRxBytes >> (j * 8)) & 0xff)
			}
		}
	}
	return nvml.SUCCESS
}

func (m *mockDevice) GetNvLinkUtilizationCounter(link int, counter int) (uint64, uint64, nvml.Return) {
	return m.rawRxBytes, m.rawTxBytes, m.utilizationCounterErr
}

// TestNVLinkStatesAllFeatureEnabled tests the AllFeatureEnabled method
func TestNVLinkStatesAllFeatureEnabled(t *testing.T) {
	tests := []struct {
		name     string
		states   NVLinkStates
		expected bool
	}{
		{
			name: "All links enabled",
			states: NVLinkStates{
				{Link: 0, FeatureEnabled: true},
				{Link: 1, FeatureEnabled: true},
			},
			expected: true,
		},
		{
			name: "Some links disabled",
			states: NVLinkStates{
				{Link: 0, FeatureEnabled: true},
				{Link: 1, FeatureEnabled: false},
			},
			expected: false,
		},
		{
			name:     "Empty states",
			states:   NVLinkStates{},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.states.AllFeatureEnabled()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestNVLinkStatesTotalCounters tests the total counters methods
func TestNVLinkStatesTotalCounters(t *testing.T) {
	states := NVLinkStates{
		{
			Link:           0,
			ReplayErrors:   10,
			RecoveryErrors: 20,
			CRCErrors:      30,
		},
		{
			Link:           1,
			ReplayErrors:   15,
			RecoveryErrors: 25,
			CRCErrors:      35,
		},
	}

	t.Run("TotalRelayErrors", func(t *testing.T) {
		assert.Equal(t, uint64(25), states.TotalRelayErrors())
	})

	t.Run("TotalRecoveryErrors", func(t *testing.T) {
		assert.Equal(t, uint64(45), states.TotalRecoveryErrors())
	})

	t.Run("TotalCRCErrors", func(t *testing.T) {
		assert.Equal(t, uint64(65), states.TotalCRCErrors())
	})
}

// TestGetNVLink tests the GetNVLink function with various device responses
func TestGetNVLink(t *testing.T) {
	// Override the NVML functions
	origDeviceGetNvLinkState := nvml.DeviceGetNvLinkState
	origDeviceGetNvLinkErrorCounter := nvml.DeviceGetNvLinkErrorCounter
	origDeviceGetFieldValues := nvml.DeviceGetFieldValues
	origDeviceGetNvLinkUtilizationCounter := nvml.DeviceGetNvLinkUtilizationCounter

	defer func() {
		// Restore original functions
		nvml.DeviceGetNvLinkState = origDeviceGetNvLinkState
		nvml.DeviceGetNvLinkErrorCounter = origDeviceGetNvLinkErrorCounter
		nvml.DeviceGetFieldValues = origDeviceGetFieldValues
		nvml.DeviceGetNvLinkUtilizationCounter = origDeviceGetNvLinkUtilizationCounter
	}()

	tests := []struct {
		name                   string
		mockDev                *mockDevice
		expectedSupported      bool
		expectedStatesCount    int
		expectedFeatureEnabled bool
		expectedReplayErrors   uint64
		expectedRecoveryErrors uint64
		expectedCRCErrors      uint64
		expectedRawTxBytes     uint64
		expectedRawRxBytes     uint64
	}{
		{
			name: "NVLink supported and working",
			mockDev: &mockDevice{
				nvLinkState:           nvml.FEATURE_ENABLED,
				nvLinkStateErr:        nvml.SUCCESS,
				replayErrors:          10,
				replayErrorsErr:       nvml.SUCCESS,
				recoveryErrors:        20,
				recoveryErrorsErr:     nvml.SUCCESS,
				crcErrors:             30,
				crcErrorsErr:          nvml.SUCCESS,
				rawTxBytes:            100,
				rawRxBytes:            200,
				fieldValuesErr:        nvml.SUCCESS,
				utilizationCounterErr: nvml.SUCCESS,
			},
			expectedSupported:      true,
			expectedStatesCount:    nvml.NVLINK_MAX_LINKS,
			expectedFeatureEnabled: true,
			expectedReplayErrors:   10,
			expectedRecoveryErrors: 20,
			expectedCRCErrors:      30,
			expectedRawTxBytes:     100 * 1024, // KiB to bytes
			expectedRawRxBytes:     200 * 1024, // KiB to bytes
		},
		{
			name: "NVLink not supported",
			mockDev: &mockDevice{
				nvLinkStateErr: nvml.ERROR_NOT_SUPPORTED,
			},
			expectedSupported:   false,
			expectedStatesCount: 0,
		},
		{
			name: "NVLink state error but continue",
			mockDev: &mockDevice{
				nvLinkState:           nvml.FEATURE_ENABLED,
				nvLinkStateErr:        nvml.ERROR_UNKNOWN,
				replayErrors:          0,
				replayErrorsErr:       nvml.SUCCESS,
				recoveryErrors:        0,
				recoveryErrorsErr:     nvml.SUCCESS,
				crcErrors:             0,
				crcErrorsErr:          nvml.SUCCESS,
				rawTxBytes:            0,
				rawRxBytes:            0,
				fieldValuesErr:        nvml.SUCCESS,
				utilizationCounterErr: nvml.SUCCESS,
			},
			expectedSupported:   true,
			expectedStatesCount: 0,
		},
		{
			name: "Fallback to GetNvLinkUtilizationCounter",
			mockDev: &mockDevice{
				nvLinkState:           nvml.FEATURE_ENABLED,
				nvLinkStateErr:        nvml.SUCCESS,
				replayErrors:          10,
				replayErrorsErr:       nvml.SUCCESS,
				recoveryErrors:        20,
				recoveryErrorsErr:     nvml.SUCCESS,
				crcErrors:             30,
				crcErrorsErr:          nvml.SUCCESS,
				rawTxBytes:            100,
				rawRxBytes:            200,
				fieldValuesErr:        nvml.ERROR_UNKNOWN,
				utilizationCounterErr: nvml.SUCCESS,
			},
			expectedSupported:      true,
			expectedStatesCount:    nvml.NVLINK_MAX_LINKS,
			expectedFeatureEnabled: true,
			expectedReplayErrors:   10,
			expectedRecoveryErrors: 20,
			expectedCRCErrors:      30,
			expectedRawTxBytes:     100 * 1024, // KiB to bytes
			expectedRawRxBytes:     200 * 1024, // KiB to bytes
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Mock the NVML functions
			nvml.DeviceGetNvLinkState = func(device nvml.Device, link int) (nvml.EnableState, nvml.Return) {
				return tc.mockDev.GetNvLinkState(link)
			}
			nvml.DeviceGetNvLinkErrorCounter = func(device nvml.Device, link int, counter nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
				return tc.mockDev.GetNvLinkErrorCounter(link, counter)
			}
			nvml.DeviceGetFieldValues = func(device nvml.Device, values []nvml.FieldValue) nvml.Return {
				return tc.mockDev.GetFieldValues(values)
			}
			nvml.DeviceGetNvLinkUtilizationCounter = func(device nvml.Device, link int, counter int) (uint64, uint64, nvml.Return) {
				return tc.mockDev.GetNvLinkUtilizationCounter(link, counter)
			}

			nvlink, err := GetNVLink("test-uuid", tc.mockDev)

			// No error should be returned
			assert.NoError(t, err)

			// Check the nvlink structure
			assert.Equal(t, "test-uuid", nvlink.UUID)
			assert.Equal(t, tc.expectedSupported, nvlink.Supported)
			assert.Equal(t, tc.expectedStatesCount, len(nvlink.States))

			// Check state values if applicable
			if tc.expectedStatesCount > 0 {
				for _, state := range nvlink.States {
					assert.Equal(t, tc.expectedFeatureEnabled, state.FeatureEnabled)
					assert.Equal(t, tc.expectedReplayErrors, state.ReplayErrors)
					assert.Equal(t, tc.expectedRecoveryErrors, state.RecoveryErrors)
					assert.Equal(t, tc.expectedCRCErrors, state.CRCErrors)
					assert.Equal(t, tc.expectedRawTxBytes, state.ThroughputRawTxBytes)
					assert.Equal(t, tc.expectedRawRxBytes, state.ThroughputRawRxBytes)
				}
			}
		})
	}
}
