package device

import (
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/olekukonko/tablewriter"

	"github.com/leptonai/gpud/pkg/log"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
)

var (
	// logV3SkipOnce ensures we only log the V3 API skip warning once
	logV3SkipOnce sync.Once
)

// FabricState represents fabric state information for a GPU device.
// This struct encapsulates all fabric-related data from V3 and V1 APIs.
type FabricState struct {
	CliqueID    uint32
	ClusterUUID string
	State       uint8
	Status      nvml.Return
	HealthMask  uint32

	// HealthSummary is the health summary of the fabric.
	// This is a uint8 value that can be converted to a string using FabricSummaryToString.
	// NOTE: "NVIDIA H100 80GB HBM3" returns nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED.
	HealthSummary uint8
}

// FabricStateEntry represents a displayable fabric state entry with formatted fields.
// This is used for rendering fabric state information in human-readable format.
type FabricStateEntry struct {
	GPUUUID     string `json:"gpu_uuid"`
	CliqueID    uint32 `json:"clique_id"`
	ClusterUUID string `json:"cluster_uuid,omitempty"`
	State       string `json:"state"`
	Status      string `json:"status"`

	// Summary is converted from HealthSummary using FabricSummaryToString.
	// NOTE: "NVIDIA H100 80GB HBM3" returns nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED.
	Summary string `json:"summary,omitempty"`

	Health FabricHealthSnapshot `json:"health"`
}

// FabricHealthSnapshot represents the health status of fabric components.
type FabricHealthSnapshot struct {
	Bandwidth             string `json:"bandwidth,omitempty"`
	RouteRecoveryProgress string `json:"route_recovery_in_progress,omitempty"`
	RouteUnhealthy        string `json:"route_unhealthy,omitempty"`
	AccessTimeoutRecovery string `json:"access_timeout_recovery,omitempty"`

	// IncorrectConfiguration, PartitionAssigned and GFMState decode the remaining
	// nvmlGpuFabricInfo_v3 healthMask fields defined in nvml.h (shifts 8, 12, 14).
	// They are what distinguishes a misconfigured or orphaned NVL72 tray from a
	// genuine NVLink fault, so they are exposed rather than collapsed into Summary.
	IncorrectConfiguration string `json:"incorrect_configuration,omitempty"`
	PartitionAssigned      string `json:"partition_assigned,omitempty"`
	GFMState               string `json:"gfm_state,omitempty"`
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
	if e.Health.IncorrectConfiguration != "" {
		table.Append([]string{"Incorrect Configuration", e.Health.IncorrectConfiguration})
	}
	if e.Health.PartitionAssigned != "" {
		table.Append([]string{"Partition Assigned", e.Health.PartitionAssigned})
	}
	if e.Health.GFMState != "" {
		table.Append([]string{"Global Fabric Manager", e.Health.GFMState})
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

	// NVML defines status only after the registration probe completes.
	if fs.State == nvml.GPU_FABRIC_STATE_COMPLETED && fs.Status != nvml.SUCCESS {
		issues = append(issues, fmt.Sprintf("status=%s", entry.Status))
	}
	issues = append(issues, fs.GetHealthIssues()...)
	sort.Strings(issues)
	return issues
}

// RegistrationPending reports the two NVML registration states that may recover
// normally. Unsupported and unknown states are not evidence of initialization.
func (fs FabricState) RegistrationPending() bool {
	return fs.State == nvml.GPU_FABRIC_STATE_NOT_STARTED || fs.State == nvml.GPU_FABRIC_STATE_IN_PROGRESS
}

// GetHealthIssues reports fabric faults independently of registration progress.
func (fs FabricState) GetHealthIssues() []string {
	issues := make([]string, 0)

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

	// Check fabric configuration. NOT_SUPPORTED means the driver/GPU does not report
	// the field and NONE means the configuration is correct; every other value
	// (including codes newer than this nvml.h, decoded as Unknown(N)) is a concrete
	// misconfiguration the fabric reported, so it is surfaced with its reason.
	incorrectConfigVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_INCORRECT_CONFIGURATION, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_INCORRECT_CONFIGURATION)
	if incorrectConfigVal != nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_NOT_SUPPORTED &&
		incorrectConfigVal != nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_NONE {
		issues = append(issues, fmt.Sprintf("incorrect configuration=%s", fabricIncorrectConfigurationToString(incorrectConfigVal)))
	}

	// PARTITION_ASSIGNED=FALSE alone is intentionally not an issue: a GPU without a
	// workload partition is the normal idle state on deployments that assign NVLink
	// partitions per job. When the missing partition is actually a fault, NVML
	// reports it separately as INCORRECT_CONFIGURATION_NO_PARTITION (handled above).

	// Check Global Fabric Manager connectivity. On NVL72 racks GFM runs in
	// NMX-Controller on the NVOS switch trays, not on the compute host
	// (https://docs.nvidia.com/dgx/dgxvr-user-guide/software.html), so a
	// disconnected GFM points the operator at the switch trays rather than this node.
	gfmStateVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_GFM_STATE, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_GFM_STATE)
	if gfmStateVal == nvml.GPU_FABRIC_HEALTH_MASK_GFM_STATE_DISCONNECTED {
		issues = append(issues, "global fabric manager disconnected")
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

	// Extract incorrect configuration reason
	incorrectConfigVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_INCORRECT_CONFIGURATION, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_INCORRECT_CONFIGURATION)
	health.IncorrectConfiguration = fabricIncorrectConfigurationToString(incorrectConfigVal)

	// Extract partition assignment status
	partitionAssignedVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_PARTITION_ASSIGNED, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_PARTITION_ASSIGNED)
	health.PartitionAssigned = fabricTriStateStatus(partitionAssignedVal,
		nvml.GPU_FABRIC_HEALTH_MASK_PARTITION_ASSIGNED_NOT_SUPPORTED,
		nvml.GPU_FABRIC_HEALTH_MASK_PARTITION_ASSIGNED_TRUE,
		nvml.GPU_FABRIC_HEALTH_MASK_PARTITION_ASSIGNED_FALSE)

	// Extract Global Fabric Manager connection state
	gfmStateVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_GFM_STATE, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_GFM_STATE)
	health.GFMState = fabricGFMStateToString(gfmStateVal)

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

// fabricIncorrectConfigurationToString maps the nvml.h
// NVML_GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_* codes to readable reasons.
func fabricIncorrectConfigurationToString(val uint32) string {
	switch val {
	case nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_NOT_SUPPORTED:
		return "Not Supported"
	case nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_NONE:
		return "None"
	case nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_INCORRECT_SYSGUID:
		return "Incorrect System GUID"
	case nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_INCORRECT_CHASSIS_SN:
		return "Incorrect Chassis Serial Number"
	case nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_NO_PARTITION:
		return "No Partition"
	case nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_INSUFFICIENT_NVLINKS:
		return "Insufficient NVLinks"
	case nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_INCOMPATIBLE_GPU_FW:
		return "Incompatible GPU Firmware"
	case nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_INVALID_LOCATION:
		return "Invalid Location"
	case nvml.GPU_FABRIC_HEALTH_MASK_INCORRECT_CONFIGURATION_GPU_STATE_INVALID:
		return "GPU State Invalid"
	default:
		return fmt.Sprintf("Unknown(%d)", val)
	}
}

// fabricGFMStateToString maps the nvml.h NVML_GPU_FABRIC_HEALTH_MASK_GFM_STATE_*
// codes to readable Global Fabric Manager connection states.
func fabricGFMStateToString(val uint32) string {
	switch val {
	case nvml.GPU_FABRIC_HEALTH_MASK_GFM_STATE_NOT_SUPPORTED:
		return "Not Supported"
	case nvml.GPU_FABRIC_HEALTH_MASK_GFM_STATE_CONNECTED:
		return "Connected"
	case nvml.GPU_FABRIC_HEALTH_MASK_GFM_STATE_DISCONNECTED:
		return "Disconnected"
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
//
// IMPORTANT: The V3 API (nvmlDeviceGetGpuFabricInfoV) was introduced in driver 550.
// Calling it on older drivers (e.g., 535.x) causes a symbol lookup error that crashes
// the process. We skip V3 API calls for drivers < MinDriverVersionForV3FabricAPI.
// See: https://docs.nvidia.com/deploy/nvml-api/change-log.html
func (d *nvDevice) GetFabricState() (FabricState, error) {
	// Only try V3 API if driver supports it (>= 550).
	// Calling GetGpuFabricInfoV() on older drivers (e.g., 535.x) causes:
	// "symbol lookup error: undefined symbol: nvmlDeviceGetGpuFabricInfoV"
	// which crashes the process (exit code 127) at the dynamic linker level.
	if d.driverMajor >= MinDriverVersionForV3FabricAPI {
		handler := d.GetGpuFabricInfoV()
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
	} else {
		// Log warning once when skipping V3 API due to old driver
		logV3SkipOnce.Do(func() {
			log.Logger.Warnw("skipping fabric state V3 API (nvmlDeviceGetGpuFabricInfoV) due to old driver version; V3 API requires driver >= 550",
				"driverMajor", d.driverMajor,
				"minRequired", MinDriverVersionForV3FabricAPI,
			)
		})
	}

	// Try V1 API (basic fabric information)
	infoV1, ret := d.GetGpuFabricInfo()

	// Check for GPU errors first
	if nvmlerrors.IsGPULostError(ret) {
		return FabricState{}, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return FabricState{}, nvmlerrors.ErrGPURequiresReset
	}
	if nvmlerrors.IsNotSupportError(ret) {
		return FabricState{}, fmt.Errorf("fabric state telemetry not supported: %w", nvml.ERROR_NOT_SUPPORTED)
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
