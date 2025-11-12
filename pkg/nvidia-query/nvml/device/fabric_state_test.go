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
