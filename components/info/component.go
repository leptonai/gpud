// Package info provides static information about the host (e.g., labels, IDs).
package info

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/file"
	gpud_manager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/memory"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/uptime"
	"github.com/leptonai/gpud/version"
)

const Name = "info"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	annotations map[string]string
	dbRO        *sql.DB
	gatherer    prometheus.Gatherer

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		annotations: gpudInstance.Annotations,
		dbRO:        gpudInstance.DBRO,
		gatherer:    pkgmetrics.DefaultGatherer(),
	}
	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			_ = c.Check()
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getLastHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

var (
	lastSQLiteMetricsMu sync.Mutex
	lastSQLiteMetrics   sqlite.Metrics
)

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking info")

	d := &Data{
		DaemonVersion: version.Version,
		Annotations:   c.annotations,
		ts:            time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	interfaces, err := net.Interfaces()
	if err != nil {
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting interfaces: %v", err)
		return d
	}

	for _, iface := range interfaces {
		macAddress := iface.HardwareAddr.String()
		if macAddress != "" {
			d.MacAddress = macAddress
			break
		}
	}

	if gpud_manager.GlobalController != nil {
		cctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		d.Packages, d.err = gpud_manager.GlobalController.Status(cctx)
		cancel()
		if err != nil {
			d.health = apiv1.StateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting package status: %v", err)
			return d
		}
	}

	d.GPUdPID = os.Getpid()
	d.GPUdUsageFileDescriptors, d.err = file.GetCurrentProcessUsage()
	if err != nil {
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting current process usage: %v", err)
		return d
	}

	d.GPUdUsageMemoryInBytes, d.err = memory.GetCurrentProcessRSSInBytes()
	if err != nil {
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting current process RSS: %v", err)
		return d
	}
	d.GPUdUsageMemoryHumanized = humanize.Bytes(d.GPUdUsageMemoryInBytes)

	if c.dbRO != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
		d.GPUdUsageDBInBytes, d.err = sqlite.ReadDBSize(cctx, c.dbRO)
		ccancel()
		if err != nil {
			d.health = apiv1.StateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting DB size: %v", err)
			return d
		}
		d.GPUdUsageDBHumanized = humanize.Bytes(d.GPUdUsageDBInBytes)
	}

	curMetrics, err := sqlite.ReadMetrics(c.gatherer)
	if err != nil {
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting SQLite metrics: %v", err)
		return d
	}

	d.GPUdUsageInsertUpdateTotal = curMetrics.InsertUpdateTotal
	d.GPUdUsageInsertUpdateAvgLatencyInSeconds = curMetrics.InsertUpdateSecondsAvg

	d.GPUdUsageDeleteTotal = curMetrics.DeleteTotal
	d.GPUdUsageDeleteAvgLatencyInSeconds = curMetrics.DeleteSecondsAvg

	d.GPUdUsageSelectTotal = curMetrics.SelectTotal
	d.GPUdUsageSelectAvgLatencyInSeconds = curMetrics.SelectSecondsAvg

	lastSQLiteMetricsMu.Lock()
	d.GPUdUsageInsertUpdateAvgQPS, d.GPUdUsageDeleteAvgQPS, d.GPUdUsageSelectAvgQPS = lastSQLiteMetrics.QPS(curMetrics)
	lastSQLiteMetrics = curMetrics
	lastSQLiteMetricsMu.Unlock()

	d.GPUdStartTimeInUnixTime, d.err = uptime.GetCurrentProcessStartTimeInUnixTime()
	if d.err != nil {
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting GPUd start time: %v", err)
		return d
	}
	d.GPUdStartTimeHumanized = humanize.Time(time.Unix(int64(d.GPUdStartTimeInUnixTime), 0))

	d.health = apiv1.StateTypeHealthy
	d.reason = fmt.Sprintf("daemon version: %s, mac address: %s", version.Version, d.MacAddress)

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	// Daemon information
	DaemonVersion string                   `json:"daemon_version"`
	MacAddress    string                   `json:"mac_address"`
	Packages      []packages.PackageStatus `json:"packages"`

	// Process information
	GPUdPID                  int    `json:"gpud_pid"`
	GPUdUsageFileDescriptors uint64 `json:"gpud_usage_file_descriptors"`

	// Memory usage
	GPUdUsageMemoryInBytes   uint64 `json:"gpud_usage_memory_in_bytes"`
	GPUdUsageMemoryHumanized string `json:"gpud_usage_memory_humanized"`

	// Database usage
	GPUdUsageDBInBytes   uint64 `json:"gpud_usage_db_in_bytes"`
	GPUdUsageDBHumanized string `json:"gpud_usage_db_humanized"`

	// Database metrics
	GPUdUsageInsertUpdateTotal               int64   `json:"gpud_usage_insert_update_total"`
	GPUdUsageInsertUpdateAvgQPS              float64 `json:"gpud_usage_insert_update_avg_qps"`
	GPUdUsageInsertUpdateAvgLatencyInSeconds float64 `json:"gpud_usage_insert_update_avg_latency_in_seconds"`

	GPUdUsageDeleteTotal               int64   `json:"gpud_usage_delete_total"`
	GPUdUsageDeleteAvgQPS              float64 `json:"gpud_usage_delete_avg_qps"`
	GPUdUsageDeleteAvgLatencyInSeconds float64 `json:"gpud_usage_delete_avg_latency_in_seconds"`

	GPUdUsageSelectTotal               int64   `json:"gpud_usage_select_total"`
	GPUdUsageSelectAvgQPS              float64 `json:"gpud_usage_select_avg_qps"`
	GPUdUsageSelectAvgLatencyInSeconds float64 `json:"gpud_usage_select_avg_latency_in_seconds"`

	// Uptime information
	GPUdStartTimeInUnixTime uint64 `json:"gpud_start_time_in_unix_time"`
	GPUdStartTimeHumanized  string `json:"gpud_start_time_humanized"`

	// Annotations
	Annotations map[string]string `json:"annotations"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (d *Data) String() string {
	if d == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.Append([]string{"Daemon Version", d.DaemonVersion})
	table.Append([]string{"Mac Address", d.MacAddress})

	table.Append([]string{"GPUd Usage: File Descriptors", fmt.Sprintf("%d", d.GPUdUsageFileDescriptors)})
	table.Append([]string{"GPUd Usage: Memory", d.GPUdUsageMemoryHumanized})
	table.Append([]string{"GPUd Usage: DB", d.GPUdUsageDBHumanized})

	table.Append([]string{"GPUd Usage: Insert/Update Total", fmt.Sprintf("%d", d.GPUdUsageInsertUpdateTotal)})
	table.Append([]string{"GPUd Usage: Insert/Update Avg QPS", fmt.Sprintf("%f", d.GPUdUsageInsertUpdateAvgQPS)})
	table.Append([]string{"GPUd Usage: Insert/Update Avg Latency", fmt.Sprintf("%f", d.GPUdUsageInsertUpdateAvgLatencyInSeconds)})

	table.Append([]string{"GPUd Usage: Delete Total", fmt.Sprintf("%d", d.GPUdUsageDeleteTotal)})
	table.Append([]string{"GPUd Usage: Delete Avg QPS", fmt.Sprintf("%f", d.GPUdUsageDeleteAvgQPS)})
	table.Append([]string{"GPUd Usage: Delete Avg Latency", fmt.Sprintf("%f", d.GPUdUsageDeleteAvgLatencyInSeconds)})

	table.Append([]string{"GPUd Usage: Select Total", fmt.Sprintf("%d", d.GPUdUsageSelectTotal)})
	table.Append([]string{"GPUd Usage: Select Avg QPS", fmt.Sprintf("%f", d.GPUdUsageSelectAvgQPS)})
	table.Append([]string{"GPUd Usage: Select Avg Latency", fmt.Sprintf("%f", d.GPUdUsageSelectAvgLatencyInSeconds)})

	table.Append([]string{"GPUd Start Time", d.GPUdStartTimeHumanized})

	table.Render()
	return buf.String()
}

func (d *Data) Summary() string {
	if d == nil {
		return ""
	}
	return d.reason
}

func (d *Data) HealthState() apiv1.HealthStateType {
	if d == nil {
		return ""
	}
	return d.health
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getLastHealthStates() apiv1.HealthStates {
	if d == nil {
		return []apiv1.HealthState{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),
		Health: d.health,
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
