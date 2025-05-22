package machineinfo

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
)

type GPUdInfo struct {
	// Process information
	PID                  int    `json:"pid"`
	UsageFileDescriptors uint64 `json:"usage_file_descriptors"`

	// Memory usage
	UsageMemoryInBytes   uint64 `json:"usage_memory_in_bytes"`
	UsageMemoryHumanized string `json:"usage_memory_humanized"`

	// Database usage
	UsageDBInBytes   uint64 `json:"usage_db_in_bytes"`
	UsageDBHumanized string `json:"usage_db_humanized"`

	// Database metrics
	UsageInsertUpdateTotal               int64   `json:"usage_insert_update_total"`
	UsageInsertUpdateAvgQPS              float64 `json:"usage_insert_update_avg_qps"`
	UsageInsertUpdateAvgLatencyInSeconds float64 `json:"usage_insert_update_avg_latency_in_seconds"`

	UsageDeleteTotal               int64   `json:"usage_delete_total"`
	UsageDeleteAvgQPS              float64 `json:"usage_delete_avg_qps"`
	UsageDeleteAvgLatencyInSeconds float64 `json:"usage_delete_avg_latency_in_seconds"`

	UsageSelectTotal               int64   `json:"usage_select_total"`
	UsageSelectAvgQPS              float64 `json:"usage_select_avg_qps"`
	UsageSelectAvgLatencyInSeconds float64 `json:"usage_select_avg_latency_in_seconds"`

	// Uptime information
	StartTimeInUnixTime uint64 `json:"start_time_in_unix_time"`
	StartTimeHumanized  string `json:"start_time_humanized"`
}

func (info *GPUdInfo) RenderTable(wr io.Writer) {
	if info == nil {
		return
	}

	table := tablewriter.NewWriter(wr)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"GPUd File Descriptors", fmt.Sprintf("%d", info.UsageFileDescriptors)})
	table.Append([]string{"GPUd Memory", info.UsageMemoryHumanized})
	table.Append([]string{"GPUd DB Size", info.UsageDBHumanized})

	table.Append([]string{"GPUd DB Insert/Update Total", fmt.Sprintf("%d", info.UsageInsertUpdateTotal)})
	table.Append([]string{"GPUd DB Insert/Update Avg QPS", fmt.Sprintf("%f", info.UsageInsertUpdateAvgQPS)})
	table.Append([]string{"GPUd DB Insert/Update Avg Latency", fmt.Sprintf("%f", info.UsageInsertUpdateAvgLatencyInSeconds)})

	table.Append([]string{"GPUd DB Delete Total", fmt.Sprintf("%d", info.UsageDeleteTotal)})
	table.Append([]string{"GPUd DB Delete Avg QPS", fmt.Sprintf("%f", info.UsageDeleteAvgQPS)})
	table.Append([]string{"GPUd DB Delete Avg Latency", fmt.Sprintf("%f", info.UsageDeleteAvgLatencyInSeconds)})

	table.Append([]string{"GPUd DB Select Total", fmt.Sprintf("%d", info.UsageSelectTotal)})
	table.Append([]string{"GPUd DB Select Avg QPS", fmt.Sprintf("%f", info.UsageSelectAvgQPS)})
	table.Append([]string{"GPUd DB Select Avg Latency", fmt.Sprintf("%f", info.UsageSelectAvgLatencyInSeconds)})

	table.Append([]string{"GPUd Start Time", info.StartTimeHumanized})
	table.Render()
}
