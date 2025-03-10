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

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	info_id "github.com/leptonai/gpud/components/info/id"
	"github.com/leptonai/gpud/pkg/file"
	gpud_manager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/memory"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/uptime"
	"github.com/leptonai/gpud/version"

	"github.com/dustin/go-humanize"
)

func New(annotations map[string]string, dbRO *sql.DB, gatherer prometheus.Gatherer) components.Component {
	return &component{
		annotations: annotations,
		dbRO:        dbRO,
		gatherer:    gatherer,
	}
}

var _ components.Component = &component{}

type component struct {
	annotations map[string]string
	dbRO        *sql.DB
	gatherer    prometheus.Gatherer
}

func (c *component) Name() string { return info_id.Name }

func (c *component) Start() error { return nil }

const (
	StateNameDaemon = "daemon"

	StateKeyDaemonVersion = "daemon_version"
	StateKeyMacAddress    = "mac_address"
	StateKeyPackages      = "packages"

	StateKeyGPUdPID = "gpud_pid"

	StateKeyGPUdUsageFileDescriptors = "gpud_usage_file_descriptors"

	StateKeyGPUdUsageMemoryInBytes   = "gpud_usage_memory_in_bytes"
	StateKeyGPUdUsageMemoryHumanized = "gpud_usage_memory_humanized"

	StateKeyGPUdUsageDBInBytes   = "gpud_usage_db_in_bytes"
	StateKeyGPUdUsageDBHumanized = "gpud_usage_db_humanized"

	StateKeyGPUdUsageInsertUpdateTotal               = "gpud_usage_insert_update_total"
	StateKeyGPUdUsageInsertUpdateAvgQPS              = "gpud_usage_insert_update_avg_qps"
	StateKeyGPUdUsageInsertUpdateAvgLatencyInSeconds = "gpud_usage_insert_update_avg_latency_in_seconds"

	StateKeyGPUdUsageDeleteTotal               = "gpud_usage_delete_total"
	StateKeyGPUdUsageDeleteAvgQPS              = "gpud_usage_delete_avg_qps"
	StateKeyGPUdUsageDeleteAvgLatencyInSeconds = "gpud_usage_delete_avg_latency_in_seconds"

	StateKeyGPUdUsageSelectTotal               = "gpud_usage_select_total"
	StateKeyGPUdUsageSelectAvgQPS              = "gpud_usage_select_avg_qps"
	StateKeyGPUdUsageSelectAvgLatencyInSeconds = "gpud_usage_select_avg_latency_in_seconds"

	StateKeyGPUdStartTimeInUnixTime = "gpud_start_time_in_unix_time"
	StateKeyGPUdStartTimeHumanized  = "gpud_start_time_humanized"

	StateNameAnnotations = "annotations"
)

var (
	lastSQLiteMetricsMu sync.Mutex
	lastSQLiteMetrics   sqlite.Metrics
)

func (c *component) States(ctx context.Context) ([]components.State, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	mac := ""
	for _, iface := range interfaces {
		macAddress := iface.HardwareAddr.String()
		if macAddress != "" {
			mac = macAddress
			break
		}
	}

	var managedPackages string
	if gpud_manager.GlobalController != nil {
		packageStatus, err := gpud_manager.GlobalController.Status(ctx)
		if err != nil {
			return nil, err
		}
		rawPayload, _ := json.Marshal(&packageStatus)
		managedPackages = string(rawPayload)
	}

	pid := os.Getpid()
	gpudUsageFileDescriptors, err := file.GetCurrentProcessUsage()
	if err != nil {
		return nil, err
	}

	gpudUsageMemoryInBytes, err := memory.GetCurrentProcessRSSInBytes()
	if err != nil {
		return nil, err
	}
	gpudUsageMemoryHumanized := humanize.Bytes(gpudUsageMemoryInBytes)

	var (
		dbSize          uint64
		dbSizeHumanized string
	)
	if c.dbRO != nil {
		dbSize, err = sqlite.ReadDBSize(ctx, c.dbRO)
		if err != nil {
			return nil, err
		}
		dbSizeHumanized = humanize.Bytes(dbSize)
	}

	curMetrics, err := sqlite.ReadMetrics(c.gatherer)
	if err != nil {
		return nil, err
	}

	lastSQLiteMetricsMu.Lock()
	insertUpdateAvgQPS, deleteAvgQPS, selectAvgQPS := lastSQLiteMetrics.QPS(curMetrics)
	lastSQLiteMetrics = curMetrics
	lastSQLiteMetricsMu.Unlock()

	gpudStartTimeInUnixTime, err := uptime.GetCurrentProcessStartTimeInUnixTime()
	if err != nil {
		return nil, err
	}
	gpudStartTimeHumanized := humanize.Time(time.Unix(int64(gpudStartTimeInUnixTime), 0))

	return []components.State{
		{
			Name:    StateNameDaemon,
			Healthy: true,
			Reason:  fmt.Sprintf("daemon version: %s, mac address: %s", version.Version, mac),
			ExtraInfo: map[string]string{
				StateKeyDaemonVersion: version.Version,
				StateKeyMacAddress:    mac,
				StateKeyPackages:      managedPackages,

				StateKeyGPUdPID: fmt.Sprintf("%d", pid),

				StateKeyGPUdUsageFileDescriptors: fmt.Sprintf("%d", gpudUsageFileDescriptors),

				StateKeyGPUdUsageMemoryInBytes:   fmt.Sprintf("%d", gpudUsageMemoryInBytes),
				StateKeyGPUdUsageMemoryHumanized: gpudUsageMemoryHumanized,

				StateKeyGPUdUsageDBInBytes:   fmt.Sprintf("%d", dbSize),
				StateKeyGPUdUsageDBHumanized: dbSizeHumanized,

				StateKeyGPUdUsageInsertUpdateTotal:               fmt.Sprintf("%d", curMetrics.InsertUpdateTotal),
				StateKeyGPUdUsageInsertUpdateAvgQPS:              fmt.Sprintf("%.3f", insertUpdateAvgQPS),
				StateKeyGPUdUsageInsertUpdateAvgLatencyInSeconds: fmt.Sprintf("%.7f", curMetrics.InsertUpdateSecondsAvg),

				StateKeyGPUdUsageDeleteTotal:               fmt.Sprintf("%d", curMetrics.DeleteTotal),
				StateKeyGPUdUsageDeleteAvgQPS:              fmt.Sprintf("%.3f", deleteAvgQPS),
				StateKeyGPUdUsageDeleteAvgLatencyInSeconds: fmt.Sprintf("%.7f", curMetrics.DeleteSecondsAvg),

				StateKeyGPUdUsageSelectTotal:               fmt.Sprintf("%d", curMetrics.SelectTotal),
				StateKeyGPUdUsageSelectAvgQPS:              fmt.Sprintf("%.3f", selectAvgQPS),
				StateKeyGPUdUsageSelectAvgLatencyInSeconds: fmt.Sprintf("%.7f", curMetrics.SelectSecondsAvg),

				StateKeyGPUdStartTimeInUnixTime: fmt.Sprintf("%d", gpudStartTimeInUnixTime),
				StateKeyGPUdStartTimeHumanized:  gpudStartTimeHumanized,
			},
		},
		{
			Name:      StateNameAnnotations,
			Healthy:   true,
			Reason:    fmt.Sprintf("annotations: %v", c.annotations),
			ExtraInfo: c.annotations,
		},
	}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	return nil
}

var _ components.SettableComponent = (*component)(nil)

func (c *component) SetStates(ctx context.Context, states ...components.State) error {
	for _, s := range states {
		if s.Name == "annotations" {
			for k, v := range s.ExtraInfo {
				c.annotations[k] = v
			}
		}
	}
	return nil
}

func (c *component) SetEvents(ctx context.Context, events ...components.Event) error {
	panic("not implemented")
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}
