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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
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
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()
	return lastCheckResult.HealthStates()
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

	cr := &checkResult{
		DaemonVersion: version.Version,
		Annotations:   c.annotations,
		ts:            time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	interfaces, err := net.Interfaces()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error getting interfaces: %v", err)
		return cr
	}

	for _, iface := range interfaces {
		macAddress := iface.HardwareAddr.String()
		if macAddress != "" {
			cr.MacAddress = macAddress
			break
		}
	}

	if gpud_manager.GlobalController != nil {
		cctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		cr.Packages, cr.err = gpud_manager.GlobalController.Status(cctx)
		cancel()
		if err != nil {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = fmt.Sprintf("error getting package status: %v", err)
			return cr
		}
	}

	cr.GPUdPID = os.Getpid()
	cr.GPUdUsageFileDescriptors, cr.err = file.GetCurrentProcessUsage()
	if err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error getting current process usage: %v", err)
		return cr
	}

	cr.GPUdUsageMemoryInBytes, cr.err = memory.GetCurrentProcessRSSInBytes()
	if err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error getting current process RSS: %v", err)
		return cr
	}
	cr.GPUdUsageMemoryHumanized = humanize.Bytes(cr.GPUdUsageMemoryInBytes)

	if c.dbRO != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
		cr.GPUdUsageDBInBytes, cr.err = sqlite.ReadDBSize(cctx, c.dbRO)
		ccancel()
		if err != nil {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = fmt.Sprintf("error getting DB size: %v", err)
			return cr
		}
		cr.GPUdUsageDBHumanized = humanize.Bytes(cr.GPUdUsageDBInBytes)
	}

	curMetrics, err := sqlite.ReadMetrics(c.gatherer)
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error getting SQLite metrics: %v", err)
		return cr
	}

	cr.GPUdUsageInsertUpdateTotal = curMetrics.InsertUpdateTotal
	cr.GPUdUsageInsertUpdateAvgLatencyInSeconds = curMetrics.InsertUpdateSecondsAvg

	cr.GPUdUsageDeleteTotal = curMetrics.DeleteTotal
	cr.GPUdUsageDeleteAvgLatencyInSeconds = curMetrics.DeleteSecondsAvg

	cr.GPUdUsageSelectTotal = curMetrics.SelectTotal
	cr.GPUdUsageSelectAvgLatencyInSeconds = curMetrics.SelectSecondsAvg

	lastSQLiteMetricsMu.Lock()
	cr.GPUdUsageInsertUpdateAvgQPS, cr.GPUdUsageDeleteAvgQPS, cr.GPUdUsageSelectAvgQPS = lastSQLiteMetrics.QPS(curMetrics)
	lastSQLiteMetrics = curMetrics
	lastSQLiteMetricsMu.Unlock()

	cr.GPUdStartTimeInUnixTime, cr.err = uptime.GetCurrentProcessStartTimeInUnixTime()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error getting GPUd start time: %v", err)
		return cr
	}
	cr.GPUdStartTimeHumanized = humanize.Time(time.Unix(int64(cr.GPUdStartTimeInUnixTime), 0))

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("daemon version: %s, mac address: %s", version.Version, cr.MacAddress)

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.Append([]string{"Daemon Version", cr.DaemonVersion})
	table.Append([]string{"Mac Address", cr.MacAddress})

	table.Append([]string{"GPUd File Descriptors", fmt.Sprintf("%d", cr.GPUdUsageFileDescriptors)})
	table.Append([]string{"GPUd Memory", cr.GPUdUsageMemoryHumanized})
	table.Append([]string{"GPUd DB Size", cr.GPUdUsageDBHumanized})

	table.Append([]string{"DB Insert/Update Total", fmt.Sprintf("%d", cr.GPUdUsageInsertUpdateTotal)})
	table.Append([]string{"DB Insert/Update Avg QPS", fmt.Sprintf("%f", cr.GPUdUsageInsertUpdateAvgQPS)})
	table.Append([]string{"DB Insert/Update Avg Latency", fmt.Sprintf("%f", cr.GPUdUsageInsertUpdateAvgLatencyInSeconds)})

	table.Append([]string{"DB Delete Total", fmt.Sprintf("%d", cr.GPUdUsageDeleteTotal)})
	table.Append([]string{"DB Delete Avg QPS", fmt.Sprintf("%f", cr.GPUdUsageDeleteAvgQPS)})
	table.Append([]string{"DB Delete Avg Latency", fmt.Sprintf("%f", cr.GPUdUsageDeleteAvgLatencyInSeconds)})

	table.Append([]string{"DB Select Total", fmt.Sprintf("%d", cr.GPUdUsageSelectTotal)})
	table.Append([]string{"DB Select Avg QPS", fmt.Sprintf("%f", cr.GPUdUsageSelectAvgQPS)})
	table.Append([]string{"DB Select Avg Latency", fmt.Sprintf("%f", cr.GPUdUsageSelectAvgLatencyInSeconds)})

	table.Append([]string{"GPUd Start Time", cr.GPUdStartTimeHumanized})

	table.Render()
	return buf.String()
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
