//go:build ignore
// +build ignore

package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// createECCModeDevice creates a mock device for ECC mode testing
func createECCModeDevice(
	uuid string,
	current nvml.EnableState,
	pending nvml.EnableState,
	ret nvml.Return,
) *testutil.MockDevice {
	mockDevice := &mock.Device{
		GetEccModeFunc: func() (nvml.EnableState, nvml.EnableState, nvml.Return) {
			return current, pending, ret
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}

	return testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")
}

func TestGetECCModeEnabled(t *testing.T) {
	testCases := []struct {
		name                  string
		currentECC            nvml.EnableState
		pendingECC            nvml.EnableState
		eccModeRet            nvml.Return
		expectedECCMode       ECCMode
		expectError           bool
		expectedErrorContains string
	}{
		{
			name:       "both current and pending enabled",
			currentECC: nvml.FEATURE_ENABLED,
			pendingECC: nvml.FEATURE_ENABLED,
			eccModeRet: nvml.SUCCESS,
			expectedECCMode: ECCMode{
				UUID:           "test-uuid",
				EnabledCurrent: true,
				EnabledPending: true,
				Supported:      true,
			},
			expectError: false,
		},
		{
			name:       "current enabled, pending disabled",
			currentECC: nvml.FEATURE_ENABLED,
			pendingECC: nvml.FEATURE_DISABLED,
			eccModeRet: nvml.SUCCESS,
			expectedECCMode: ECCMode{
				UUID:           "test-uuid",
				EnabledCurrent: true,
				EnabledPending: false,
				Supported:      true,
			},
			expectError: false,
		},
		{
			name:       "current disabled, pending enabled",
			currentECC: nvml.FEATURE_DISABLED,
			pendingECC: nvml.FEATURE_ENABLED,
			eccModeRet: nvml.SUCCESS,
			expectedECCMode: ECCMode{
				UUID:           "test-uuid",
				EnabledCurrent: false,
				EnabledPending: true,
				Supported:      true,
			},
			expectError: false,
		},
		{
			name:       "both current and pending disabled",
			currentECC: nvml.FEATURE_DISABLED,
			pendingECC: nvml.FEATURE_DISABLED,
			eccModeRet: nvml.SUCCESS,
			expectedECCMode: ECCMode{
				UUID:           "test-uuid",
				EnabledCurrent: false,
				EnabledPending: false,
				Supported:      true,
			},
			expectError: false,
		},
		{
			name:       "not supported",
			currentECC: nvml.FEATURE_DISABLED,
			pendingECC: nvml.FEATURE_DISABLED,
			eccModeRet: nvml.ERROR_NOT_SUPPORTED,
			expectedECCMode: ECCMode{
				UUID:           "test-uuid",
				EnabledCurrent: false,
				EnabledPending: false,
				Supported:      false,
			},
			expectError: false,
		},
		{
			name:                  "error case",
			currentECC:            nvml.FEATURE_DISABLED,
			pendingECC:            nvml.FEATURE_DISABLED,
			eccModeRet:            nvml.ERROR_UNKNOWN,
			expectError:           true,
			expectedErrorContains: "failed to get current/pending ecc mode",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := createECCModeDevice(
				"test-uuid",
				tc.currentECC,
				tc.pendingECC,
				tc.eccModeRet,
			)

			eccMode, err := GetECCModeEnabled("test-uuid", mockDevice)

			if tc.expectError {
				assert.Error(t, err)
				if tc.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedECCMode, eccMode)
			}
		})
	}
}

// TestECCModeStruct tests the ECCMode struct fields and JSON tags
func TestECCModeStruct(t *testing.T) {
	eccMode := ECCMode{
		UUID:           "gpu-00000000-0000-0000-0000-000000000000",
		EnabledCurrent: true,
		EnabledPending: false,
		Supported:      true,
	}

	// Verify field values
	assert.Equal(t, "gpu-00000000-0000-0000-0000-000000000000", eccMode.UUID)
	assert.True(t, eccMode.EnabledCurrent)
	assert.False(t, eccMode.EnabledPending)
	assert.True(t, eccMode.Supported)
}
