// Package info provides static information about the host (e.g., labels, IDs).
package info

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/leptonai/gpud/components"
	info_id "github.com/leptonai/gpud/components/info/id"
	"github.com/leptonai/gpud/components/state"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/manager"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/memory"
	"github.com/leptonai/gpud/pkg/uptime"
	"github.com/leptonai/gpud/version"

	"github.com/dustin/go-humanize"
)

func New(annotations map[string]string, dbRO *sql.DB) components.Component {
	return &component{
		annotations: annotations,
		dbRO:        dbRO,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	annotations map[string]string
	dbRO        *sql.DB
}

func (c *component) Name() string { return info_id.Name }

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

	StateKeyGPUdStartTimeInUnixTime = "gpud_start_time_in_unix_time"
	StateKeyGPUdStartTimeHumanized  = "gpud_start_time_humanized"

	StateNameAnnotations = "annotations"
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
	if manager.GlobalController != nil {
		packageStatus, err := manager.GlobalController.Status(ctx)
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
		dbSize, err = state.ReadDBSize(ctx, c.dbRO)
		if err != nil {
			return nil, err
		}
		dbSizeHumanized = humanize.Bytes(dbSize)
	}

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
