package device

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
)

func TestFabricState_GetIssues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		state    FabricState
		expected []string
	}{
		{
			name: "healthy state",
			state: FabricState{
				CliqueID:      101,
				ClusterUUID:   "cluster-uuid",
				State:         nvml.GPU_FABRIC_STATE_COMPLETED,
				Status:        nvml.SUCCESS,
				HealthMask:    0,
				HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
			},
			expected: []string{},
		},
		{
			name: "state and status issues",
			state: FabricState{
				CliqueID:      101,
				ClusterUUID:   "cluster",
				State:         nvml.GPU_FABRIC_STATE_IN_PROGRESS,
				Status:        nvml.ERROR_UNKNOWN,
				HealthMask:    0,
				HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
			},
			expected: []string{"state=In Progress", "status=ERROR_UNKNOWN"},
		},
		{
			name: "unhealthy with multiple issues",
			state: FabricState{
				CliqueID:    101,
				ClusterUUID: "cluster",
				State:       nvml.GPU_FABRIC_STATE_IN_PROGRESS,
				Status:      nvml.ERROR_UNKNOWN,
				HealthMask: uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW |
					uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_RECOVERY |
					uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY,
				HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY,
			},
			expected: []string{
				"bandwidth degraded",
				"route recovery in progress",
				"route unhealthy",
				"state=In Progress",
				"status=ERROR_UNKNOWN",
				"summary=Unhealthy",
			},
		},
		{
			name: "limited capacity",
			state: FabricState{
				CliqueID:      101,
				ClusterUUID:   "cluster",
				State:         nvml.GPU_FABRIC_STATE_COMPLETED,
				Status:        nvml.SUCCESS,
				HealthMask:    0,
				HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY,
			},
			expected: []string{"summary=Limited Capacity"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			issues := tt.state.GetIssues()
			assert.Equal(t, tt.expected, issues)
		})
	}
}

func TestGetHealthMaskIssues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mask     uint32
		expected []string
	}{
		{
			name:     "no issues",
			mask:     0,
			expected: []string{},
		},
		{
			name:     "bandwidth degraded",
			mask:     uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE) << nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW,
			expected: []string{"bandwidth degraded"},
		},
		{
			name:     "route recovery in progress",
			mask:     uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_TRUE) << nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_RECOVERY,
			expected: []string{"route recovery in progress"},
		},
		{
			name:     "route unhealthy",
			mask:     uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_TRUE) << nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY,
			expected: []string{"route unhealthy"},
		},
		{
			name:     "access timeout recovery",
			mask:     uint32(nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_TRUE) << nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ACCESS_TIMEOUT_RECOVERY,
			expected: []string{"access timeout recovery in progress"},
		},
		{
			name: "multiple issues",
			mask: uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_FALSE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW |
				uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_FALSE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_RECOVERY |
				uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_FALSE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY |
				uint32(nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ACCESS_TIMEOUT_RECOVERY,
			expected: []string{"access timeout recovery in progress"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			issues := getHealthMaskIssues(tt.mask)
			assert.Equal(t, tt.expected, issues)
		})
	}
}

func TestParseHealthMask(t *testing.T) {
	t.Parallel()

	mask := uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_FALSE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_FALSE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_RECOVERY |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_FALSE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ACCESS_TIMEOUT_RECOVERY

	health := ParseHealthMask(mask)
	assert.Equal(t, "Full", health.Bandwidth)
	assert.Equal(t, "False", health.RouteRecoveryProgress)
	assert.Equal(t, "False", health.RouteUnhealthy)
	assert.Equal(t, "True", health.AccessTimeoutRecovery)
}

// TestDriverVersionGatingLogic tests the driver version comparison logic
// that determines whether to use V3 or V1 fabric state API.
// This is a unit test for the gating condition itself.
func TestDriverVersionGatingLogic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		driverMajor int
		expectV3API bool
		description string
	}{
		{
			name:        "driver 535 (old H100 driver)",
			driverMajor: 535,
			expectV3API: false,
			description: "Driver 535.x does not have nvmlDeviceGetGpuFabricInfoV symbol",
		},
		{
			name:        "driver 545 (pre-V3 API)",
			driverMajor: 545,
			expectV3API: false,
			description: "Driver 545.x is before V3 API introduction",
		},
		{
			name:        "driver 549 (boundary - one below minimum)",
			driverMajor: 549,
			expectV3API: false,
			description: "Driver 549 is one version below the minimum required",
		},
		{
			name:        "driver 550 (minimum for V3 API)",
			driverMajor: 550,
			expectV3API: true,
			description: "Driver 550 introduced nvmlDeviceGetGpuFabricInfoV",
		},
		{
			name:        "driver 551 (boundary - one above minimum)",
			driverMajor: 551,
			expectV3API: true,
			description: "Driver 551 should support V3 API",
		},
		{
			name:        "driver 560 (newer driver)",
			driverMajor: 560,
			expectV3API: true,
			description: "Newer drivers should support V3 API",
		},
		{
			name:        "driver 0 (uninitialized)",
			driverMajor: 0,
			expectV3API: false,
			description: "Zero driver version (uninitialized) should skip V3 API",
		},
		{
			name:        "driver -1 (invalid)",
			driverMajor: -1,
			expectV3API: false,
			description: "Negative driver version should skip V3 API",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// This is the same condition used in GetFabricState()
			shouldUseV3 := tt.driverMajor >= MinDriverVersionForV3FabricAPI
			assert.Equal(t, tt.expectV3API, shouldUseV3,
				"%s: driver %d should have V3 API = %v, got %v",
				tt.description, tt.driverMajor, tt.expectV3API, shouldUseV3)
		})
	}
}

// TestFormatClusterUUID tests the UUID formatting function
func TestFormatClusterUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    [16]uint8
		expected string
	}{
		{
			name:     "all zeros returns empty string",
			input:    [16]uint8{},
			expected: "",
		},
		{
			name:     "valid UUID",
			input:    [16]uint8{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			expected: "01020304-0506-0708-090a-0b0c0d0e0f10",
		},
		{
			name:     "UUID with mixed values",
			input:    [16]uint8{0xaa, 0xbb, 0xcc, 0xdd, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb},
			expected: "aabbccdd-0011-2233-4455-66778899aabb",
		},
		{
			name:     "UUID with single non-zero byte",
			input:    [16]uint8{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected: "01000000-0000-0000-0000-000000000000",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := formatClusterUUID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFabricStateToEntry tests the conversion of FabricState to FabricStateEntry
func TestFabricStateToEntry(t *testing.T) {
	t.Parallel()

	t.Run("healthy state with V3 fields", func(t *testing.T) {
		t.Parallel()

		state := FabricState{
			CliqueID:      42,
			ClusterUUID:   "test-cluster-uuid",
			State:         nvml.GPU_FABRIC_STATE_COMPLETED,
			Status:        nvml.SUCCESS,
			HealthMask:    0,
			HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
		}

		entry := state.ToEntry("GPU-TEST-UUID")

		assert.Equal(t, "GPU-TEST-UUID", entry.GPUUUID)
		assert.Equal(t, uint32(42), entry.CliqueID)
		assert.Equal(t, "test-cluster-uuid", entry.ClusterUUID)
		assert.Equal(t, "Completed", entry.State)
		assert.Equal(t, "Success", entry.Status)
		assert.Equal(t, "Healthy", entry.Summary)
	})

	t.Run("V1 fallback state (no health summary)", func(t *testing.T) {
		t.Parallel()

		state := FabricState{
			CliqueID:      777,
			ClusterUUID:   "",
			State:         nvml.GPU_FABRIC_STATE_COMPLETED,
			Status:        nvml.SUCCESS,
			HealthMask:    0,
			HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED,
		}

		entry := state.ToEntry("GPU-V1-UUID")

		assert.Equal(t, "GPU-V1-UUID", entry.GPUUUID)
		assert.Equal(t, uint32(777), entry.CliqueID)
		assert.Equal(t, "", entry.ClusterUUID)
		assert.Equal(t, "Not Supported", entry.Summary)
	})
}
