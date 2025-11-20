package fabricmanager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	devwrap "github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmltestutil "github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
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
				Entries: []devwrap.FabricStateEntry{
					{
						GPUUUID:     "GPU-0",
						CliqueID:    4026,
						ClusterUUID: "9c6f5af3-53bf-49b5-a436-b66766c413c3",
						State:       "Completed",
						Status:      "Success",
						Summary:     "Healthy",
						Health: devwrap.FabricHealthSnapshot{
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

	// Ensure the mock advertises fabric state support so we take the fabric-state path.
	require.True(t, comp.nvmlInstance.FabricStateSupported())

	result := comp.Check()
	cr, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.True(t, cr.FabricStateSupported)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA GB200 checked fabric state; NVIDIA GB200 does not support fabric manager", cr.reason)
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
				Entries: []devwrap.FabricStateEntry{{GPUUUID: "GPU-0"}},
				Healthy: false,
				Reason:  reason,
			}
		},
	}

	require.True(t, comp.nvmlInstance.FabricStateSupported())

	result := comp.Check()
	cr, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.False(t, cr.FabricManagerActive)
	assert.True(t, cr.FabricStateSupported)
	// FM unsupported path appends its reason and keeps health healthy
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA GB200 with unhealthy fabric state: "+reason+"; NVIDIA GB200 does not support fabric manager", cr.reason)
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

	require.True(t, comp.nvmlInstance.FabricStateSupported())

	result := comp.Check()
	cr, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.True(t, cr.FabricStateSupported)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA GB200 with unhealthy fabric state: ; NVIDIA GB200 does not support fabric manager", cr.reason)
	assert.Nil(t, cr.err)
}

func TestCheck_FabricStateSupportedUnhealthyWithBothReasonAndError(t *testing.T) {
	t.Parallel()

	reason := "GPU GPU-0: route unhealthy"
	fabricErr := errors.New("NVML query failed")

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          false,
			supportsFabricState: true,
			productName:         "NVIDIA GB200 NVL72",
		},
		collectFabricStateFunc: func() fabricStateReport {
			return fabricStateReport{
				Entries: []devwrap.FabricStateEntry{{GPUUUID: "GPU-0"}},
				Healthy: false,
				Reason:  reason,
				Err:     fabricErr,
			}
		},
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.True(t, cr.FabricStateSupported)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA GB200 NVL72 with unhealthy fabric state: "+reason+"; NVIDIA GB200 NVL72 does not support fabric manager", cr.reason)
	assert.Equal(t, reason, cr.FabricStateReason)
	assert.Nil(t, cr.err)
}

func TestCheck_FabricStateSupportedHealthyWithMultipleGPUs(t *testing.T) {
	t.Parallel()

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		nvmlInstance: &mockNVMLInstance{
			exists:              true,
			supportsFM:          false,
			supportsFabricState: true,
			productName:         "NVIDIA H100",
		},
		collectFabricStateFunc: func() fabricStateReport {
			return fabricStateReport{
				Entries: []devwrap.FabricStateEntry{
					{GPUUUID: "GPU-0", State: "Completed", Status: "Success"},
					{GPUUUID: "GPU-1", State: "Completed", Status: "Success"},
					{GPUUUID: "GPU-2", State: "Completed", Status: "Success"},
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
	assert.Equal(t, "NVIDIA H100 checked fabric state; NVIDIA H100 does not support fabric manager", cr.reason)
	assert.Len(t, cr.FabricStates, 3)
	assert.Equal(t, "", cr.FabricStateReason)
}

func TestCheck_FabricStateSupportedUnhealthyWithEmptyReason(t *testing.T) {
	t.Parallel()

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
				Entries: []devwrap.FabricStateEntry{{GPUUUID: "GPU-0"}},
				Healthy: false,
				Reason:  "", // Empty reason
			}
		},
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	assert.True(t, ok)
	assert.True(t, cr.FabricStateSupported)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA GB200 with unhealthy fabric state: ; NVIDIA GB200 does not support fabric manager", cr.reason)
	assert.Equal(t, "", cr.FabricStateReason)
}

// Unit tests for fabric state functions

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
			result := devwrap.FabricStateToString(tt.state)
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
			result := devwrap.FabricStatusToString(tt.status)
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
			result := devwrap.FabricSummaryToString(tt.summary)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFabricStateEntryRenderTable(t *testing.T) {
	t.Parallel()

	entry := devwrap.FabricStateEntry{
		GPUUUID:     "GPU-123",
		CliqueID:    4026,
		ClusterUUID: "9c6f5af3-53bf-49b5-a436-b66766c413c3",
		State:       "Completed",
		Status:      "Success",
		Summary:     "Healthy",
		Health: devwrap.FabricHealthSnapshot{
			Bandwidth:             "Full",
			RouteRecoveryProgress: "False",
			RouteUnhealthy:        "False",
			AccessTimeoutRecovery: "False",
		},
	}

	// Use RenderTable directly instead of the removed helper function
	var buf bytes.Buffer
	entry.RenderTable(&buf)
	result := buf.String()

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
				Entries: []devwrap.FabricStateEntry{
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
				Entries: []devwrap.FabricStateEntry{
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
				Entries: []devwrap.FabricStateEntry{},
				Healthy: true,
			},
			contains: []string{"No fabric state entries"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Use RenderTable directly instead of the removed helper function
			var buf bytes.Buffer
			tt.report.RenderTable(&buf)
			result := buf.String()
			for _, str := range tt.contains {
				assert.True(t, strings.Contains(result, str), "Expected result to contain '%s' but got:\n%s", str, result)
			}
		})
	}
}

// mockFabricStateDevice implements GetFabricState() for testing
type mockFabricStateDevice struct {
	*nvmltestutil.MockDevice
	state devwrap.FabricState
	err   error
}

func (m *mockFabricStateDevice) GetFabricState() (devwrap.FabricState, error) {
	return m.state, m.err
}

func TestGetFabricInfoV1Success(t *testing.T) {
	t.Parallel()

	device := &mockFabricStateDevice{
		MockDevice: nvmltestutil.NewMockDevice(&nvmlmock.Device{}, "arch", "brand", "cuda", "pci0"),
		state: devwrap.FabricState{
			CliqueID:      1234,
			ClusterUUID:   "9c6f5af3-53bf-49b5-a436-b66766c413c3",
			State:         nvml.GPU_FABRIC_STATE_COMPLETED,
			Status:        nvml.SUCCESS,
			HealthMask:    0,
			HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED,
		},
		err: nil,
	}

	data, err := getFabricInfo(device)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1234), data.CliqueID)
	assert.Equal(t, "9c6f5af3-53bf-49b5-a436-b66766c413c3", data.ClusterUUID)
	assert.Equal(t, uint8(nvml.GPU_FABRIC_STATE_COMPLETED), data.State)
	assert.Equal(t, nvml.SUCCESS, data.Status)
	assert.Equal(t, uint8(nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED), data.HealthSummary)
}

func TestGetFabricInfoV1Errors(t *testing.T) {
	t.Parallel()

	t.Run("not supported", func(t *testing.T) {
		device := &mockFabricStateDevice{
			MockDevice: nvmltestutil.NewMockDevice(&nvmlmock.Device{}, "arch", "brand", "cuda", "pci0"),
			state:      devwrap.FabricState{},
			err:        fmt.Errorf("fabric state telemetry not supported"),
		}
		_, err := getFabricInfo(device)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not supported")
	})

	t.Run("nvml error", func(t *testing.T) {
		device := &mockFabricStateDevice{
			MockDevice: nvmltestutil.NewMockDevice(&nvmlmock.Device{}, "arch", "brand", "cuda", "pci0"),
			state:      devwrap.FabricState{},
			err:        fmt.Errorf("nvmlDeviceGetGpuFabricInfo failed: ERROR_NO_PERMISSION"),
		}
		_, err := getFabricInfo(device)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nvmlDeviceGetGpuFabricInfo failed")
	})
}

// TestFabricInfoData tests that fabric info data is properly represented in devwrap.FabricState
func TestFabricInfoData(t *testing.T) {
	t.Parallel()

	// The conversion is now done internally via Device.GetFabricState()
	// This test validates that the devwrap.FabricState structure is properly populated
	data := devwrap.FabricState{
		CliqueID:      777,
		ClusterUUID:   "aabbccdd-eeff-1011-2233-445566778899",
		State:         nvml.GPU_FABRIC_STATE_IN_PROGRESS,
		Status:        nvml.ERROR_OPERATING_SYSTEM,
		HealthMask:    0x3f,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY,
	}

	assert.Equal(t, uint32(777), data.CliqueID)
	assert.Equal(t, "aabbccdd-eeff-1011-2233-445566778899", data.ClusterUUID)
	assert.Equal(t, uint8(nvml.GPU_FABRIC_STATE_IN_PROGRESS), data.State)
	assert.Equal(t, nvml.ERROR_OPERATING_SYSTEM, data.Status)
	assert.Equal(t, uint32(0x3f), data.HealthMask)
	assert.Equal(t, uint8(nvml.GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY), data.HealthSummary)
}

func TestFormatFabricStateEntryIssues(t *testing.T) {
	t.Parallel()

	mask := uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_RECOVERY |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY

	state := devwrap.FabricState{
		CliqueID:      101,
		ClusterUUID:   "cluster",
		State:         nvml.GPU_FABRIC_STATE_IN_PROGRESS,
		Status:        nvml.ERROR_UNKNOWN,
		HealthMask:    mask,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY,
	}

	entry := state.ToEntry("GPU-issue")
	issues := state.GetIssues()

	assert.Equal(t, "GPU-issue", entry.GPUUUID)

	// The issues are sorted lexicographically by the GetIssues method
	expected := []string{
		"bandwidth degraded",
		"route recovery in progress",
		"route unhealthy",
		"state=In Progress",
		"status=ERROR_UNKNOWN",
		"summary=Unhealthy",
	}
	assert.Equal(t, expected, issues)
}

// TestFabricHealthFromMask moved to device package as TestParseHealthMask and TestGetHealthMaskIssues

// ---
// Tests focusing on collectFabricState sorting behavior
// ---

// issueDevice wraps testutil.MockDevice to attach a desired devwrap.FabricState payload
// that is returned by the test override of getFabricInfoFn.
type issueDevice struct {
	*nvmltestutil.MockDevice
	info devwrap.FabricState
	err  error
}

// fakeNVMLInstanceDevices embeds the existing mockNVMLInstance and overrides Devices().
type fakeNVMLInstanceDevices struct {
	mockNVMLInstance
	devs map[string]devwrap.Device
}

func (f *fakeNVMLInstanceDevices) Devices() map[string]devwrap.Device { return f.devs }

func TestCollectFabricState_SortsEntriesAndReasons(t *testing.T) {
	// Override NVML query with deterministic stub for this test
	orig := getFabricInfoFn
	getFabricInfoFn = func(dev interface{}) (devwrap.FabricState, error) {
		if d, ok := dev.(*issueDevice); ok {
			return d.info, d.err
		}
		return devwrap.FabricState{}, fmt.Errorf("unexpected device type %T", dev)
	}
	t.Cleanup(func() { getFabricInfoFn = orig })

	infoA := devwrap.FabricState{
		CliqueID:      1,
		ClusterUUID:   "",
		State:         nvml.GPU_FABRIC_STATE_COMPLETED,
		Status:        nvml.SUCCESS,
		HealthMask:    0,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY,
	}
	// For V1, healthSummary is Not Supported by default; to exercise summary path we switch to V3 in unit below.
	// Instead, we rely on health mask via fabricHealthFromMask by calling formatFabricStateEntry directly.

	// GPU-B: issues -> bandwidth degraded, route unhealthy, state=In Progress, status=ERROR_UNKNOWN, summary=Unhealthy
	maskB := uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY
	infoB := devwrap.FabricState{
		CliqueID:      2,
		ClusterUUID:   "",
		State:         nvml.GPU_FABRIC_STATE_IN_PROGRESS,
		Status:        nvml.ERROR_UNKNOWN,
		HealthMask:    maskB,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY,
	}

	// We will bypass the lack of V1 health mask/summary by using formatFabricStateEntry for mask-derived checks
	// and keep collectFabricState focused on entry/reason sorting. To give collectFabricState issues for GPU-A and GPU-B,
	// we simulate via V1 fields: GPU-A will be healthy (no issues), GPU-B will be unhealthy.
	// Then verify that entries are sorted by UUID and the single reason (for GPU-B) is deterministic and sorted.

	inst := &fakeNVMLInstanceDevices{
		mockNVMLInstance: mockNVMLInstance{exists: true, productName: "NVIDIA Test", deviceCount: 2},
		devs: map[string]devwrap.Device{
			// Insert out of order to ensure sorting by key (UUID)
			"GPU-B": &issueDevice{MockDevice: nvmltestutil.NewMockDevice(&nvmlmock.Device{GetGpuFabricInfoVFunc: func() nvml.GpuFabricInfoHandler { return nvml.GpuFabricInfoHandler{} }}, "arch", "brand", "cuda", "pci0"), info: infoB},
			"GPU-A": &issueDevice{MockDevice: nvmltestutil.NewMockDevice(&nvmlmock.Device{GetGpuFabricInfoVFunc: func() nvml.GpuFabricInfoHandler { return nvml.GpuFabricInfoHandler{} }}, "arch", "brand", "cuda", "pci1"), info: infoA},
		},
	}

	// Run collection
	report := collectFabricState(inst)

	// Entries should be sorted lexicographically by UUID: GPU-A, GPU-B
	if assert.Len(t, report.Entries, 2) {
		assert.Equal(t, "GPU-A", report.Entries[0].GPUUUID)
		assert.Equal(t, "GPU-B", report.Entries[1].GPUUUID)
	}

	// For GPU-B, compute expected sorted issues using the device package methods
	issuesB := infoB.GetIssues()
	expectedReasonB := fmt.Sprintf("GPU GPU-B: %s", strings.Join(issuesB, ", "))

	// report.Reason should contain only GPU-B's issues (GPU-A is healthy)
	assert.False(t, report.Healthy)
	assert.Equal(t, expectedReasonB, report.Reason)
}

func TestCollectFabricState_SortsReasonsAcrossMultipleGPUs(t *testing.T) {
	// Build three devices; two with issues, inserted out of order
	orig := getFabricInfoFn
	getFabricInfoFn = func(dev interface{}) (devwrap.FabricState, error) {
		if d, ok := dev.(*issueDevice); ok {
			return d.info, d.err
		}
		return devwrap.FabricState{}, fmt.Errorf("unexpected device type %T", dev)
	}
	t.Cleanup(func() { getFabricInfoFn = orig })

	// GPU-C healthy
	infoC := devwrap.FabricState{CliqueID: 3, State: nvml.GPU_FABRIC_STATE_COMPLETED, Status: nvml.SUCCESS}

	// Build reasons deterministically using the device package methods
	maskA := uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_TRUE) << nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_RECOVERY
	stateA := devwrap.FabricState{
		CliqueID:      1,
		State:         nvml.GPU_FABRIC_STATE_COMPLETED,
		Status:        nvml.SUCCESS,
		HealthMask:    maskA,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY,
	}
	issuesA := stateA.GetIssues()
	expectedA := fmt.Sprintf("GPU GPU-A: %s", strings.Join(issuesA, ", "))

	maskB := uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_TRUE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY
	stateB := devwrap.FabricState{
		CliqueID:      2,
		State:         nvml.GPU_FABRIC_STATE_IN_PROGRESS,
		Status:        nvml.ERROR_UNKNOWN,
		HealthMask:    maskB,
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY,
	}
	issuesB := stateB.GetIssues()
	expectedB := fmt.Sprintf("GPU GPU-B: %s", strings.Join(issuesB, ", "))

	// For collectFabricState inputs, V1 info influences only state/status; health masks are not surfaced via V1.
	// We still get sorting validation across reasons by constructing expected strings and ensuring the final
	// concatenation order is lexicographic by GPU ID (A then B).
	infoA := stateA
	infoB := stateB

	inst := &fakeNVMLInstanceDevices{
		mockNVMLInstance: mockNVMLInstance{exists: true, productName: "NVIDIA Test", deviceCount: 3},
		devs: map[string]devwrap.Device{
			// Intentionally unsorted insertion
			"GPU-C": &issueDevice{MockDevice: nvmltestutil.NewMockDevice(&nvmlmock.Device{GetGpuFabricInfoVFunc: func() nvml.GpuFabricInfoHandler { return nvml.GpuFabricInfoHandler{} }}, "arch", "brand", "cuda", "pci0"), info: infoC},
			"GPU-B": &issueDevice{MockDevice: nvmltestutil.NewMockDevice(&nvmlmock.Device{GetGpuFabricInfoVFunc: func() nvml.GpuFabricInfoHandler { return nvml.GpuFabricInfoHandler{} }}, "arch", "brand", "cuda", "pci1"), info: infoB},
			"GPU-A": &issueDevice{MockDevice: nvmltestutil.NewMockDevice(&nvmlmock.Device{GetGpuFabricInfoVFunc: func() nvml.GpuFabricInfoHandler { return nvml.GpuFabricInfoHandler{} }}, "arch", "brand", "cuda", "pci2"), info: infoA},
		},
	}

	report := collectFabricState(inst)

	// Entries sorted by UUID
	if assert.Len(t, report.Entries, 3) {
		assert.Equal(t, "GPU-A", report.Entries[0].GPUUUID)
		assert.Equal(t, "GPU-B", report.Entries[1].GPUUUID)
		assert.Equal(t, "GPU-C", report.Entries[2].GPUUUID)
	}

	// Healthy must be false (GPU-B has non-success/not-completed)
	assert.False(t, report.Healthy)

	// Reasons sorted lexicographically across GPUs -> GPU-A first, then GPU-B
	expectedReason := fmt.Sprintf("%s; %s", expectedA, expectedB)
	assert.Equal(t, expectedReason, report.Reason)
}
