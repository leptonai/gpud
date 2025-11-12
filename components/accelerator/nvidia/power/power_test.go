package power

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia-query/nvml/errors"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetUsedPercent(t *testing.T) {
	tests := []struct {
		name        string
		power       Power
		expected    float64
		expectErr   bool
		expectedErr error
	}{
		{
			name: "valid percent",
			power: Power{
				UsedPercent: "75.50",
			},
			expected:  75.50,
			expectErr: false,
		},
		{
			name: "zero percent",
			power: Power{
				UsedPercent: "0.0",
			},
			expected:  0.0,
			expectErr: false,
		},
		{
			name: "invalid percent",
			power: Power{
				UsedPercent: "not-a-float",
			},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.power.GetUsedPercent()

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

// TestGetPower tests the real GetPower function from the package
func TestGetPower(t *testing.T) {
	t.Run("test GPU lost on power usage", func(t *testing.T) {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})

	// Test GPU requires reset on power usage
	t.Run("test GPU requires reset on power usage", func(t *testing.T) {
		originalErrorString := nvml.ErrorString
		nvml.ErrorString = func(ret nvml.Return) string {
			if ret == nvml.Return(4242) {
				return "GPU requires reset"
			}
			return originalErrorString(ret)
		}
		defer func() { nvml.ErrorString = originalErrorString }()

		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 0, nvml.Return(4242)
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})

	// Test power limit GPU lost error
	t.Run("test GPU lost on power limit", func(t *testing.T) {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})

	// Test power management GPU lost error
	t.Run("test GPU lost on power management", func(t *testing.T) {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 200, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})

	// Test successful case
	t.Run("successful power query", func(t *testing.T) {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 200, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 250, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		power, err := GetPower("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.Equal(t, "GPU-TEST", power.UUID)
		assert.Equal(t, uint32(100), power.UsageMilliWatts)
		assert.Equal(t, uint32(200), power.EnforcedLimitMilliWatts)
		assert.Equal(t, uint32(250), power.ManagementLimitMilliWatts)
		assert.Equal(t, "50.00", power.UsedPercent) // 100/200 * 100 = 50.00
	})

	// Test not supported cases
	t.Run("power usage not supported", func(t *testing.T) {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 200, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 250, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		power, err := GetPower("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.False(t, power.GetPowerUsageSupported)
	})
}
