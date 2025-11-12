package device

import (
	"encoding/hex"
	"fmt"
	"io"
	"sort"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/olekukonko/tablewriter"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia-query/nvml/errors"
)

// FabricState represents fabric state information for a GPU device.
// This struct encapsulates all fabric-related data from V3 and V1 APIs.
type FabricState struct {
	CliqueID      uint32
	ClusterUUID   string
	State         uint8
	Status        nvml.Return
	HealthMask    uint32
	HealthSummary uint8
}

// FabricStateEntry represents a displayable fabric state entry with formatted fields.
// This is used for rendering fabric state information in human-readable format.
type FabricStateEntry struct {
	GPUUUID     string               `json:"gpu_uuid"`
	CliqueID    uint32               `json:"clique_id"`
	ClusterUUID string               `json:"cluster_uuid,omitempty"`
	State       string               `json:"state"`
	Status      string               `json:"status"`
	Summary     string               `json:"summary,omitempty"`
	Health      FabricHealthSnapshot `json:"health"`
}

// FabricHealthSnapshot represents the health status of fabric components.
type FabricHealthSnapshot struct {
	Bandwidth             string `json:"bandwidth,omitempty"`
	RouteRecoveryProgress string `json:"route_recovery_in_progress,omitempty"`
	RouteUnhealthy        string `json:"route_unhealthy,omitempty"`
	AccessTimeoutRecovery string `json:"access_timeout_recovery,omitempty"`
}

// RenderTable renders a single fabric state entry as a formatted table
func (e FabricStateEntry) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetBorder(true)

	table.Append([]string{"GPU UUID", e.GPUUUID})
	table.Append([]string{"Clique ID", fmt.Sprintf("%d", e.CliqueID)})
	if e.ClusterUUID != "" {
		table.Append([]string{"Cluster UUID", e.ClusterUUID})
	}
	table.Append([]string{"State", e.State})
	table.Append([]string{"Status", e.Status})
	if e.Summary != "" {
		table.Append([]string{"Health Summary", e.Summary})
	}

	// Render health snapshot details
	if e.Health.Bandwidth != "" {
		table.Append([]string{"Bandwidth", e.Health.Bandwidth})
	}
	if e.Health.RouteRecoveryProgress != "" {
		table.Append([]string{"Route Recovery", e.Health.RouteRecoveryProgress})
	}
	if e.Health.RouteUnhealthy != "" {
		table.Append([]string{"Route Unhealthy", e.Health.RouteUnhealthy})
	}
	if e.Health.AccessTimeoutRecovery != "" {
		table.Append([]string{"Access Timeout Recovery", e.Health.AccessTimeoutRecovery})
	}

	table.Render()
}

// ToEntry converts a FabricState to a displayable FabricStateEntry with formatted fields.
// The gpuUUID parameter is used to identify the GPU in the display.
func (fs FabricState) ToEntry(gpuUUID string) FabricStateEntry {
	entry := FabricStateEntry{
		GPUUUID:     gpuUUID,
		CliqueID:    fs.CliqueID,
		ClusterUUID: fs.ClusterUUID,
		State:       FabricStateToString(fs.State),
		Status:      FabricStatusToString(fs.Status),
		Summary:     FabricSummaryToString(fs.HealthSummary),
	}

	// Parse health mask into health snapshot
	entry.Health = ParseHealthMask(fs.HealthMask)

	return entry
}

// GetIssues analyzes the fabric state and returns a sorted list of issues found.
// This is useful for health reporting and diagnostics.
func (fs FabricState) GetIssues() []string {
	issues := make([]string, 0)

	// Convert to entry to get string representations
	entry := fs.ToEntry("")

	// Check state
	if fs.State != nvml.GPU_FABRIC_STATE_COMPLETED {
		issues = append(issues, fmt.Sprintf("state=%s", entry.State))
	}

	// Check status
	if fs.Status != nvml.SUCCESS {
		issues = append(issues, fmt.Sprintf("status=%s", entry.Status))
	}

	// Check health summary
	switch fs.HealthSummary {
	case nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY:
		issues = append(issues, "summary=Unhealthy")
	case nvml.GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY:
		issues = append(issues, "summary=Limited Capacity")
	}

	// Add health mask issues
	healthIssues := getHealthMaskIssues(fs.HealthMask)
	issues = append(issues, healthIssues...)

	sort.Strings(issues)
	return issues
}

// getHealthMaskIssues extracts health issues from a health mask.
// Returns a list of human-readable issue descriptions.
func getHealthMaskIssues(mask uint32) []string {
	issues := make([]string, 0)

	// Check bandwidth
	bandwidthVal := (mask >> nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW) & nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_DEGRADED_BW
	if bandwidthVal == nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE {
		issues = append(issues, "bandwidth degraded")
	}

	// Check route recovery
	routeRecoveryVal := (mask >> nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_RECOVERY) & nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_ROUTE_RECOVERY
	if routeRecoveryVal == nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_TRUE {
		issues = append(issues, "route recovery in progress")
	}

	// Check route unhealthy
	routeUnhealthyVal := (mask >> nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY) & nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_ROUTE_UNHEALTHY
	if routeUnhealthyVal == nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_TRUE {
		issues = append(issues, "route unhealthy")
	}

	// Check access timeout
	accessTimeoutVal := (mask >> nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ACCESS_TIMEOUT_RECOVERY) & nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_ACCESS_TIMEOUT_RECOVERY
	if accessTimeoutVal == nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_TRUE {
		issues = append(issues, "access timeout recovery in progress")
	}

	return issues
}

// FabricStateToString converts a fabric state code to a human-readable string.
func FabricStateToString(state uint8) string {
	switch state {
	case nvml.GPU_FABRIC_STATE_NOT_SUPPORTED:
		return "Not Supported"
	case nvml.GPU_FABRIC_STATE_NOT_STARTED:
		return "Not Started"
	case nvml.GPU_FABRIC_STATE_IN_PROGRESS:
		return "In Progress"
	case nvml.GPU_FABRIC_STATE_COMPLETED:
		return "Completed"
	default:
		return fmt.Sprintf("Unknown(%d)", state)
	}
}

// FabricStatusToString converts an NVML return code to a human-readable string.
func FabricStatusToString(status nvml.Return) string {
	if status == nvml.SUCCESS {
		return "Success"
	}
	return status.Error()
}

// FabricSummaryToString converts a fabric health summary code to a human-readable string.
func FabricSummaryToString(summary uint8) string {
	switch summary {
	case nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED:
		return "Not Supported"
	case nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY:
		return "Healthy"
	case nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY:
		return "Unhealthy"
	case nvml.GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY:
		return "Limited Capacity"
	default:
		return fmt.Sprintf("Unknown(%d)", summary)
	}
}

// ParseHealthMask parses a health mask into a FabricHealthSnapshot.
func ParseHealthMask(mask uint32) FabricHealthSnapshot {
	health := FabricHealthSnapshot{}

	// Extract bandwidth status
	bandwidthVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_DEGRADED_BW)
	health.Bandwidth = fabricBandwidthStatus(bandwidthVal)

	// Extract route recovery status
	routeRecoveryVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_RECOVERY, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_ROUTE_RECOVERY)
	health.RouteRecoveryProgress = fabricTriStateStatus(routeRecoveryVal,
		nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_NOT_SUPPORTED,
		nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_TRUE,
		nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_FALSE)

	// Extract route unhealthy status
	routeUnhealthyVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_ROUTE_UNHEALTHY)
	health.RouteUnhealthy = fabricTriStateStatus(routeUnhealthyVal,
		nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_NOT_SUPPORTED,
		nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_TRUE,
		nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_FALSE)

	// Extract access timeout recovery status
	accessTimeoutVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ACCESS_TIMEOUT_RECOVERY, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_ACCESS_TIMEOUT_RECOVERY)
	health.AccessTimeoutRecovery = fabricTriStateStatus(accessTimeoutVal,
		nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_NOT_SUPPORTED,
		nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_TRUE,
		nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_FALSE)

	return health
}

func extractHealthValue(mask uint32, shift uint32, width uint32) uint32 {
	return (mask >> shift) & width
}

func fabricBandwidthStatus(val uint32) string {
	switch val {
	case nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_NOT_SUPPORTED:
		return "Not Supported"
	case nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE:
		return "Degraded"
	case nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_FALSE:
		return "Full"
	default:
		return fmt.Sprintf("Unknown(%d)", val)
	}
}

func fabricTriStateStatus(val, notSupported, trueValue, falseValue uint32) string {
	switch val {
	case notSupported:
		return "Not Supported"
	case trueValue:
		return "True"
	case falseValue:
		return "False"
	default:
		return fmt.Sprintf("Unknown(%d)", val)
	}
}

// GetFabricState retrieves fabric state information from the device.
// It attempts V3 API first for detailed health metrics, falling back to V1 API if needed.
// This method properly handles GPU lost and reset required errors.
func (d *nvDevice) GetFabricState() (FabricState, error) {
	// Try V3 API first (provides detailed health metrics)
	handler := d.Device.GetGpuFabricInfoV()
	info, ret := handler.V3()
	if ret == nvml.SUCCESS {
		return FabricState{
			CliqueID:      info.CliqueId,
			ClusterUUID:   formatClusterUUID(info.ClusterUuid),
			State:         info.State,
			Status:        nvml.Return(info.Status),
			HealthMask:    info.HealthMask,
			HealthSummary: info.HealthSummary,
		}, nil
	}
	// V3 failed, fall through to V1

	// Try V1 API (basic fabric information)
	infoV1, ret := d.Device.GetGpuFabricInfo()

	// Check for GPU errors first
	if nvmlerrors.IsGPULostError(ret) {
		return FabricState{}, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return FabricState{}, nvmlerrors.ErrGPURequiresReset
	}
	if nvmlerrors.IsNotSupportError(ret) {
		return FabricState{}, fmt.Errorf("fabric state telemetry not supported")
	}
	if ret != nvml.SUCCESS {
		return FabricState{}, fmt.Errorf("nvmlDeviceGetGpuFabricInfo failed: %s", nvml.ErrorString(ret))
	}

	// V1 success
	return FabricState{
		CliqueID:      infoV1.CliqueId,
		ClusterUUID:   formatClusterUUID(infoV1.ClusterUuid),
		State:         infoV1.State,
		Status:        nvml.Return(infoV1.Status),
		HealthMask:    0,                                            // V1 doesn't have health mask
		HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED, // V1 doesn't support health summary
	}, nil
}

// formatClusterUUID converts a raw 16-byte UUID to a formatted string.
// Returns empty string if the UUID is all zeros.
func formatClusterUUID(raw [16]uint8) string {
	if raw == ([16]uint8{}) {
		return ""
	}
	buf := make([]byte, 32)
	hex.Encode(buf, raw[:])
	s := string(buf)
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:])
}
