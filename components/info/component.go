// Package info provides static information about the host (e.g., labels, IDs).
package info

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/file"
	gpud_manager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/memory"
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

func New(annotations map[string]string, dbRO *sql.DB, gatherer prometheus.Gatherer) components.Component {
	ctx, cancel := context.WithCancel(context.Background())
	return &component{
		ctx:         ctx,
		cancel:      cancel,
		annotations: annotations,
		dbRO:        dbRO,
		gatherer:    gatherer,
	}
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			c.CheckOnce()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

var (
	lastSQLiteMetricsMu sync.Mutex
	lastSQLiteMetrics   sqlite.Metrics
)

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking info")
	d := Data{
		DaemonVersion: version.Version,
		Annotations:   c.annotations,
		ts:            time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	interfaces, err := net.Interfaces()
	if err != nil {
		d.err = fmt.Errorf("failed to get interfaces: %v", err)
		return
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
		d.Packages, err = gpud_manager.GlobalController.Status(cctx)
		cancel()
		if err != nil {
			d.err = fmt.Errorf("failed to get package status: %v", err)
			return
		}
	}

	d.GPUdPID = os.Getpid()
	d.GPUdUsageFileDescriptors, err = file.GetCurrentProcessUsage()
	if err != nil {
		d.err = fmt.Errorf("failed to get current process usage: %v", err)
		return
	}

	d.GPUdUsageMemoryInBytes, err = memory.GetCurrentProcessRSSInBytes()
	if err != nil {
		d.err = fmt.Errorf("failed to get current process RSS: %v", err)
		return
	}
	d.GPUdUsageMemoryHumanized = humanize.Bytes(d.GPUdUsageMemoryInBytes)

	if c.dbRO != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
		d.GPUdUsageDBInBytes, err = sqlite.ReadDBSize(cctx, c.dbRO)
		ccancel()
		if err != nil {
			d.err = fmt.Errorf("failed to get DB size: %v", err)
			return
		}
		d.GPUdUsageDBHumanized = humanize.Bytes(d.GPUdUsageDBInBytes)
	}

	curMetrics, err := sqlite.ReadMetrics(c.gatherer)
	if err != nil {
		d.err = fmt.Errorf("failed to get SQLite metrics: %v", err)
		return
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

	d.GPUdStartTimeInUnixTime, err = uptime.GetCurrentProcessStartTimeInUnixTime()
	if err != nil {
		d.err = fmt.Errorf("failed to get GPUd start time: %v", err)
		return
	}
	d.GPUdStartTimeHumanized = humanize.Time(time.Unix(int64(d.GPUdStartTimeInUnixTime), 0))
}

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
}

func (d *Data) getReason() string {
	if d == nil {
		return "no info data"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to get info data -- %s", d.err)
	}
	r := fmt.Sprintf("daemon version: %s, mac address: %s", version.Version, d.MacAddress)
	if len(d.Annotations) > 0 {
		r += fmt.Sprintf(", annotations: %q", d.Annotations)
	}
	return r
}

func (d *Data) getHealth() (string, bool) {
	healthy := d == nil || d.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates() ([]components.State, error) {
	if d == nil {
		return []components.State{
			{
				Name:    Name,
				Health:  components.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := components.State{
		Name:   Name,
		Reason: d.getReason(),
		Error:  d.getError(),
	}
	state.Health, state.Healthy = d.getHealth()

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
