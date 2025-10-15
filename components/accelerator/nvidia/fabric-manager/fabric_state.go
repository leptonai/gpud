package fabricmanager

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/olekukonko/tablewriter"
)

type fabricStateReport struct {
	Entries []fabricStateEntry
	Healthy bool
	Reason  string
	Err     error
}

// RenderTable renders the fabric state report as a formatted table
func (r fabricStateReport) RenderTable(wr io.Writer) {
	if len(r.Entries) == 0 {
		_, _ = wr.Write([]byte("No fabric state entries\n"))
		return
	}

	for i, entry := range r.Entries {
		if i > 0 {
			_, _ = wr.Write([]byte("\n"))
		}
		entry.RenderTable(wr)
	}

	if !r.Healthy && r.Reason != "" {
		_, _ = wr.Write([]byte(fmt.Sprintf("\nOverall Status: UNHEALTHY - %s\n", r.Reason)))
	} else if r.Healthy {
		_, _ = wr.Write([]byte("\nOverall Status: HEALTHY\n"))
	}
}

type fabricStateEntry struct {
	GPUUUID     string               `json:"gpu_uuid"`
	CliqueID    uint32               `json:"clique_id"`
	ClusterUUID string               `json:"cluster_uuid,omitempty"`
	State       string               `json:"state"`
	Status      string               `json:"status"`
	Summary     string               `json:"summary,omitempty"`
	Health      fabricHealthSnapshot `json:"health"`
}

// RenderTable renders a single fabric state entry as a formatted table
func (e fabricStateEntry) RenderTable(wr io.Writer) {
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

type fabricHealthSnapshot struct {
	Bandwidth             string `json:"bandwidth,omitempty"`
	RouteRecoveryProgress string `json:"route_recovery_in_progress,omitempty"`
	RouteUnhealthy        string `json:"route_unhealthy,omitempty"`
	AccessTimeoutRecovery string `json:"access_timeout_recovery,omitempty"`
}

type fabricInfoData struct {
	cliqueID      uint32
	clusterUUID   string
	state         uint8
	status        nvml.Return
	healthMask    uint32
	healthSummary uint8
}

type fabricInfoVGetter interface {
	GetGpuFabricInfoV() nvml.GpuFabricInfoHandler
}

type fabricInfoGetter interface {
	GetGpuFabricInfo() (nvml.GpuFabricInfo, nvml.Return)
}

func getFabricInfo(dev interface{}) (fabricInfoData, error) {
	if dev == nil {
		return fabricInfoData{}, fmt.Errorf("nil nvml device handle")
	}

	if getter, ok := dev.(fabricInfoVGetter); ok {
		info, ret := getter.GetGpuFabricInfoV().V3()
		switch ret {
		case nvml.SUCCESS:
			return fabricInfoDataFromV3(info), nil
		case nvml.ERROR_NOT_SUPPORTED:
			// fall back to v1 path
		default:
			return fabricInfoData{}, fmt.Errorf("nvmlDeviceGetGpuFabricInfoV failed: %s", ret.Error())
		}
	}

	if getter, ok := dev.(fabricInfoGetter); ok {
		info, ret := getter.GetGpuFabricInfo()
		switch ret {
		case nvml.SUCCESS:
			return fabricInfoDataFromV1(info), nil
		case nvml.ERROR_NOT_SUPPORTED:
			return fabricInfoData{}, fmt.Errorf("fabric state telemetry not supported")
		default:
			return fabricInfoData{}, fmt.Errorf("nvmlDeviceGetGpuFabricInfo failed: %s", ret.Error())
		}
	}

	return fabricInfoData{}, fmt.Errorf("fabric state telemetry not available")
}

func fabricInfoDataFromV3(info nvml.GpuFabricInfo_v3) fabricInfoData {
	return fabricInfoData{
		cliqueID:      info.CliqueId,
		clusterUUID:   formatFabricUUID(info.ClusterUuid),
		state:         info.State,
		status:        nvml.Return(info.Status),
		healthMask:    info.HealthMask,
		healthSummary: info.HealthSummary,
	}
}

func fabricInfoDataFromV1(info nvml.GpuFabricInfo) fabricInfoData {
	return fabricInfoData{
		cliqueID:      info.CliqueId,
		clusterUUID:   formatFabricUUID(info.ClusterUuid),
		state:         info.State,
		status:        nvml.Return(info.Status),
		healthMask:    0,
		healthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED,
	}
}

func formatFabricStateEntry(uuid string, info fabricInfoData) (fabricStateEntry, []string) {
	entry := fabricStateEntry{
		GPUUUID:     uuid,
		CliqueID:    info.cliqueID,
		ClusterUUID: info.clusterUUID,
		State:       fabricStateToString(info.state),
		Status:      fabricStatusToString(info.status),
		Summary:     fabricSummaryToString(info.healthSummary),
	}

	health, healthIssues := fabricHealthFromMask(info.healthMask)
	entry.Health = health

	issues := make([]string, 0)
	if info.state != nvml.GPU_FABRIC_STATE_COMPLETED {
		issues = append(issues, fmt.Sprintf("state=%s", entry.State))
	}
	if info.status != nvml.SUCCESS {
		issues = append(issues, fmt.Sprintf("status=%s", entry.Status))
	}
	switch info.healthSummary {
	case nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY:
		issues = append(issues, "summary=Unhealthy")
	case nvml.GPU_FABRIC_HEALTH_SUMMARY_LIMITED_CAPACITY:
		issues = append(issues, "summary=Limited Capacity")
	}
	issues = append(issues, healthIssues...)

	return entry, issues
}

func fabricHealthFromMask(mask uint32) (fabricHealthSnapshot, []string) {
	health := fabricHealthSnapshot{}
	issues := make([]string, 0)

	bandwidthVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_DEGRADED_BW)
	bandwidthStr, bandwidthIssue := fabricBandwidthStatus(bandwidthVal)
	health.Bandwidth = bandwidthStr
	if bandwidthIssue {
		issues = append(issues, "bandwidth degraded")
	}

	routeRecoveryVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_RECOVERY, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_ROUTE_RECOVERY)
	routeRecoveryStr, routeRecoveryIssue := fabricTriStateStatus(routeRecoveryVal, nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_NOT_SUPPORTED, nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_TRUE, nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_FALSE)
	health.RouteRecoveryProgress = routeRecoveryStr
	if routeRecoveryIssue {
		issues = append(issues, "route recovery in progress")
	}

	routeUnhealthyVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_ROUTE_UNHEALTHY)
	routeUnhealthyStr, routeUnhealthyIssue := fabricTriStateStatus(routeUnhealthyVal, nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_NOT_SUPPORTED, nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_TRUE, nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_FALSE)
	health.RouteUnhealthy = routeUnhealthyStr
	if routeUnhealthyIssue {
		issues = append(issues, "route unhealthy")
	}

	accessTimeoutVal := extractHealthValue(mask, nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ACCESS_TIMEOUT_RECOVERY, nvml.GPU_FABRIC_HEALTH_MASK_WIDTH_ACCESS_TIMEOUT_RECOVERY)
	accessTimeoutStr, accessTimeoutIssue := fabricTriStateStatus(accessTimeoutVal, nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_NOT_SUPPORTED, nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_TRUE, nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_FALSE)
	health.AccessTimeoutRecovery = accessTimeoutStr
	if accessTimeoutIssue {
		issues = append(issues, "access timeout recovery in progress")
	}

	return health, issues
}

func fabricBandwidthStatus(val uint32) (string, bool) {
	switch val {
	case nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_NOT_SUPPORTED:
		return "Not Supported", false
	case nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE:
		return "Degraded", true
	case nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_FALSE:
		return "Full", false
	default:
		return fmt.Sprintf("Unknown(%d)", val), true
	}
}

func fabricTriStateStatus(val, notSupported, trueValue, falseValue uint32) (string, bool) {
	switch val {
	case notSupported:
		return "Not Supported", false
	case trueValue:
		return "True", true
	case falseValue:
		return "False", false
	default:
		return fmt.Sprintf("Unknown(%d)", val), true
	}
}

func extractHealthValue(mask uint32, shift uint32, width uint32) uint32 {
	return (mask >> shift) & width
}

func fabricStateToString(state uint8) string {
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

func fabricStatusToString(status nvml.Return) string {
	if status == nvml.SUCCESS {
		return "Success"
	}
	return status.Error()
}

func fabricSummaryToString(summary uint8) string {
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

func formatFabricUUID(raw [16]uint8) string {
	if raw == ([16]uint8{}) {
		return ""
	}
	buf := make([]byte, 32)
	hex.Encode(buf, raw[:])
	s := string(buf)
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:])
}

// fabricStateEntryToString converts a fabric state entry to a formatted string using RenderTable
func fabricStateEntryToString(entry fabricStateEntry) string {
	var buf bytes.Buffer
	entry.RenderTable(&buf)
	return buf.String()
}

// fabricStateReportToString converts a fabric state report to a formatted string using RenderTable
func fabricStateReportToString(report fabricStateReport) string {
	var buf bytes.Buffer
	report.RenderTable(&buf)
	return buf.String()
}
