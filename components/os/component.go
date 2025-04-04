// Package os queries the host OS information (e.g., kernel version).
package os

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/host"
	procs "github.com/shirou/gopsutil/v4/process"

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
	ctx              context.Context
	cancel           context.CancelFunc
	rebootEventStore pkghost.RebootEventStore

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, rebootEventStore pkghost.RebootEventStore) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:              cctx,
		cancel:           ccancel,
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

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.rebootEventStore.GetRebootEvents(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

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
	log.Logger.Infow("checking memory")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	d.VirtualizationEnvironment = pkghost.VirtualizationEnv()
	d.SystemManufacturer = pkghost.SystemManufacturer()
	d.MachineMetadata = MachineMetadata{
		BootID:        pkghost.BootID(),
		DmidecodeUUID: pkghost.DmidecodeUUID(),
		OSMachineID:   pkghost.OSMachineID(),
	}
	d.Host = Host{ID: pkghost.HostID()}
	d.Kernel = Kernel{Arch: pkghost.Arch(), Version: pkghost.KernelVersion()}
	d.Platform = Platform{Name: pkghost.Platform(), Family: pkghost.PlatformFamily(), Version: pkghost.PlatformVersion()}

	if err := c.rebootEventStore.RecordReboot(c.ctx); err != nil {
		log.Logger.Warnw("failed to create reboot event", "error", err)
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
	uptime, err := host.UptimeWithContext(cctx)
	ccancel()
	if err != nil {
		d.err = fmt.Errorf("failed to get uptime: %w", err)
		return
	}

	d.Uptimes = Uptimes{
		Seconds:             uptime,
		BootTimeUnixSeconds: pkghost.BootTimeUnixSeconds(),
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 10*time.Second)
	allProcs, err := process.CountProcessesByStatus(cctx)
	ccancel()
	if err != nil {
		d.err = fmt.Errorf("failed to get process count: %w", err)
		return
	}

	for status, procsWithStatus := range allProcs {
		if status == procs.Zombie {
			d.ProcessCountZombieProcesses = len(procsWithStatus)
			break
		}
	}
}

type Data struct {
	VirtualizationEnvironment   pkghost.VirtualizationEnvironment `json:"virtualization_environment"`
	SystemManufacturer          string                            `json:"system_manufacturer"`
	MachineMetadata             MachineMetadata                   `json:"machine_metadata"`
	MachineRebooted             bool                              `json:"machine_rebooted"`
	Host                        Host                              `json:"host"`
	Kernel                      Kernel                            `json:"kernel"`
	Platform                    Platform                          `json:"platform"`
	Uptimes                     Uptimes                           `json:"uptimes"`
	ProcessCountZombieProcesses int                               `json:"process_count_zombie_processes"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error
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

func (d *Data) getReason() string {
	if d == nil {
		return "no os data"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to get os data -- %s", d.err)
	}

	if d.ProcessCountZombieProcesses > zombieProcessCountThreshold {
		return fmt.Sprintf("too many zombie processes: %d (threshold: %d)", d.ProcessCountZombieProcesses, zombieProcessCountThreshold)
	}

	return fmt.Sprintf("os kernel version %s", d.Kernel.Version)
}

func (d *Data) getHealth() (string, bool) {
	healthy := d == nil || d.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}

	if d != nil && d.ProcessCountZombieProcesses > zombieProcessCountThreshold {
		healthy = false
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
