// Package infiniband monitors the infiniband status of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package infiniband

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	infinibandclass "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/class"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const (
	Name = "accelerator-nvidia-infiniband"

	defaultIbPortsSnapshotsRetentionPeriod = 10 * time.Minute
	defaultIbPortDropThreshold             = 5 * time.Minute // 1-minute of buffer in case each periodic check takes longer
	defaultIbPortFlapEvaluatePeriod        = 5 * time.Minute // 1-minute of buffer in case each periodic check takes longer
	defaultRequestTimeout                  = 15 * time.Second
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance   nvidianvml.Instance
	toolOverwrites pkgconfigcommon.ToolOverwrites

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	getTimeNowFunc      func() time.Time
	getThresholdsFunc   func() infiniband.ExpectedPortStates
	getClassDevicesFunc func() (infinibandclass.Devices, error)
	getIbstatOutputFunc func(ctx context.Context) (*infiniband.IbstatOutput, error)

	// ibPortsSnapshotsRetentionPeriod is the duration to retain the last ib ports data.
	ibPortsSnapshotsRetentionPeriod time.Duration
	// lastIbPortsSnapshots is the last ib ports data, with up to [ibPortsSnapshotsRetentionPeriod] items.
	// They are sorted in the ascending order of [ibPortsSnapshot.ts].
	lastIbPortsSnapshots ibPortsSnapshots

	ibPortDropThreshold      time.Duration
	ibPortFlapEvaluatePeriod time.Duration
	requestTimeout           time.Duration

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

// ibPortsSnapshot is a snapshot of all IB ports at a given time
// TODO: store them in events store for persistency on gpud restarts
type ibPortsSnapshot struct {
	ts time.Time

	// all IB ports that are found in the system
	all []infiniband.IBPort

	// only unhealthy IB ports that violates the thresholds
	unhealthy []infiniband.IBPort
}

type ibPortsSnapshots []ibPortsSnapshot

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstance:   gpudInstance.NVMLInstance,
		toolOverwrites: gpudInstance.NVIDIAToolOverwrites,

		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: GetDefaultExpectedPortStates,
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.LoadDevices(gpudInstance.NVIDIAToolOverwrites.InfinibandClassRootDir)
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return infiniband.GetIbstatOutput(ctx, []string{gpudInstance.NVIDIAToolOverwrites.IbstatCommand})
		},

		ibPortsSnapshotsRetentionPeriod: defaultIbPortsSnapshotsRetentionPeriod,
		ibPortDropThreshold:             defaultIbPortDropThreshold,
		ibPortFlapEvaluatePeriod:        defaultIbPortFlapEvaluatePeriod,
		requestTimeout:                  defaultRequestTimeout,
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
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}
}

func (c *component) IsSupported() bool {
	if c.nvmlInstance == nil {
		return false
	}
	return c.nvmlInstance.NVMLExists() && c.nvmlInstance.ProductName() != ""
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
	evs, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	return evs.Events(), nil
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
	log.Logger.Infow("checking nvidia gpu infiniband")

	cr := &checkResult{
		ts: c.getTimeNowFunc(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// nothing specified for this machine, gpud MUST skip the ib check
	thresholds := c.getThresholdsFunc()
	if thresholds.IsZero() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonNoThreshold
		return cr
	}

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}

	var err error
	cr.ClassDevices, err = c.getClassDevicesFunc()
	if len(cr.ClassDevices) == 0 || err != nil {
		log.Logger.Warnw("error loading infiniband class devices", "devices", len(cr.ClassDevices), "error", err)
	}

	var currentPorts []infiniband.IBPort
	for _, dev := range cr.ClassDevices {
		for _, port := range dev.Ports {
			ibport := infiniband.IBPort{
				Port:          port.Port,
				Device:        dev.Name,
				State:         port.State,
				PhysicalState: port.PhysState,
				RateGBSec:     int(port.RateGBSec),
				LinkLayer:     port.LinkLayer,
			}
			if !ibport.IsIBPort() {
				continue
			}

			if port.Counters.LinkDowned != nil {
				ibport.TotalLinkDowned = *port.Counters.LinkDowned

				devicePort := dev.Name + "_" + port.Name
				linkDowned := float64(*port.Counters.LinkDowned)
				metricIbLinkedDowned.With(prometheus.Labels{"device_port": devicePort}).Set(linkDowned)
			}

			currentPorts = append(currentPorts, ibport)
		}
	}

	if c.getIbstatOutputFunc != nil {
		// TODO: deprecate in favor of class dir data
		// "ibstat" may fail if there's a port device that is wrongly mapped (e.g., exit 255)
		// but can still return the partial output with the correct data
		cctx, ccancel := context.WithTimeout(c.ctx, c.requestTimeout)
		cr.IbstatOutput, cr.err = c.getIbstatOutputFunc(cctx)
		ccancel()

		// TODO: deprecate in favor of class dir data
		if cr.err != nil {
			if errors.Is(cr.err, infiniband.ErrNoIbstatCommand) {
				cr.health = apiv1.HealthStateTypeHealthy
				cr.reason = "ibstat command not found"
			} else {
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "ibstat command failed"
				log.Logger.Warnw(cr.reason, "error", cr.err)
			}
		}
	}

	// no event bucket, no need for timeseries data checks
	// (e.g., "gpud scan" one-off checks)
	if c.eventBucket == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonNoEventBucket
		return cr
	}

	// "ibstat" command returned no data, skip the evaluation
	// TODO: deprecate in favor of class dir data
	if cr.err == nil && len(currentPorts) == 0 && cr.IbstatOutput == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonNoIbPortData
		log.Logger.Warnw(cr.reason)
		return cr
	}

	evaluateHealthStateWithThresholds(thresholds, currentPorts, cr)
	if len(cr.unhealthyIBPorts) == 0 && cr.IbstatOutput != nil {
		log.Logger.Warnw("infiniband class dir data is available but no unhealthy IB ports are found, using ibstat as fallback", "ibClassDir", c.toolOverwrites.InfinibandClassRootDir)

		// only use ibstat as fallback if class dir data is not available or no unhealthy IB ports are found
		// TODO: deprecate in favor of class dir data
		evaluateHealthStateWithThresholds(thresholds, cr.IbstatOutput.Parsed.IBPorts(), cr)
	}

	// ib switch fault does NOT need historical ports data
	// just use the current ports data
	evaluateIBSwitchFault(currentPorts, cr)

	// ib port drop/flap detection requires historical ports data
	// record them in memory for now
	c.recordIbPortsSnapshot(ibPortsSnapshot{
		ts:        cr.ts,
		all:       currentPorts,
		unhealthy: cr.unhealthyIBPorts,
	})
	evaluateIBPortsDrop(c.lastIbPortsSnapshots, c.ibPortDropThreshold, cr)
	evaluateIBPortFlap(c.lastIbPortsSnapshots, c.ibPortFlapEvaluatePeriod, cr)

	return cr
}

func (c *component) recordIbPortsSnapshot(snapshot ibPortsSnapshot) {
	c.lastIbPortsSnapshots = append(c.lastIbPortsSnapshots, snapshot)

	// truncate any entry that is older than this timestamp
	staleTime := snapshot.ts.Add(-c.ibPortsSnapshotsRetentionPeriod)

	filtered := make([]ibPortsSnapshot, 0, len(c.lastIbPortsSnapshots))
	for _, existing := range c.lastIbPortsSnapshots {
		if existing.ts.Before(staleTime) {
			// this entry happened before the stale time, skip it
			continue
		}
		filtered = append(filtered, existing)
	}

	// now overwrite
	c.lastIbPortsSnapshots = filtered
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ClassDevices infinibandclass.Devices `json:"class_devices"`
	// TODO: deprecate in favor of class dir data
	IbstatOutput *infiniband.IbstatOutput `json:"ibstat_output"`

	// current unhealthy ib ports that are problematic
	// (down/polling/disabled, below expected ib port thresholds)
	unhealthyIBPorts []infiniband.IBPort `json:"-"`

	// timestamp of the last check
	ts time.Time
	// error from the last check with "ibstat" command and other operations
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the suggested actions for the last check
	suggestedActions *apiv1.SuggestedActions
	// tracks the reason of the last check
	reason string

	// reasonIbSwitchFault is set if and only if
	// all ports are down
	// when this check happened
	reasonIbSwitchFault string

	// IB port drop when a port has been down for more than 4-minute
	reasonIbPortsDrop string

	// IB port flap when a port is down and back to active for the last 4-minute
	reasonIbPortsFlap string
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if cr.IbstatOutput == nil {
		return "no data"
	}

	out := ""

	if len(cr.ClassDevices) > 0 {
		buf := bytes.NewBuffer(nil)
		cr.ClassDevices.RenderTable(buf)

		out += buf.String() + "\n\n"
	}

	if cr.IbstatOutput != nil {
		buf := bytes.NewBuffer(nil)
		table := tablewriter.NewWriter(buf)
		table.SetAlignment(tablewriter.ALIGN_CENTER)
		table.SetHeader([]string{"Port Device Name", "Port1 State", "Port1 Physical State", "Port1 Rate"})
		for _, card := range cr.IbstatOutput.Parsed {
			table.Append([]string{
				card.Device,
				card.Port1.State,
				card.Port1.PhysicalState,
				fmt.Sprintf("%d", card.Port1.Rate),
			})
		}
		table.Render()

		out += buf.String() + "\n\n"
	}

	return out
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}

	reason := cr.reason

	if cr.reasonIbSwitchFault != "" {
		reason += "; " + cr.reasonIbSwitchFault
	}
	if cr.reasonIbPortsDrop != "" {
		reason += "; " + cr.reasonIbPortsDrop
	}
	if cr.reasonIbPortsFlap != "" {
		reason += "; " + cr.reasonIbPortsFlap
	}

	return reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getSuggestedActions() *apiv1.SuggestedActions {
	if cr == nil {
		return nil
	}
	return cr.suggestedActions
}

func (cr *checkResult) getError() string {
	if cr == nil {
		return ""
	}
	if cr.err != nil {
		return cr.err.Error()
	}

	return ""
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
		SuggestedActions: cr.getSuggestedActions(),
		Error:            cr.getError(),
	}

	return apiv1.HealthStates{state}
}
