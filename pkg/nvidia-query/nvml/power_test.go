package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetPower(t *testing.T) {
	testUUID := "GPU-12345678"

	tests := []struct {
		name                       string
		powerUsage                 uint32
		enforcedPowerLimit         uint32
		managementPowerLimit       uint32
		powerUsageReturn           nvml.Return
		enforcedPowerLimitReturn   nvml.Return
		managementPowerLimitReturn nvml.Return
		expectedPower              Power
		expectError                bool
	}{
		{
			name:                       "success case - all values available",
			powerUsage:                 150000,
			enforcedPowerLimit:         250000,
			managementPowerLimit:       300000,
			powerUsageReturn:           nvml.SUCCESS,
			enforcedPowerLimitReturn:   nvml.SUCCESS,
			managementPowerLimitReturn: nvml.SUCCESS,
			expectedPower: Power{
				UUID:                             testUUID,
				UsageMilliWatts:                  150000,
				EnforcedLimitMilliWatts:          250000,
				ManagementLimitMilliWatts:        300000,
				UsedPercent:                      "60.00",
				GetPowerUsageSupported:           false,
				GetPowerLimitSupported:           false,
				GetPowerManagementLimitSupported: false,
			},
			expectError: false,
		},
		{
			name:                       "success case - enforced limit not available",
			powerUsage:                 150000,
			enforcedPowerLimit:         0,
			managementPowerLimit:       300000,
			powerUsageReturn:           nvml.SUCCESS,
			enforcedPowerLimitReturn:   nvml.ERROR_NOT_SUPPORTED,
			managementPowerLimitReturn: nvml.SUCCESS,
			expectedPower: Power{
				UUID:                             testUUID,
				UsageMilliWatts:                  150000,
				EnforcedLimitMilliWatts:          0,
				ManagementLimitMilliWatts:        300000,
				UsedPercent:                      "50.00",
				GetPowerUsageSupported:           false,
				GetPowerLimitSupported:           false,
				GetPowerManagementLimitSupported: false,
			},
			expectError: false,
		},
		{
			name:                       "success case - both limits not available",
			powerUsage:                 150000,
			enforcedPowerLimit:         0,
			managementPowerLimit:       0,
			powerUsageReturn:           nvml.SUCCESS,
			enforcedPowerLimitReturn:   nvml.ERROR_NOT_SUPPORTED,
			managementPowerLimitReturn: nvml.ERROR_NOT_SUPPORTED,
			expectedPower: Power{
				UUID:                             testUUID,
				UsageMilliWatts:                  150000,
				EnforcedLimitMilliWatts:          0,
				ManagementLimitMilliWatts:        0,
				UsedPercent:                      "0.0",
				GetPowerUsageSupported:           false,
				GetPowerLimitSupported:           false,
				GetPowerManagementLimitSupported: false,
			},
			expectError: false,
		},
		{
			name:                       "power usage not supported",
			powerUsage:                 0,
			enforcedPowerLimit:         250000,
			managementPowerLimit:       300000,
			powerUsageReturn:           nvml.ERROR_NOT_SUPPORTED,
			enforcedPowerLimitReturn:   nvml.SUCCESS,
			managementPowerLimitReturn: nvml.SUCCESS,
			expectedPower: Power{
				UUID:                             testUUID,
				UsageMilliWatts:                  0,
				EnforcedLimitMilliWatts:          250000,
				ManagementLimitMilliWatts:        300000,
				UsedPercent:                      "0.00",
				GetPowerUsageSupported:           false,
				GetPowerLimitSupported:           false,
				GetPowerManagementLimitSupported: false,
			},
			expectError: false,
		},
		{
			name:                       "power usage error",
			powerUsage:                 0,
			enforcedPowerLimit:         250000,
			managementPowerLimit:       300000,
			powerUsageReturn:           nvml.ERROR_UNKNOWN,
			enforcedPowerLimitReturn:   nvml.SUCCESS,
			managementPowerLimitReturn: nvml.SUCCESS,
			expectedPower: Power{
				UUID: testUUID,
			},
			expectError: true,
		},
		{
			name:                       "enforced power limit error",
			powerUsage:                 150000,
			enforcedPowerLimit:         0,
			managementPowerLimit:       300000,
			powerUsageReturn:           nvml.SUCCESS,
			enforcedPowerLimitReturn:   nvml.ERROR_UNKNOWN,
			managementPowerLimitReturn: nvml.SUCCESS,
			expectedPower: Power{
				UUID:                   testUUID,
				UsageMilliWatts:        150000,
				GetPowerUsageSupported: false,
			},
			expectError: true,
		},
		{
			name:                       "management power limit error",
			powerUsage:                 150000,
			enforcedPowerLimit:         250000,
			managementPowerLimit:       0,
			powerUsageReturn:           nvml.SUCCESS,
			enforcedPowerLimitReturn:   nvml.SUCCESS,
			managementPowerLimitReturn: nvml.ERROR_UNKNOWN,
			expectedPower: Power{
				UUID:                    testUUID,
				UsageMilliWatts:         150000,
				EnforcedLimitMilliWatts: 250000,
				GetPowerUsageSupported:  false,
				GetPowerLimitSupported:  false,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock device
			mockDevice := &testutil.MockDevice{
				Device: &mock.Device{
					GetPowerUsageFunc: func() (uint32, nvml.Return) {
						return tt.powerUsage, tt.powerUsageReturn
					},
					GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
						return tt.enforcedPowerLimit, tt.enforcedPowerLimitReturn
					},
					GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
						return tt.managementPowerLimit, tt.managementPowerLimitReturn
					},
				},
			}

			// Call the function being tested
			power, err := GetPower(testUUID, mockDevice)

			// Check results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPower.UUID, power.UUID)
				assert.Equal(t, tt.expectedPower.UsageMilliWatts, power.UsageMilliWatts)
				assert.Equal(t, tt.expectedPower.EnforcedLimitMilliWatts, power.EnforcedLimitMilliWatts)
				assert.Equal(t, tt.expectedPower.ManagementLimitMilliWatts, power.ManagementLimitMilliWatts)
				assert.Equal(t, tt.expectedPower.UsedPercent, power.UsedPercent)
				assert.Equal(t, tt.expectedPower.GetPowerUsageSupported, power.GetPowerUsageSupported)
				assert.Equal(t, tt.expectedPower.GetPowerLimitSupported, power.GetPowerLimitSupported)
				assert.Equal(t, tt.expectedPower.GetPowerManagementLimitSupported, power.GetPowerManagementLimitSupported)
			}
		})
	}
}

func TestGetPowerWithNilDevice(t *testing.T) {
	var nilDevice device.Device = nil
	testUUID := "GPU-NILTEST"

	// We expect the function to panic with a nil device
	assert.Panics(t, func() {
		// Call the function with a nil device
		_, _ = GetPower(testUUID, nilDevice)
	}, "Expected panic when calling GetPower with nil device")
}

func TestPowerGetUsedPercent(t *testing.T) {
	tests := []struct {
		name        string
		usedPercent string
		expected    float64
		expectError bool
	}{
		{
			name:        "valid percentage",
			usedPercent: "75.50",
			expected:    75.50,
			expectError: false,
		},
		{
			name:        "zero percentage",
			usedPercent: "0.0",
			expected:    0.0,
			expectError: false,
		},
		{
			name:        "invalid percentage",
			usedPercent: "not-a-number",
			expected:    0.0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			power := Power{
				UsedPercent: tt.usedPercent,
			}

			result, err := power.GetUsedPercent()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
