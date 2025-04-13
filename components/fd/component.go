// Package fd tracks the number of file descriptors used on the host.
package fd

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

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

	kmsgSyncer  *kmsg.Syncer
	eventBucket eventstore.Bucket

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

func New(ctx context.Context, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	kmsgSyncer, err := kmsg.NewSyncer(cctx, Match, eventBucket)
	if err != nil {
		ccancel()
		return nil, err
	}

	return &component{
		ctx:    cctx,
		cancel: ccancel,

		getFileHandlesFunc:            file.GetFileHandles,
		countRunningPIDsFunc:          process.CountRunningPids,
		getUsageFunc:                  file.GetUsage,
		getLimitFunc:                  file.GetLimit,
		checkFileHandlesSupportedFunc: file.CheckFileHandlesSupported,
		checkFDLimitSupportedFunc:     file.CheckFDLimitSupported,

		kmsgSyncer:  kmsgSyncer,
		eventBucket: eventBucket,

		thresholdAllocatedFileHandles: DefaultThresholdAllocatedFileHandles,
		thresholdRunningPIDs:          DefaultThresholdRunningPIDs,
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

func (c *component) States(ctx context.Context) ([]apiv1.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]apiv1.Event, error) {
	return c.eventBucket.Get(ctx, since)
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

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking file descriptors")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	allocatedFileHandles, _, err := c.getFileHandlesFunc()
	if err != nil {
		d.err = err
		d.healthy = false
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting file handles -- %s", err)
		return
	}
	d.AllocatedFileHandles = allocatedFileHandles
	metricAllocatedFileHandles.With(prometheus.Labels{}).Set(float64(allocatedFileHandles))

	runningPIDs, err := c.countRunningPIDsFunc()
	if err != nil {
		d.err = err
		d.healthy = false
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting running pids -- %s", err)
		return
	}
	d.RunningPIDs = runningPIDs
	metricRunningPIDs.With(prometheus.Labels{}).Set(float64(runningPIDs))

	// may fail for mac
	// e.g.,
	// stat /proc: no such file or directory
	usage, uerr := c.getUsageFunc()
	if uerr != nil {
		d.err = uerr
		d.healthy = false
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting usage -- %s", uerr)
		return
	}
	d.Usage = usage

	limit, err := c.getLimitFunc()
	if err != nil {
		d.err = err
		d.healthy = false
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting limit -- %s", err)
		return
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
		d.healthy = false
		d.health = apiv1.StateTypeDegraded
		d.reason = ErrFileHandlesAllocationExceedsWarning
	} else {
		d.healthy = true
		d.health = apiv1.StateTypeHealthy
		d.reason = fmt.Sprintf("current file descriptors: %d, threshold: %d, used_percent: %s",
			d.Usage,
			d.ThresholdAllocatedFileHandles,
			d.ThresholdAllocatedFileHandlesPercent,
		)
	}
}

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
	healthy bool
	health  apiv1.StateType
	// tracks the reason of the last check
	reason string
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates() ([]apiv1.State, error) {
	if d == nil {
		return []apiv1.State{
			{
				Name:              Name,
				Health:            apiv1.StateTypeHealthy,
				DeprecatedHealthy: true,
				Reason:            "no data yet",
			},
		}, nil
	}

	state := apiv1.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		DeprecatedHealthy: d.healthy,
		Health:            d.health,
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.State{state}, nil
}

func calcUsagePct(usage, limit uint64) float64 {
	if limit > 0 {
		return float64(usage) / float64(limit) * 100
	}
	return 0
}
