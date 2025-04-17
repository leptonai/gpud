// Package fd tracks the number of file descriptors used on the host.
package fd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// Name is the name of the file descriptor component.
const Name = "file-descriptor"

const (
	// DefaultThresholdAllocatedFileHandles is some high number, in case the system is under high file descriptor usage.
	DefaultThresholdAllocatedFileHandles = 10000000

	// DefaultThresholdRunningPIDs is some high number, in case fd-max is unlimited
	DefaultThresholdRunningPIDs = 900000

	WarningFileHandlesAllocationPercent    = 80.0
	ErrFileHandlesAllocationExceedsWarning = "file handles allocation exceeds its threshold (80%)"
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getFileHandlesFunc            func() (uint64, uint64, error)
	countRunningPIDsFunc          func() (uint64, error)
	getUsageFunc                  func() (uint64, error)
	getLimitFunc                  func() (uint64, error)
	checkFileHandlesSupportedFunc func() bool
	checkFDLimitSupportedFunc     func() bool

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	// thresholdAllocatedFileHandles is the number of file descriptors that are currently allocated,
	// at which we consider the system to be under high file descriptor usage.
	thresholdAllocatedFileHandles uint64
	// thresholdRunningPIDs is the number of running pids at which
	// we consider the system to be under high file descriptor usage.
	// This is useful for triggering alerts when the system is under high load.
	// And useful when the actual system fd-max is set to unlimited.
	thresholdRunningPIDs uint64

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		getFileHandlesFunc:            file.GetFileHandles,
		countRunningPIDsFunc:          process.CountRunningPids,
		getUsageFunc:                  file.GetUsage,
		getLimitFunc:                  file.GetLimit,
		checkFileHandlesSupportedFunc: file.CheckFileHandlesSupported,
		checkFDLimitSupportedFunc:     file.CheckFDLimitSupported,

		thresholdAllocatedFileHandles: DefaultThresholdAllocatedFileHandles,
		thresholdRunningPIDs:          DefaultThresholdRunningPIDs,
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		c.kmsgSyncer, err = kmsg.NewSyncer(cctx, Match, c.eventBucket)
		if err != nil {
			ccancel()
			return nil, err
		}
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
	if c.eventBucket != nil {
		return c.eventBucket.Get(ctx, since)
	}
	return nil, nil
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
	log.Logger.Infow("checking file descriptors")
	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	allocatedFileHandles, _, err := c.getFileHandlesFunc()
	if err != nil {
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting file handles -- %s", err)
		return d
	}
	d.AllocatedFileHandles = allocatedFileHandles
	metricAllocatedFileHandles.With(prometheus.Labels{}).Set(float64(allocatedFileHandles))

	runningPIDs, err := c.countRunningPIDsFunc()
	if err != nil {
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting running pids -- %s", err)
		return d
	}
	d.RunningPIDs = runningPIDs
	metricRunningPIDs.With(prometheus.Labels{}).Set(float64(runningPIDs))

	// may fail for mac
	// e.g.,
	// stat /proc: no such file or directory
	usage, uerr := c.getUsageFunc()
	if uerr != nil {
		d.err = uerr
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting usage -- %s", uerr)
		return d
	}
	d.Usage = usage

	limit, err := c.getLimitFunc()
	if err != nil {
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting limit -- %s", err)
		return d
	}
	d.Limit = limit
	metricLimit.With(prometheus.Labels{}).Set(float64(limit))

	allocatedFileHandlesPct := calcUsagePct(allocatedFileHandles, limit)
	d.AllocatedFileHandlesPercent = fmt.Sprintf("%.2f", allocatedFileHandlesPct)
	metricAllocatedFileHandlesPercent.With(prometheus.Labels{}).Set(allocatedFileHandlesPct)

	usageVal := runningPIDs // for mac
	if usage > 0 {
		usageVal = usage
	}
	usedPct := calcUsagePct(usageVal, limit)
	d.UsedPercent = fmt.Sprintf("%.2f", usedPct)
	metricUsedPercent.With(prometheus.Labels{}).Set(usedPct)

	fileHandlesSupported := c.checkFileHandlesSupportedFunc()
	d.FileHandlesSupported = fileHandlesSupported

	fdLimitSupported := c.checkFDLimitSupportedFunc()
	d.FDLimitSupported = fdLimitSupported

	var thresholdRunningPIDsPct float64
	if fdLimitSupported && c.thresholdRunningPIDs > 0 {
		thresholdRunningPIDsPct = calcUsagePct(usage, c.thresholdRunningPIDs)
	}
	d.ThresholdRunningPIDs = c.thresholdRunningPIDs
	d.ThresholdRunningPIDsPercent = fmt.Sprintf("%.2f", thresholdRunningPIDsPct)
	metricThresholdRunningPIDs.With(prometheus.Labels{}).Set(float64(c.thresholdRunningPIDs))
	metricThresholdRunningPIDsPercent.With(prometheus.Labels{}).Set(thresholdRunningPIDsPct)

	var thresholdAllocatedFileHandlesPct float64
	if c.thresholdAllocatedFileHandles > 0 {
		thresholdAllocatedFileHandlesPct = calcUsagePct(usage, min(c.thresholdAllocatedFileHandles, limit))
	}
	d.ThresholdAllocatedFileHandles = c.thresholdAllocatedFileHandles
	d.ThresholdAllocatedFileHandlesPercent = fmt.Sprintf("%.2f", thresholdAllocatedFileHandlesPct)
	metricThresholdAllocatedFileHandles.With(prometheus.Labels{}).Set(float64(c.thresholdAllocatedFileHandles))
	metricThresholdAllocatedFileHandlesPercent.With(prometheus.Labels{}).Set(thresholdAllocatedFileHandlesPct)

	if thresholdAllocatedFileHandlesPct > WarningFileHandlesAllocationPercent {
		d.health = apiv1.StateTypeDegraded
		d.reason = ErrFileHandlesAllocationExceedsWarning
	} else {
		d.health = apiv1.StateTypeHealthy
		d.reason = fmt.Sprintf("current file descriptors: %d, threshold: %d, used_percent: %s",
			d.Usage,
			d.ThresholdAllocatedFileHandles,
			d.ThresholdAllocatedFileHandlesPercent,
		)
	}
	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	// The number of file descriptors currently allocated on the host (not per process).
	AllocatedFileHandles uint64 `json:"allocated_file_handles"`
	// The number of running PIDs returned by https://pkg.go.dev/github.com/shirou/gopsutil/v4/process#Pids.
	RunningPIDs uint64 `json:"running_pids"`
	Usage       uint64 `json:"usage"`
	Limit       uint64 `json:"limit"`

	// AllocatedFileHandlesPercent is the percentage of file descriptors that are currently allocated,
	// based on the current file descriptor limit and the current number of file descriptors allocated on the host (not per process).
	AllocatedFileHandlesPercent string `json:"allocated_file_handles_percent"`
	// UsedPercent is the percentage of file descriptors that are currently in use,
	// based on the current file descriptor limit on the host (not per process).
	UsedPercent string `json:"used_percent"`

	ThresholdAllocatedFileHandles        uint64 `json:"threshold_allocated_file_handles"`
	ThresholdAllocatedFileHandlesPercent string `json:"threshold_allocated_file_handles_percent"`

	ThresholdRunningPIDs        uint64 `json:"threshold_running_pids"`
	ThresholdRunningPIDsPercent string `json:"threshold_running_pids_percent"`

	// Set to true if the file handles are supported.
	FileHandlesSupported bool `json:"file_handles_supported"`
	// Set to true if the file descriptor limit is supported.
	FDLimitSupported bool `json:"fd_limit_supported"`

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

	table.Append([]string{"Allocated File Handles", fmt.Sprintf("%d", d.AllocatedFileHandles)})
	table.Append([]string{"Running PIDs", fmt.Sprintf("%d", d.RunningPIDs)})
	table.Append([]string{"Usage", fmt.Sprintf("%d", d.Usage)})
	table.Append([]string{"Limit", fmt.Sprintf("%d", d.Limit)})
	table.Append([]string{"Allocated File Handles Percent", d.AllocatedFileHandlesPercent})
	table.Append([]string{"Used Percent", d.UsedPercent})

	table.Append([]string{"Threshold Allocated File Handles", fmt.Sprintf("%d", d.ThresholdAllocatedFileHandles)})
	table.Append([]string{"Threshold Allocated File Handles %", d.ThresholdAllocatedFileHandlesPercent})

	table.Append([]string{"Threshold Running PIDs", fmt.Sprintf("%d", d.ThresholdRunningPIDs)})
	table.Append([]string{"Threshold Running PIDs %", d.ThresholdRunningPIDsPercent})

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
		return apiv1.HealthStates{
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

func calcUsagePct(usage, limit uint64) float64 {
	if limit > 0 {
		return float64(usage) / float64(limit) * 100
	}
	return 0
}
