// Package os queries the host OS information (e.g., kernel version).
package os

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/shirou/gopsutil/v4/host"
	procs "github.com/shirou/gopsutil/v4/process"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/file"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// Name is the ID of the OS component.
const Name = "os"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	countProcessesByStatusFunc func(ctx context.Context) (map[string][]*procs.Process, error)

	rebootEventStore pkghost.RebootEventStore

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, rebootEventStore pkghost.RebootEventStore) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		countProcessesByStatusFunc: process.CountProcessesByStatus,

		rebootEventStore: rebootEventStore,
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

func (c *component) HealthStates(ctx context.Context) (apiv1.HealthStates, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if c.rebootEventStore != nil {
		return c.rebootEventStore.GetRebootEvents(ctx, since)
	}
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking os")

	d := checkHealthState(c.ctx, c.countProcessesByStatusFunc)

	c.lastMu.Lock()
	c.lastData = d
	c.lastMu.Unlock()

	if err := c.rebootEventStore.RecordReboot(c.ctx); err != nil {
		log.Logger.Warnw("error creating reboot event", "error", err)
	}
}

func checkHealthState(ctx context.Context, countProcessesByStatusFunc func(ctx context.Context) (map[string][]*procs.Process, error)) *Data {
	d := &Data{
		ts: time.Now().UTC(),

		VirtualizationEnvironment: pkghost.VirtualizationEnv(),
		SystemManufacturer:        pkghost.SystemManufacturer(),
		MachineMetadata: MachineMetadata{
			BootID:        pkghost.BootID(),
			DmidecodeUUID: pkghost.DmidecodeUUID(),
			OSMachineID:   pkghost.OSMachineID(),
		},
		Host:     Host{ID: pkghost.HostID()},
		Kernel:   Kernel{Arch: pkghost.Arch(), Version: pkghost.KernelVersion()},
		Platform: Platform{Name: pkghost.Platform(), Family: pkghost.PlatformFamily(), Version: pkghost.PlatformVersion()},
	}

	cctx, ccancel := context.WithTimeout(ctx, 15*time.Second)
	uptime, err := host.UptimeWithContext(cctx)
	ccancel()
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error getting uptime: %s", err)
		return d
	}

	d.Uptimes = Uptimes{
		Seconds:             uptime,
		BootTimeUnixSeconds: pkghost.BootTimeUnixSeconds(),
	}

	allProcs, err := countProcessesByStatusFunc(ctx)
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error getting process count: %s", err)
		return d
	}

	for status, procsWithStatus := range allProcs {
		if status == procs.Zombie {
			d.ProcessCountZombieProcesses = len(procsWithStatus)
			break
		}
	}
	if d.ProcessCountZombieProcesses > zombieProcessCountThreshold {
		d.healthy = false
		d.reason = fmt.Sprintf("too many zombie processes: %d (threshold: %d)", d.ProcessCountZombieProcesses, zombieProcessCountThreshold)
		return d
	}

	d.healthy = true
	d.reason = fmt.Sprintf("os kernel version %s", d.Kernel.Version)
	return d
}

func CheckHealthState(ctx context.Context) (components.HealthStateCheckResult, error) {
	d := checkHealthState(ctx, process.CountProcessesByStatus)
	if d.err != nil {
		return nil, d.err
	}
	return d, nil
}

var _ components.HealthStateCheckResult = &Data{}

type Data struct {
	VirtualizationEnvironment   pkghost.VirtualizationEnvironment `json:"virtualization_environment"`
	SystemManufacturer          string                            `json:"system_manufacturer"`
	MachineMetadata             MachineMetadata                   `json:"machine_metadata"`
	Host                        Host                              `json:"host"`
	Kernel                      Kernel                            `json:"kernel"`
	Platform                    Platform                          `json:"platform"`
	Uptimes                     Uptimes                           `json:"uptimes"`
	ProcessCountZombieProcesses int                               `json:"process_count_zombie_processes"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	healthy bool
	// tracks the reason of the last check
	reason string
}

type MachineMetadata struct {
	BootID        string `json:"boot_id"`
	DmidecodeUUID string `json:"dmidecode_uuid"`
	OSMachineID   string `json:"os_machine_id"`
}

type Host struct {
	ID string `json:"id"`
}

type Kernel struct {
	Arch    string `json:"arch"`
	Version string `json:"version"`
}

type Platform struct {
	Name    string `json:"name"`
	Family  string `json:"family"`
	Version string `json:"version"`
}

type Uptimes struct {
	Seconds             uint64 `json:"seconds"`
	BootTimeUnixSeconds uint64 `json:"boot_time_unix_seconds"`
}

func (d *Data) String() string {
	if d == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"VM Type", d.VirtualizationEnvironment.Type})
	table.Append([]string{"Kernel Arch", d.Kernel.Arch})
	table.Append([]string{"Kernel Version", d.Kernel.Version})
	table.Append([]string{"Platform Name", d.Platform.Name})
	table.Append([]string{"Platform Version", d.Platform.Version})

	boottimeTS := time.Unix(int64(d.Uptimes.BootTimeUnixSeconds), 0)
	nowUTC := time.Now().UTC()
	uptimeHumanized := humanize.RelTime(boottimeTS, nowUTC, "ago", "from now")
	table.Append([]string{"Uptime", uptimeHumanized})

	table.Append([]string{"Zombie Process Count", fmt.Sprintf("%d", d.ProcessCountZombieProcesses)})
	table.Render()

	return buf.String()
}

func (d *Data) Summary() string {
	return d.reason
}

func (d *Data) HealthState() apiv1.HealthStateType {
	if d == nil {
		return ""
	}
	if d.healthy {
		return apiv1.StateTypeHealthy
	}
	return apiv1.StateTypeUnhealthy
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getHealthStates() (apiv1.HealthStates, error) {
	if d == nil {
		return []apiv1.HealthState{
			{
				Name:              Name,
				Health:            apiv1.StateTypeHealthy,
				DeprecatedHealthy: true,
				Reason:            "no data yet",
			},
		}, nil
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		DeprecatedHealthy: d.healthy,
		Health:            d.HealthState(),
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.HealthState{state}, nil
}

var zombieProcessCountThreshold = 1000

func init() {
	// Linux-specific operations
	if runtime.GOOS != "linux" {
		return
	}

	// File descriptor limit check is Linux-specific
	if file.CheckFDLimitSupported() {
		limit, err := file.GetLimit()
		if limit > 0 && err == nil {
			// set to 20% of system limit
			zombieProcessCountThreshold = int(float64(limit) * 0.20)
		}
	}
}
