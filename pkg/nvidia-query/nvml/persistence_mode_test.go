package nvml

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetPersistenceMode(t *testing.T) {
	testCases := []struct {
		name                    string
		persistenceMode         nvml.EnableState
		persistenceModeRet      nvml.Return
		expectedPersistenceMode PersistenceMode
		expectError             bool
		expectedErrorContains   string
	}{
		{
			name:               "persistence mode enabled",
			persistenceMode:    nvml.FEATURE_ENABLED,
			persistenceModeRet: nvml.SUCCESS,
			expectedPersistenceMode: PersistenceMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   true,
				Supported: true,
			},
			expectError: false,
		},
		{
			name:               "persistence mode disabled",
			persistenceMode:    nvml.FEATURE_DISABLED,
			persistenceModeRet: nvml.SUCCESS,
			expectedPersistenceMode: PersistenceMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   false,
				Supported: true,
			},
			expectError: false,
		},
		{
			name:               "not supported",
			persistenceMode:    nvml.FEATURE_DISABLED,
			persistenceModeRet: nvml.ERROR_NOT_SUPPORTED,
			expectedPersistenceMode: PersistenceMode{
				UUID:      "test-uuid",
				BusID:     "test-pci",
				Enabled:   false,
				Supported: false,
			},
			expectError: false,
		},
		{
			name:                  "error case",
			persistenceMode:       nvml.FEATURE_DISABLED,
			persistenceModeRet:    nvml.ERROR_UNKNOWN,
			expectError:           true,
			expectedErrorContains: "failed to get device persistence mode",
		},
		{
			name:                  "GPU lost error",
			persistenceMode:       nvml.FEATURE_DISABLED,
			persistenceModeRet:    nvml.ERROR_GPU_IS_LOST,
			expectError:           true,
			expectedErrorContains: "GPU lost",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := testutil.CreatePersistenceModeDevice(
				"test-uuid",
				tc.persistenceMode,
				tc.persistenceModeRet,
			)

			persistenceMode, err := GetPersistenceMode("test-uuid", mockDevice)

			if tc.expectError {
				assert.Error(t, err)
				if tc.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrorContains)
				}
				if tc.persistenceModeRet == nvml.ERROR_GPU_IS_LOST {
					assert.True(t, errors.Is(err, ErrGPULost), "Expected GPU lost error")
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedPersistenceMode, persistenceMode)
			}
		})
	}
}
