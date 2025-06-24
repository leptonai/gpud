// Package os queries the host OS information (e.g., kernel version).
package os

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v4/host"
	procs "github.com/shirou/gopsutil/v4/process"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/file"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// Name is the ID of the OS component.
const Name = "os"

const (
	// DefaultMaxAllocatedFileHandles is some high number, in case the system is under high file descriptor usage.
	DefaultMaxAllocatedFileHandles = 10000000

	// DefaultMaxRunningPIDs is some high number, in case fd-max is unlimited
	DefaultMaxRunningPIDs = 900000
)

const (
	defaultMaxAllocatedFileHandlesPctDegraded  = 80.0
	defaultMaxAllocatedFileHandlesPctUnhealthy = 95.0

	defaultMaxRunningPIDsPctDegraded  = 80.0
	defaultMaxRunningPIDsPctUnhealthy = 95.0
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	rebootEventStore pkghost.RebootEventStore
	eventBucket      eventstore.Bucket
	kmsgSyncer       *kmsg.Syncer

	countProcessesByStatusFunc           func(ctx context.Context) (map[string][]process.ProcessStatus, error)
	zombieProcessCountThresholdDegraded  int
	zombieProcessCountThresholdUnhealthy int

	getHostUptimeFunc             func(ctx context.Context) (uint64, error)
	getFileHandlesFunc            func() (uint64, uint64, error)
	countRunningPIDsFunc          func() (uint64, error)
	getUsageFunc                  func() (uint64, error)
	getLimitFunc                  func() (uint64, error)
	checkFileHandlesSupportedFunc func() bool
	checkFDLimitSupportedFunc     func() bool

	// maxAllocatedFileHandles is the number of file descriptors that are currently allocated,
	// at which we consider the system to be under high file descriptor usage.
	maxAllocatedFileHandles             uint64
	maxAllocatedFileHandlesPctDegraded  float64
	maxAllocatedFileHandlesPctUnhealthy float64

	// maxRunningPIDs is the number of running pids at which
	// we consider the system to be under high file descriptor usage.
	// This is useful for triggering alerts when the system is under high load.
	// And useful when the actual system fd-max is set to unlimited.
	maxRunningPIDs             uint64
	maxRunningPIDsPctDegraded  float64
	maxRunningPIDsPctUnhealthy float64

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		rebootEventStore: gpudInstance.RebootEventStore,

		countProcessesByStatusFunc:           process.CountProcessesByStatus,
		zombieProcessCountThresholdDegraded:  defaultZombieProcessCountThresholdDegraded,
		zombieProcessCountThresholdUnhealthy: defaultZombieProcessCountThresholdUnhealthy,

		getHostUptimeFunc:             host.UptimeWithContext,
		getFileHandlesFunc:            file.GetFileHandles,
		countRunningPIDsFunc:          process.CountRunningPids,
		getUsageFunc:                  file.GetUsage,
		getLimitFunc:                  file.GetLimit,
		checkFileHandlesSupportedFunc: file.CheckFileHandlesSupported,
		checkFDLimitSupportedFunc:     file.CheckFDLimitSupported,

		maxAllocatedFileHandles:             DefaultMaxAllocatedFileHandles,
		maxAllocatedFileHandlesPctDegraded:  defaultMaxAllocatedFileHandlesPctDegraded,
		maxAllocatedFileHandlesPctUnhealthy: defaultMaxAllocatedFileHandlesPctUnhealthy,

		maxRunningPIDs:             DefaultMaxRunningPIDs,
		maxRunningPIDsPctDegraded:  defaultMaxRunningPIDsPctDegraded,
		maxRunningPIDsPctUnhealthy: defaultMaxRunningPIDsPctUnhealthy,
	}

	if gpudInstance.EventStore != nil {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		if os.Geteuid() == 0 {
			c.kmsgSyncer, err = kmsg.NewSyncer(cctx, Match, c.eventBucket)
			if err != nil {
				ccancel()
				return nil, err
			}
		}
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		Name,
	}
}

func (c *component) IsSupported() bool {
	return true
}

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
	if c.eventBucket == nil && c.rebootEventStore == nil {
		return nil, nil
	}

	var events apiv1.Events
	if c.eventBucket != nil {
		componentEvents, err := c.eventBucket.Get(ctx, since)
		if err != nil {
			return nil, err
		}

		if len(componentEvents) > 0 {
			events = make(apiv1.Events, 0, len(componentEvents))
			for _, ev := range componentEvents {
				// to prevent duplicate events
				// since "reboot" events and "os" events
				// share the same event store bucket "os"
				if ev.Name == pkghost.EventNameReboot {
					continue
				}
				events = append(events, ev.ToEvent())
			}
		}
	}

	// for now,
	// reboot events are recorded in the "os" bucket
	// until we migrate, this method manually selects
	// only the "reboot" events from the "os" bucket
	// thus there should be no overlap between "eventBucket" and "rebootEventStore"
	if c.rebootEventStore != nil {
		rebootEvents, err := c.rebootEventStore.GetRebootEvents(ctx, since)
		if err != nil {
			return nil, err
		}
		if len(rebootEvents) > 0 {
			events = append(events, rebootEvents.Events()...)
		}
	}

	if len(events) == 0 {
		return nil, nil
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.Time.After(events[j].Time.Time)
	})

	return events, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.kmsgSyncer != nil {
		c.kmsgSyncer.Close()
	}
	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

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
	uptime, err := c.getHostUptimeFunc(cctx)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting uptime"
		log.Logger.Warnw(cr.reason, "error", cr.err)
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
		cr.reason = "error getting process count"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}

	for status, procsWithStatus := range allProcs {
		if status == procs.Zombie {
			cr.ZombieProcesses = len(procsWithStatus)
			break
		}
	}

	metricZombieProcesses.With(prometheus.Labels{}).Set(float64(cr.ZombieProcesses))

	if cr.ZombieProcesses > c.zombieProcessCountThresholdUnhealthy {
		// exceeded high threshold, mark unhealthy
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("too many zombie processes (unhealthy state threshold: %d)", c.zombieProcessCountThresholdUnhealthy)
		cr.suggestedActions = defaultSuggestedActionsForFd
		log.Logger.Warnw(cr.reason, "count", cr.ZombieProcesses)
		return cr
	}
	if cr.ZombieProcesses > c.zombieProcessCountThresholdDegraded {
		// only lower threshold is reached, mark degraded first
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = fmt.Sprintf("too many zombie processes (degraded state threshold: %d)", c.zombieProcessCountThresholdDegraded)
		cr.suggestedActions = defaultSuggestedActionsForFd
		log.Logger.Warnw(cr.reason, "count", cr.ZombieProcesses)
		return cr
	}

	cr.FileDescriptors.AllocatedFileHandles, _, cr.err = c.getFileHandlesFunc()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting file handles"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}
	metricAllocatedFileHandles.With(prometheus.Labels{}).Set(float64(cr.FileDescriptors.AllocatedFileHandles))

	cr.FileDescriptors.RunningPIDs, cr.err = c.countRunningPIDsFunc()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting running pids"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}
	metricRunningPIDs.With(prometheus.Labels{}).Set(float64(cr.FileDescriptors.RunningPIDs))

	// may fail for mac
	// e.g.,
	// stat /proc: no such file or directory
	cr.FileDescriptors.Usage, cr.err = c.getUsageFunc()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting file descriptor usage"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}

	cr.FileDescriptors.Limit, cr.err = c.getLimitFunc()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting file descriptor limit"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}
	metricLimit.With(prometheus.Labels{}).Set(float64(cr.FileDescriptors.Limit))

	allocatedFileHandlesPct := calcUsagePct(cr.FileDescriptors.AllocatedFileHandles, cr.FileDescriptors.Limit)
	cr.FileDescriptors.AllocatedFileHandlesPercent = fmt.Sprintf("%.2f", allocatedFileHandlesPct)
	metricAllocatedFileHandlesPercent.With(prometheus.Labels{}).Set(allocatedFileHandlesPct)

	usageVal := cr.FileDescriptors.RunningPIDs // for mac
	if cr.FileDescriptors.Usage > 0 {
		usageVal = cr.FileDescriptors.Usage
	}
	usedPct := calcUsagePct(usageVal, cr.FileDescriptors.Limit)
	cr.FileDescriptors.UsedPercent = fmt.Sprintf("%.2f", usedPct)
	metricUsedPercent.With(prometheus.Labels{}).Set(usedPct)

	fileHandlesSupported := c.checkFileHandlesSupportedFunc()
	cr.FileDescriptors.FileHandlesSupported = fileHandlesSupported

	fdLimitSupported := c.checkFDLimitSupportedFunc()
	cr.FileDescriptors.FDLimitSupported = fdLimitSupported

	var maxRunningPIDsPct float64
	if c.maxRunningPIDs > 0 {
		maxRunningPIDsPct = calcUsagePct(cr.FileDescriptors.RunningPIDs, c.maxRunningPIDs)
	}

	cr.FileDescriptors.ThresholdRunningPIDs = c.maxRunningPIDs
	metricThresholdRunningPIDs.With(prometheus.Labels{}).Set(float64(c.maxRunningPIDs))

	cr.FileDescriptors.ThresholdRunningPIDsPercent = fmt.Sprintf("%.2f", maxRunningPIDsPct)
	metricThresholdRunningPIDsPercent.With(prometheus.Labels{}).Set(maxRunningPIDsPct)

	if c.maxRunningPIDsPctDegraded > 0 {
		if maxRunningPIDsPct > c.maxRunningPIDsPctUnhealthy {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = fmt.Sprintf("too many running pids (unhealthy state percent threshold: %.2f %%)", c.maxRunningPIDsPctUnhealthy)
			log.Logger.Warnw(cr.reason, "count", cr.FileDescriptors.RunningPIDs)
			cr.suggestedActions = defaultSuggestedActionsForFd
			return cr
		}
		if maxRunningPIDsPct > c.maxRunningPIDsPctDegraded {
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("too many running pids (degraded state percent threshold: %.2f %%)", c.maxRunningPIDsPctDegraded)
			log.Logger.Warnw(cr.reason, "count", cr.FileDescriptors.RunningPIDs)
			cr.suggestedActions = defaultSuggestedActionsForFd
			return cr
		}
	}

	var maxAllocatedFileHandlesPct float64
	if c.maxAllocatedFileHandles > 0 {
		maxAllocatedFileHandlesPct = calcUsagePct(cr.FileDescriptors.Usage, min(c.maxAllocatedFileHandles, cr.FileDescriptors.Limit))
	}

	cr.FileDescriptors.ThresholdAllocatedFileHandles = c.maxAllocatedFileHandles
	metricThresholdAllocatedFileHandles.With(prometheus.Labels{}).Set(float64(c.maxAllocatedFileHandles))

	cr.FileDescriptors.ThresholdAllocatedFileHandlesPercent = fmt.Sprintf("%.2f", maxAllocatedFileHandlesPct)
	metricThresholdAllocatedFileHandlesPercent.With(prometheus.Labels{}).Set(maxAllocatedFileHandlesPct)

	if c.maxAllocatedFileHandlesPctDegraded > 0 {
		if maxAllocatedFileHandlesPct > c.maxAllocatedFileHandlesPctUnhealthy {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = fmt.Sprintf("too many allocated file handles (unhealthy state percent threshold: %.2f %%)", c.maxAllocatedFileHandlesPctUnhealthy)
			log.Logger.Warnw(cr.reason, "count", cr.FileDescriptors.AllocatedFileHandles)
			cr.suggestedActions = defaultSuggestedActionsForFd
			return cr
		}
		if maxAllocatedFileHandlesPct > c.maxAllocatedFileHandlesPctDegraded {
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("too many allocated file handles (degraded state percent threshold: %.2f %%)", c.maxAllocatedFileHandlesPctDegraded)
			log.Logger.Warnw(cr.reason, "count", cr.FileDescriptors.AllocatedFileHandles)
			cr.suggestedActions = defaultSuggestedActionsForFd
			return cr
		}
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "ok"

	return cr
}

var defaultSuggestedActionsForFd = &apiv1.SuggestedActions{
	Description: "check/restart user applications for leaky file descriptors",
	RepairActions: []apiv1.RepairActionType{
		apiv1.RepairActionTypeCheckUserAppAndGPU,
	},
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	VirtualizationEnvironment pkghost.VirtualizationEnvironment `json:"virtualization_environment"`
	SystemManufacturer        string                            `json:"system_manufacturer"`
	MachineMetadata           MachineMetadata                   `json:"machine_metadata"`
	Host                      Host                              `json:"host"`
	Kernel                    Kernel                            `json:"kernel"`
	Platform                  Platform                          `json:"platform"`
	Uptimes                   Uptimes                           `json:"uptimes"`
	ZombieProcesses           int                               `json:"zombie_processes"`
	FileDescriptors           FileDescriptors                   `json:"file_descriptors"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
	// suggested actions
	suggestedActions *apiv1.SuggestedActions
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

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	boottimeTS := time.Unix(int64(cr.Uptimes.BootTimeUnixSeconds), 0)
	nowUTC := time.Now().UTC()
	uptimeHumanized := humanize.RelTime(boottimeTS, nowUTC, "ago", "from now")

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"VM Type", cr.VirtualizationEnvironment.Type})
	table.Append([]string{"Kernel Arch", cr.Kernel.Arch})
	table.Append([]string{"Kernel Version", cr.Kernel.Version})
	table.Append([]string{"Platform Name", cr.Platform.Name})
	table.Append([]string{"Platform Version", cr.Platform.Version})
	table.Append([]string{"Uptime", uptimeHumanized})
	table.Append([]string{"Zombie Process Count", fmt.Sprintf("%d", cr.ZombieProcesses)})

	table.Append([]string{"File Descriptor Running PIDs", fmt.Sprintf("%d", cr.FileDescriptors.RunningPIDs)})
	table.Append([]string{"File Descriptor Usage", fmt.Sprintf("%d", cr.FileDescriptors.Usage)})
	table.Append([]string{"File Descriptor Limit", fmt.Sprintf("%d", cr.FileDescriptors.Limit)})
	table.Append([]string{"File Descriptor Used %", cr.FileDescriptors.UsedPercent})
	table.Append([]string{"File Descriptor Allocated File Handles", fmt.Sprintf("%d", cr.FileDescriptors.AllocatedFileHandles)})
	table.Append([]string{"File Descriptor Allocated File Handles %", cr.FileDescriptors.AllocatedFileHandlesPercent})
	table.Append([]string{"File Descriptor Threshold Alloc File Handles", fmt.Sprintf("%d", cr.FileDescriptors.ThresholdAllocatedFileHandles)})
	table.Append([]string{"File Descriptor Threshold Alloc File Handles %", cr.FileDescriptors.ThresholdAllocatedFileHandlesPercent})
	table.Append([]string{"File Descriptor Threshold Running PIDs", fmt.Sprintf("%d", cr.FileDescriptors.ThresholdRunningPIDs)})
	table.Append([]string{"File Descriptor Threshold Running PIDs %", cr.FileDescriptors.ThresholdRunningPIDsPercent})

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
		Time:             metav1.NewTime(cr.ts),
		Component:        Name,
		Name:             Name,
		Health:           cr.health,
		Reason:           cr.reason,
		SuggestedActions: cr.suggestedActions,
		Error:            cr.getError(),
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{"data": string(b)}
	return apiv1.HealthStates{state}
}

var (
	defaultZombieProcessCountThresholdDegraded  = 1000
	defaultZombieProcessCountThresholdUnhealthy = 2000
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
			defaultZombieProcessCountThresholdDegraded = int(float64(limit) * 0.20)

			// set to 80% of system limit
			defaultZombieProcessCountThresholdUnhealthy = int(float64(limit) * 0.80)
		}
	}
}

func calcUsagePct(usage, limit uint64) float64 {
	if limit > 0 {
		return float64(usage) / float64(limit) * 100
	}
	return 0
}
