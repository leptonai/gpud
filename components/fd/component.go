// Package fd tracks the number of file descriptors used on the host.
package fd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
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

func (c *component) Labels() []string {
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
	if c.eventBucket == nil {
		return nil, nil
	}
	evStoreEvents, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	apiEvents := make(apiv1.Events, len(evStoreEvents))
	for i, ev := range evStoreEvents {
		apiEvents[i] = ev.ToEvent()
	}
	return apiEvents, nil
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
	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	cr.AllocatedFileHandles, _, cr.err = c.getFileHandlesFunc()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting file handles"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr
	}
	metricAllocatedFileHandles.With(prometheus.Labels{}).Set(float64(cr.AllocatedFileHandles))

	cr.RunningPIDs, cr.err = c.countRunningPIDsFunc()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting running pids"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr
	}
	metricRunningPIDs.With(prometheus.Labels{}).Set(float64(cr.RunningPIDs))

	// may fail for mac
	// e.g.,
	// stat /proc: no such file or directory
	cr.Usage, cr.err = c.getUsageFunc()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting usage"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr
	}

	cr.Limit, cr.err = c.getLimitFunc()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting limit"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr
	}
	metricLimit.With(prometheus.Labels{}).Set(float64(cr.Limit))

	allocatedFileHandlesPct := calcUsagePct(cr.AllocatedFileHandles, cr.Limit)
	cr.AllocatedFileHandlesPercent = fmt.Sprintf("%.2f", allocatedFileHandlesPct)
	metricAllocatedFileHandlesPercent.With(prometheus.Labels{}).Set(allocatedFileHandlesPct)

	usageVal := cr.RunningPIDs // for mac
	if cr.Usage > 0 {
		usageVal = cr.Usage
	}
	usedPct := calcUsagePct(usageVal, cr.Limit)
	cr.UsedPercent = fmt.Sprintf("%.2f", usedPct)
	metricUsedPercent.With(prometheus.Labels{}).Set(usedPct)

	fileHandlesSupported := c.checkFileHandlesSupportedFunc()
	cr.FileHandlesSupported = fileHandlesSupported

	fdLimitSupported := c.checkFDLimitSupportedFunc()
	cr.FDLimitSupported = fdLimitSupported

	var thresholdRunningPIDsPct float64
	if fdLimitSupported && c.thresholdRunningPIDs > 0 {
		thresholdRunningPIDsPct = calcUsagePct(cr.Usage, c.thresholdRunningPIDs)
	}
	cr.ThresholdRunningPIDs = c.thresholdRunningPIDs
	cr.ThresholdRunningPIDsPercent = fmt.Sprintf("%.2f", thresholdRunningPIDsPct)
	metricThresholdRunningPIDs.With(prometheus.Labels{}).Set(float64(c.thresholdRunningPIDs))
	metricThresholdRunningPIDsPercent.With(prometheus.Labels{}).Set(thresholdRunningPIDsPct)

	var thresholdAllocatedFileHandlesPct float64
	if c.thresholdAllocatedFileHandles > 0 {
		thresholdAllocatedFileHandlesPct = calcUsagePct(cr.Usage, min(c.thresholdAllocatedFileHandles, cr.Limit))
	}
	cr.ThresholdAllocatedFileHandles = c.thresholdAllocatedFileHandles
	cr.ThresholdAllocatedFileHandlesPercent = fmt.Sprintf("%.2f", thresholdAllocatedFileHandlesPct)
	metricThresholdAllocatedFileHandles.With(prometheus.Labels{}).Set(float64(c.thresholdAllocatedFileHandles))
	metricThresholdAllocatedFileHandlesPercent.With(prometheus.Labels{}).Set(thresholdAllocatedFileHandlesPct)

	if thresholdAllocatedFileHandlesPct > WarningFileHandlesAllocationPercent {
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = ErrFileHandlesAllocationExceedsWarning
	} else {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no issue found (file descriptor usage is within the threshold)"
	}
	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.Append([]string{"Running PIDs", fmt.Sprintf("%d", cr.RunningPIDs)})
	table.Append([]string{"Usage", fmt.Sprintf("%d", cr.Usage)})
	table.Append([]string{"Limit", fmt.Sprintf("%d", cr.Limit)})
	table.Append([]string{"Used %", cr.UsedPercent})

	table.Append([]string{"Allocated File Handles", fmt.Sprintf("%d", cr.AllocatedFileHandles)})
	table.Append([]string{"Allocated File Handles %", cr.AllocatedFileHandlesPercent})

	table.Append([]string{"Threshold Alloc File Handles", fmt.Sprintf("%d", cr.ThresholdAllocatedFileHandles)})
	table.Append([]string{"Threshold Alloc File Handles %", cr.ThresholdAllocatedFileHandlesPercent})

	table.Append([]string{"Threshold Running PIDs", fmt.Sprintf("%d", cr.ThresholdRunningPIDs)})
	table.Append([]string{"Threshold Running PIDs %", cr.ThresholdRunningPIDsPercent})

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
	state.ExtraInfo = map[string]string{"data": string(b)}
	return apiv1.HealthStates{state}
}

func calcUsagePct(usage, limit uint64) float64 {
	if limit > 0 {
		return float64(usage) / float64(limit) * 100
	}
	return 0
}
