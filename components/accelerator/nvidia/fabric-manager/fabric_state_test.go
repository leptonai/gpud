package fabricmanager

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestCheck_FabricStateSupportedHealthy(t *testing.T) {
	t.Parallel()

	mockInstance := &mockNVMLInstance{
		exists:              true,
		supportsFM:          false,
		supportsFabricState: true,
		productName:         "NVIDIA GB200",
		deviceCount:         2,
	}

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		nvmlInstance: mockInstance,
		collectFabricStateFunc: func() fabricStateReport {
			return fabricStateReport{
				Entries: []fabricStateEntry{
					{
						GPUUUID:     "GPU-0",
						CliqueID:    4026,
						ClusterUUID: "9c6f5af3-53bf-49b5-a436-b66766c413c3",
						State:       "Completed",
						Status:      "Success",
						Summary:     "Healthy",
						Health: fabricHealthSnapshot{
							Bandwidth:             "Full",
							RouteRecoveryProgress: "False",
							RouteUnhealthy:        "False",
							AccessTimeoutRecovery: "False",
						},
					},
				},
				Healthy: true,
			}
		},
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.True(t, cr.FabricStateSupported)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA GB200 checked fabric state", cr.reason)
	assert.Len(t, cr.FabricStates, 1)
	assert.Equal(t, "", cr.FabricStateReason)
	assert.Nil(t, cr.err)
}

func TestCheck_FabricStateSupportedUnhealthy(t *testing.T) {
	t.Parallel()

	reason := "GPU GPU-0: bandwidth degraded"

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          false,
			supportsFabricState: true,
			productName:         "NVIDIA GB200",
		},
		collectFabricStateFunc: func() fabricStateReport {
			return fabricStateReport{
				Entries: []fabricStateEntry{{GPUUUID: "GPU-0"}},
				Healthy: false,
				Reason:  reason,
			}
		},
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.False(t, cr.FabricManagerActive)
	assert.True(t, cr.FabricStateSupported)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA GB200 checked fabric state", cr.reason)
	assert.Equal(t, reason, cr.FabricStateReason)
	assert.Nil(t, cr.err)
}

func TestCheck_FabricStateSupportedError(t *testing.T) {
	t.Parallel()

	fabricErr := errors.New("mock fabric failure")

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          false,
			supportsFabricState: true,
			productName:         "NVIDIA GB200",
		},
		collectFabricStateFunc: func() fabricStateReport {
			return fabricStateReport{
				Healthy: false,
				Err:     fabricErr,
			}
		},
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.True(t, cr.FabricStateSupported)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA GB200 checked fabric state", cr.reason)
	assert.Nil(t, cr.err)
}

// Unit tests for fabric state functions

func TestFormatFabricUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    [16]uint8
		expected string
	}{
		{
			name:     "empty UUID",
			input:    [16]uint8{},
			expected: "",
		},
		{
			name:     "valid UUID",
			input:    [16]uint8{0x9c, 0x6f, 0x5a, 0xf3, 0x53, 0xbf, 0x49, 0xb5, 0xa4, 0x36, 0xb6, 0x67, 0x66, 0xc4, 0x13, 0xc3},
			expected: "9c6f5af3-53bf-49b5-a436-b66766c413c3",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := formatFabricUUID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFabricStateToString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		state    uint8
		expected string
	}{
		{
			name:     "not supported",
			state:    nvml.GPU_FABRIC_STATE_NOT_SUPPORTED,
			expected: "Not Supported",
		},
		{
			name:     "not started",
			state:    nvml.GPU_FABRIC_STATE_NOT_STARTED,
			expected: "Not Started",
		},
		{
			name:     "in progress",
			state:    nvml.GPU_FABRIC_STATE_IN_PROGRESS,
			expected: "In Progress",
		},
		{
			name:     "completed",
			state:    nvml.GPU_FABRIC_STATE_COMPLETED,
			expected: "Completed",
		},
		{
			name:     "unknown state",
			state:    99,
			expected: "Unknown(99)",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := fabricStateToString(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFabricStatusToString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   nvml.Return
		expected string
	}{
		{
			name:     "success",
			status:   nvml.SUCCESS,
			expected: "Success",
		},
		{
			name:     "error not supported",
			status:   nvml.ERROR_NOT_SUPPORTED,
			expected: "ERROR_NOT_SUPPORTED",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := fabricStatusToString(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFabricSummaryToString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		summary  uint8
		expected string
	}{
		{
			name:     "not supported",
			summary:  nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED,
			expected: "Not Supported",
		},
		{
			name:     "healthy",
			summary:  nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
			expected: "Healthy",
		},
		{
			name:     "unhealthy",
			summary:  nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY,
			expected: "Unhealthy",
		},
		{
			name:     "limited capacity",
			summary:  nvml.GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY,
			expected: "Limited Capacity",
		},
		{
			name:     "unknown",
			summary:  99,
			expected: "Unknown(99)",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := fabricSummaryToString(tt.summary)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFabricBandwidthStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		val           uint32
		expectedStr   string
		expectedIssue bool
	}{
		{
			name:          "not supported",
			val:           nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_NOT_SUPPORTED,
			expectedStr:   "Not Supported",
			expectedIssue: false,
		},
		{
			name:          "degraded",
			val:           nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE,
			expectedStr:   "Degraded",
			expectedIssue: true,
		},
		{
			name:          "full bandwidth",
			val:           nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_FALSE,
			expectedStr:   "Full",
			expectedIssue: false,
		},
		{
			name:          "unknown value",
			val:           99,
			expectedStr:   "Unknown(99)",
			expectedIssue: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			str, issue := fabricBandwidthStatus(tt.val)
			assert.Equal(t, tt.expectedStr, str)
			assert.Equal(t, tt.expectedIssue, issue)
		})
	}
}

func TestFabricTriStateStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		val           uint32
		notSupported  uint32
		trueValue     uint32
		falseValue    uint32
		expectedStr   string
		expectedIssue bool
	}{
		{
			name:          "not supported",
			val:           0,
			notSupported:  0,
			trueValue:     1,
			falseValue:    2,
			expectedStr:   "Not Supported",
			expectedIssue: false,
		},
		{
			name:          "true value",
			val:           1,
			notSupported:  0,
			trueValue:     1,
			falseValue:    2,
			expectedStr:   "True",
			expectedIssue: true,
		},
		{
			name:          "false value",
			val:           2,
			notSupported:  0,
			trueValue:     1,
			falseValue:    2,
			expectedStr:   "False",
			expectedIssue: false,
		},
		{
			name:          "unknown value",
			val:           99,
			notSupported:  0,
			trueValue:     1,
			falseValue:    2,
			expectedStr:   "Unknown(99)",
			expectedIssue: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			str, issue := fabricTriStateStatus(tt.val, tt.notSupported, tt.trueValue, tt.falseValue)
			assert.Equal(t, tt.expectedStr, str)
			assert.Equal(t, tt.expectedIssue, issue)
		})
	}
}

func TestExtractHealthValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mask     uint32
		shift    uint32
		width    uint32
		expected uint32
	}{
		{
			name:     "extract low bits",
			mask:     0b1111,
			shift:    0,
			width:    0b1111,
			expected: 0b1111,
		},
		{
			name:     "extract shifted bits",
			mask:     0b11110000,
			shift:    4,
			width:    0b1111,
			expected: 0b1111,
		},
		{
			name:     "extract with masking",
			mask:     0b10101010,
			shift:    1,
			width:    0b11,
			expected: 0b01,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := extractHealthValue(tt.mask, tt.shift, tt.width)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFabricStateEntryRenderTable(t *testing.T) {
	t.Parallel()

	entry := fabricStateEntry{
		GPUUUID:     "GPU-123",
		CliqueID:    4026,
		ClusterUUID: "9c6f5af3-53bf-49b5-a436-b66766c413c3",
		State:       "Completed",
		Status:      "Success",
		Summary:     "Healthy",
		Health: fabricHealthSnapshot{
			Bandwidth:             "Full",
			RouteRecoveryProgress: "False",
			RouteUnhealthy:        "False",
			AccessTimeoutRecovery: "False",
		},
	}

	result := fabricStateEntryToString(entry)

	// Verify the result contains key information
	assert.Contains(t, result, "GPU-123")
	assert.Contains(t, result, "4026")
	assert.Contains(t, result, "9c6f5af3-53bf-49b5-a436-b66766c413c3")
	assert.Contains(t, result, "Completed")
	assert.Contains(t, result, "Success")
	assert.Contains(t, result, "Healthy")
	assert.Contains(t, result, "Full")
}

func TestFabricStateReportRenderTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		report   fabricStateReport
		contains []string
	}{
		{
			name: "healthy report with entries",
			report: fabricStateReport{
				Entries: []fabricStateEntry{
					{
						GPUUUID:  "GPU-0",
						CliqueID: 4026,
						State:    "Completed",
						Status:   "Success",
					},
				},
				Healthy: true,
			},
			contains: []string{"GPU-0", "4026", "Completed", "Success", "HEALTHY"},
		},
		{
			name: "unhealthy report with reason",
			report: fabricStateReport{
				Entries: []fabricStateEntry{
					{
						GPUUUID: "GPU-1",
						State:   "Not Started",
					},
				},
				Healthy: false,
				Reason:  "bandwidth degraded",
			},
			contains: []string{"GPU-1", "Not Started", "UNHEALTHY", "bandwidth degraded"},
		},
		{
			name: "empty report",
			report: fabricStateReport{
				Entries: []fabricStateEntry{},
				Healthy: true,
			},
			contains: []string{"No fabric state entries"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := fabricStateReportToString(tt.report)
			for _, str := range tt.contains {
				assert.True(t, strings.Contains(result, str), "Expected result to contain '%s' but got:\n%s", str, result)
			}
		})
	}
}
