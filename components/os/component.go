// Package os queries the host OS information (e.g., kernel version).
package os

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/shirou/gopsutil/v4/host"
	procs "github.com/shirou/gopsutil/v4/process"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/file"
	pkg_host "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/reboot"
)

// Name is the ID of the OS component.
const Name = "os"

var _ components.Component = &component{}

type component struct {
	ctx         context.Context
	cancel      context.CancelFunc
	eventStore  eventstore.Store
	eventBucket eventstore.Bucket

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventStore:  eventStore,
		eventBucket: eventBucket,
	}, nil
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
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()
	c.eventBucket.Close()

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

	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	virtEnv, err := pkg_host.SystemdDetectVirt(cctx)
	ccancel()
	if err != nil {
		// ignore "context.DeadlineExceeded" since it's not a critical error and it's non-actionable
		if !errors.Is(err, context.DeadlineExceeded) {
			d.err = fmt.Errorf("failed to get virtualization environment using 'systemd-detect-virt': %w", err)
			return
		}

		log.Logger.Warnw("failed to get virtualization environment using 'systemd-detect-virt'", "error", err)
	}
	d.VirtualizationEnvironment = virtEnv

	// for some reason, init failed
	if currentSystemManufacturer == "" && runtime.GOOS == "linux" {
		cctx, ccancel = context.WithTimeout(c.ctx, 20*time.Second)
		currentSystemManufacturer, err = pkg_host.SystemManufacturer(cctx)
		ccancel()
		if err != nil {
			log.Logger.Warnw("failed to get system manufacturer", "error", err)
		}
	}
	d.SystemManufacturer = currentSystemManufacturer

	d.MachineMetadata = currentMachineMetadata

	if err := reboot.RecordEvent(c.ctx, c.eventStore, Name, reboot.LastReboot); err != nil {
		log.Logger.Warnw("failed to create reboot event", "error", err)
	}

	hostID, err := host.HostID()
	if err != nil {
		d.err = fmt.Errorf("failed to get host id: %w", err)
		return
	}
	d.Host = Host{ID: hostID}

	arch, err := host.KernelArch()
	if err != nil {
		d.err = fmt.Errorf("failed to get kernel architecture: %w", err)
		return
	}
	kernelVer, err := host.KernelVersion()
	if err != nil {
		d.err = fmt.Errorf("failed to get kernel version: %w", err)
		return
	}
	d.Kernel = Kernel{Arch: arch, Version: kernelVer}

	platform, family, version, err := host.PlatformInformation()
	if err != nil {
		d.err = fmt.Errorf("failed to get platform information: %w", err)
		return
	}
	d.Platform = Platform{Name: platform, Family: family, Version: version}

	cctx, ccancel = context.WithTimeout(c.ctx, 10*time.Second)
	uptime, err := host.UptimeWithContext(cctx)
	ccancel()
	if err != nil {
		d.err = fmt.Errorf("failed to get uptime: %w", err)
		return
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 10*time.Second)
	boottime, err := host.BootTimeWithContext(cctx)
	ccancel()
	if err != nil {
		d.err = fmt.Errorf("failed to get boot time: %w", err)
		return
	}

	now := time.Now().UTC()
	d.Uptimes = Uptimes{
		Seconds:             uptime,
		SecondsHumanized:    humanize.RelTime(now.Add(time.Duration(-int64(uptime))*time.Second), now, "ago", "from now"),
		BootTimeUnixSeconds: boottime,
		BootTimeHumanized:   humanize.RelTime(time.Unix(int64(boottime), 0), now, "ago", "from now"),
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
	VirtualizationEnvironment   pkg_host.VirtualizationEnvironment `json:"virtualization_environment"`
	SystemManufacturer          string                             `json:"system_manufacturer"`
	MachineMetadata             MachineMetadata                    `json:"machine_metadata"`
	MachineRebooted             bool                               `json:"machine_rebooted"`
	Host                        Host                               `json:"host"`
	Kernel                      Kernel                             `json:"kernel"`
	Platform                    Platform                           `json:"platform"`
	Uptimes                     Uptimes                            `json:"uptimes"`
	ProcessCountZombieProcesses int                                `json:"process_count_zombie_processes"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
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
	Seconds          uint64 `json:"seconds"`
	SecondsHumanized string `json:"seconds_humanized"`

	BootTimeUnixSeconds uint64 `json:"boot_time_unix_seconds"`
	BootTimeHumanized   string `json:"boot_time_humanized"`
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

var (
	currentMachineMetadata    MachineMetadata
	currentSystemManufacturer string

	zombieProcessCountThreshold = 1000
)

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

	// Get boot ID (Linux-specific)
	var err error
	currentMachineMetadata.BootID, err = pkg_host.GetBootID()
	if err != nil {
		log.Logger.Warnw("failed to get boot id", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	currentMachineMetadata.DmidecodeUUID, err = pkg_host.DmidecodeUUID(ctx)
	cancel()
	if err != nil {
		log.Logger.Debugw("failed to get dmidecode uuid", "error", err)
	}

	currentMachineMetadata.OSMachineID, err = pkg_host.GetOSMachineID()
	if err != nil {
		log.Logger.Warnw("failed to get os machine id", "error", err)
	}

	cctx, ccancel := context.WithTimeout(context.Background(), 20*time.Second)
	currentSystemManufacturer, err = pkg_host.SystemManufacturer(cctx)
	ccancel()
	if err != nil {
		log.Logger.Warnw("failed to get system manufacturer", "error", err)
	}
}
