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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	rebootEventStore pkghost.RebootEventStore

	countProcessesByStatusFunc  func(ctx context.Context) (map[string][]process.ProcessStatus, error)
	zombieProcessCountThreshold int

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	return &component{
		ctx:                         cctx,
		cancel:                      ccancel,
		rebootEventStore:            gpudInstance.RebootEventStore,
		countProcessesByStatusFunc:  process.CountProcessesByStatus,
		zombieProcessCountThreshold: defaultZombieProcessCountThreshold,
	}, nil
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
	if c.rebootEventStore == nil {
		return nil, nil
	}
	return c.rebootEventStore.GetRebootEvents(ctx, since)
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking os")

	cr := &checkResult{
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

	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	uptime, err := host.UptimeWithContext(cctx)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error getting uptime: %s", err)
		return cr
	}

	cr.Uptimes = Uptimes{
		Seconds:             uptime,
		BootTimeUnixSeconds: pkghost.BootTimeUnixSeconds(),
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 15*time.Second)
	allProcs, err := c.countProcessesByStatusFunc(cctx)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error getting process count: %s", err)
		return cr
	}

	for status, procsWithStatus := range allProcs {
		if status == procs.Zombie {
			cr.ProcessCountZombieProcesses = len(procsWithStatus)
			break
		}
	}
	if cr.ProcessCountZombieProcesses > c.zombieProcessCountThreshold {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("too many zombie processes: %d (threshold: %d)", cr.ProcessCountZombieProcesses, c.zombieProcessCountThreshold)
		return cr
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("os kernel version %s", cr.Kernel.Version)

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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
	health apiv1.HealthStateType
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

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"VM Type", cr.VirtualizationEnvironment.Type})
	table.Append([]string{"Kernel Arch", cr.Kernel.Arch})
	table.Append([]string{"Kernel Version", cr.Kernel.Version})
	table.Append([]string{"Platform Name", cr.Platform.Name})
	table.Append([]string{"Platform Version", cr.Platform.Version})

	boottimeTS := time.Unix(int64(cr.Uptimes.BootTimeUnixSeconds), 0)
	nowUTC := time.Now().UTC()
	uptimeHumanized := humanize.RelTime(boottimeTS, nowUTC, "ago", "from now")
	table.Append([]string{"Uptime", uptimeHumanized})

	table.Append([]string{"Zombie Process Count", fmt.Sprintf("%d", cr.ProcessCountZombieProcesses)})
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

var defaultZombieProcessCountThreshold = 1000

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
			defaultZombieProcessCountThreshold = int(float64(limit) * 0.20)
		}
	}
}
