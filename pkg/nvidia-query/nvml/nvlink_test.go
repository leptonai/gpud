package nvml

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
)

// Mock device implementation
type mockDevice struct {
	device.Device
	nvLinkState       nvml.EnableState
	nvLinkStateErr    nvml.Return
	replayErrors      uint64
	replayErrorsErr   nvml.Return
	recoveryErrors    uint64
	recoveryErrorsErr nvml.Return
	crcErrors         uint64
	crcErrorsErr      nvml.Return
	fieldValuesErr    nvml.Return
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

	defer func() {
		// Restore original functions
		nvml.DeviceGetNvLinkState = origDeviceGetNvLinkState
		nvml.DeviceGetNvLinkErrorCounter = origDeviceGetNvLinkErrorCounter
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
		expectError            bool
		expectedErrorContains  string
	}{
		{
			name: "NVLink supported and working",
			mockDev: &mockDevice{
				nvLinkState:       nvml.FEATURE_ENABLED,
				nvLinkStateErr:    nvml.SUCCESS,
				replayErrors:      10,
				replayErrorsErr:   nvml.SUCCESS,
				recoveryErrors:    20,
				recoveryErrorsErr: nvml.SUCCESS,
				crcErrors:         30,
				crcErrorsErr:      nvml.SUCCESS,
				fieldValuesErr:    nvml.SUCCESS,
			},
			expectedSupported:      true,
			expectedStatesCount:    nvml.NVLINK_MAX_LINKS,
			expectedFeatureEnabled: true,
			expectedReplayErrors:   10,
			expectedRecoveryErrors: 20,
			expectedCRCErrors:      30,
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
				nvLinkState:       nvml.FEATURE_ENABLED,
				nvLinkStateErr:    nvml.ERROR_UNKNOWN,
				replayErrors:      0,
				replayErrorsErr:   nvml.SUCCESS,
				recoveryErrors:    0,
				recoveryErrorsErr: nvml.SUCCESS,
				crcErrors:         0,
				crcErrorsErr:      nvml.SUCCESS,
				fieldValuesErr:    nvml.SUCCESS,
			},
			expectedSupported:   true,
			expectedStatesCount: 0,
		},
		{
			name: "GPU lost error",
			mockDev: &mockDevice{
				nvLinkStateErr: nvml.ERROR_GPU_IS_LOST,
			},
			expectedSupported:     false,
			expectedStatesCount:   0,
			expectError:           true,
			expectedErrorContains: "gpu lost",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Mock the NVML functions
			nvml.DeviceGetNvLinkErrorCounter = func(device nvml.Device, link int, counter nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
				return tc.mockDev.GetNvLinkErrorCounter(link, counter)
			}

			nvlink, err := GetNVLink("test-uuid", tc.mockDev)

			if tc.expectError {
				assert.Error(t, err)
				if tc.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorContains)
				}
				if tc.mockDev.nvLinkStateErr == nvml.ERROR_GPU_IS_LOST {
					assert.True(t, errors.Is(err, ErrGPULost), "Expected GPU lost error")
				}
				return
			}

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
				}
			}
		})
	}
}
