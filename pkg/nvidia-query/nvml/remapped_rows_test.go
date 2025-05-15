package nvml

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestRemappedRows_QualifiesForRMA(t *testing.T) {
	tests := []struct {
		name                           string
		remappingFailed                bool
		remappedDueToUncorrectableErrs int
		want                           bool
	}{
		{
			name:                           "qualifies when remapping failed with <8 uncorrectable errors",
			remappingFailed:                true,
			remappedDueToUncorrectableErrs: 5,
			want:                           true,
		},
		{
			name:                           "qualifies when remapping failed with >=8 uncorrectable errors",
			remappingFailed:                true,
			remappedDueToUncorrectableErrs: 8,
			want:                           true,
		},
		{
			name:                           "does not qualify when remapping hasn't failed",
			remappingFailed:                false,
			remappedDueToUncorrectableErrs: 8,
			want:                           false,
		},
		{
			name:                           "does not qualify when remapping hasn't failed with <8 errors",
			remappingFailed:                false,
			remappedDueToUncorrectableErrs: 5,
			want:                           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RemappedRows{
				RemappingFailed:                  tt.remappingFailed,
				RemappedDueToUncorrectableErrors: tt.remappedDueToUncorrectableErrs,
			}
			if got := r.QualifiesForRMA(); got != tt.want {
				t.Errorf("RemappedRows.QualifiesForRMA() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemappedRows_RequiresReset(t *testing.T) {
	tests := []struct {
		name             string
		remappingPending bool
		want             bool
	}{
		{
			name:             "requires reset when remapping is pending",
			remappingPending: true,
			want:             true,
		},
		{
			name:             "does not require reset when no remapping is pending",
			remappingPending: false,
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RemappedRows{
				RemappingPending: tt.remappingPending,
			}
			if got := r.RequiresReset(); got != tt.want {
				t.Errorf("RemappedRows.RequiresReset() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRemappedRows(t *testing.T) {
	testUUID := "GPU-12345678"

	tests := []struct {
		name                 string
		corrRows             int
		uncRows              int
		isPending            bool
		failureOccurred      bool
		ret                  nvml.Return
		expectedRemappedRows RemappedRows
		expectError          bool
		errorContains        string
		expectedErrorType    error
	}{
		{
			name:            "success case",
			corrRows:        2,
			uncRows:         1,
			isPending:       true,
			failureOccurred: false,
			ret:             nvml.SUCCESS,
			expectedRemappedRows: RemappedRows{
				UUID:                             testUUID,
				RemappedDueToCorrectableErrors:   2,
				RemappedDueToUncorrectableErrors: 1,
				RemappingPending:                 true,
				RemappingFailed:                  false,
				Supported:                        true,
			},
			expectError:       false,
			errorContains:     "",
			expectedErrorType: nil,
		},
		{
			name:            "feature not supported",
			corrRows:        0,
			uncRows:         0,
			isPending:       false,
			failureOccurred: false,
			ret:             nvml.ERROR_NOT_SUPPORTED,
			expectedRemappedRows: RemappedRows{
				UUID:                             testUUID,
				RemappedDueToCorrectableErrors:   0,
				RemappedDueToUncorrectableErrors: 0,
				RemappingPending:                 false,
				RemappingFailed:                  false,
				Supported:                        false,
			},
			expectError:       false,
			errorContains:     "",
			expectedErrorType: nil,
		},
		{
			name:            "severe error with failure",
			corrRows:        8,
			uncRows:         7,
			isPending:       true,
			failureOccurred: true,
			ret:             nvml.SUCCESS,
			expectedRemappedRows: RemappedRows{
				UUID:                             testUUID,
				RemappedDueToCorrectableErrors:   8,
				RemappedDueToUncorrectableErrors: 7,
				RemappingPending:                 true,
				RemappingFailed:                  true,
				Supported:                        true,
			},
			expectError:       false,
			errorContains:     "",
			expectedErrorType: nil,
		},
		{
			name:            "other error",
			corrRows:        0,
			uncRows:         0,
			isPending:       false,
			failureOccurred: false,
			ret:             nvml.ERROR_UNKNOWN,
			expectedRemappedRows: RemappedRows{
				UUID:      testUUID,
				Supported: true,
			},
			expectError:       true,
			errorContains:     "failed to get device remapped rows",
			expectedErrorType: nil,
		},
		{
			name:            "GPU lost error",
			corrRows:        0,
			uncRows:         0,
			isPending:       false,
			failureOccurred: false,
			ret:             nvml.ERROR_GPU_IS_LOST,
			expectedRemappedRows: RemappedRows{
				UUID:      testUUID,
				Supported: true,
			},
			expectError:       true,
			errorContains:     "",
			expectedErrorType: ErrGPULost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock device
			mockDevice := &testutil.MockDevice{
				Device: &mock.Device{
					GetRemappedRowsFunc: func() (int, int, bool, bool, nvml.Return) {
						return tt.corrRows, tt.uncRows, tt.isPending, tt.failureOccurred, tt.ret
					},
				},
			}

			// Call the function being tested
			rows, err := GetRemappedRows(testUUID, mockDevice)

			// Check results
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				if tt.expectedErrorType != nil {
					assert.True(t, errors.Is(err, tt.expectedErrorType), "Expected error type %v but got %v", tt.expectedErrorType, err)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedRemappedRows.UUID, rows.UUID)
				assert.Equal(t, tt.expectedRemappedRows.RemappedDueToCorrectableErrors, rows.RemappedDueToCorrectableErrors)
				assert.Equal(t, tt.expectedRemappedRows.RemappedDueToUncorrectableErrors, rows.RemappedDueToUncorrectableErrors)
				assert.Equal(t, tt.expectedRemappedRows.RemappingPending, rows.RemappingPending)
				assert.Equal(t, tt.expectedRemappedRows.RemappingFailed, rows.RemappingFailed)
				assert.Equal(t, tt.expectedRemappedRows.Supported, rows.Supported)
			}
		})
	}
}

func TestGetRemappedRowsWithNilDevice(t *testing.T) {
	var nilDevice device.Device = nil
	testUUID := "GPU-NILTEST"

	// We expect the function to panic with a nil device
	assert.Panics(t, func() {
		// Call the function with a nil device
		_, _ = GetRemappedRows(testUUID, nilDevice)
	}, "Expected panic when calling GetRemappedRows with nil device")
}
