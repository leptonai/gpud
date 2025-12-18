package device

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinDriverVersionForV3FabricAPI(t *testing.T) {
	t.Parallel()

	// Verify the constant is set correctly per NVIDIA documentation
	// nvmlDeviceGetGpuFabricInfoV was introduced in driver 550
	// See: https://docs.nvidia.com/deploy/nvml-api/change-log.html
	assert.Equal(t, 550, MinDriverVersionForV3FabricAPI,
		"MinDriverVersionForV3FabricAPI should be 550 per NVIDIA NVML changelog")
}

func TestWithDriverMajor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		driverMajor   int
		expectedMajor int
	}{
		{
			name:          "driver version 535 (old driver)",
			driverMajor:   535,
			expectedMajor: 535,
		},
		{
			name:          "driver version 550 (minimum for V3 API)",
			driverMajor:   550,
			expectedMajor: 550,
		},
		{
			name:          "driver version 560 (newer driver)",
			driverMajor:   560,
			expectedMajor: 560,
		},
		{
			name:          "driver version 0 (uninitialized)",
			driverMajor:   0,
			expectedMajor: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			op := &Op{}
			opt := WithDriverMajor(tt.driverMajor)
			opt(op)

			assert.Equal(t, tt.expectedMajor, op.DriverMajor)
		})
	}
}

func TestOpApplyOpts(t *testing.T) {
	t.Parallel()

	t.Run("multiple options applied correctly", func(t *testing.T) {
		t.Parallel()

		op := &Op{}
		opts := []OpOption{
			WithDriverMajor(550),
			WithGPULost(),
			WithGPURequiresReset(),
			WithFabricHealthUnhealthy(),
		}
		op.applyOpts(opts)

		assert.Equal(t, 550, op.DriverMajor)
		assert.True(t, op.GPULost)
		assert.True(t, op.GPURequiresReset)
		assert.True(t, op.FabricHealthUnhealthy)
	})

	t.Run("driver major can be overwritten", func(t *testing.T) {
		t.Parallel()

		op := &Op{}
		opts := []OpOption{
			WithDriverMajor(535),
			WithDriverMajor(550), // overwrite
		}
		op.applyOpts(opts)

		assert.Equal(t, 550, op.DriverMajor)
	})
}

func TestNvDeviceDriverMajorField(t *testing.T) {
	t.Parallel()

	// Test that the driverMajor field is correctly set on nvDevice
	// We can't directly test nvDevice since it's private, but we can verify
	// the Op struct correctly stores the value

	tests := []struct {
		name        string
		driverMajor int
		expectV3API bool // whether V3 API should be attempted
	}{
		{
			name:        "driver 535 should skip V3 API",
			driverMajor: 535,
			expectV3API: false,
		},
		{
			name:        "driver 549 should skip V3 API",
			driverMajor: 549,
			expectV3API: false,
		},
		{
			name:        "driver 550 should use V3 API",
			driverMajor: 550,
			expectV3API: true,
		},
		{
			name:        "driver 560 should use V3 API",
			driverMajor: 560,
			expectV3API: true,
		},
		{
			name:        "driver 0 (uninitialized) should skip V3 API",
			driverMajor: 0,
			expectV3API: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Verify the logic matches expectations
			shouldUseV3 := tt.driverMajor >= MinDriverVersionForV3FabricAPI
			assert.Equal(t, tt.expectV3API, shouldUseV3,
				"driver major %d: expected V3 API usage = %v", tt.driverMajor, tt.expectV3API)
		})
	}
}

func TestDriverVersionBoundaryConditions(t *testing.T) {
	t.Parallel()

	// Test boundary conditions around the minimum driver version
	tests := []struct {
		driverMajor int
		expectV3    bool
		description string
	}{
		{driverMajor: 548, expectV3: false, description: "two versions below minimum"},
		{driverMajor: 549, expectV3: false, description: "one version below minimum"},
		{driverMajor: 550, expectV3: true, description: "exactly at minimum"},
		{driverMajor: 551, expectV3: true, description: "one version above minimum"},
		{driverMajor: 552, expectV3: true, description: "two versions above minimum"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.description, func(t *testing.T) {
			t.Parallel()

			shouldUseV3 := tt.driverMajor >= MinDriverVersionForV3FabricAPI
			require.Equal(t, tt.expectV3, shouldUseV3,
				"driver %d: V3 API should be %s",
				tt.driverMajor,
				map[bool]string{true: "enabled", false: "disabled"}[tt.expectV3])
		})
	}
}
