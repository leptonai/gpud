// Package infiniband monitors the infiniband status of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package infiniband

import (
	"bytes"
	"context"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

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
	infinibandstore "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/store"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const (
	Name = "accelerator-nvidia-infiniband"

	defaultCheckInterval  = 30 * time.Second
	defaultRequestTimeout = 15 * time.Second
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkInterval  time.Duration
	requestTimeout time.Duration

	nvmlInstance   nvidianvml.Instance
	toolOverwrites pkgconfigcommon.ToolOverwrites

	ibPortsStore infinibandstore.Store
	eventBucket  eventstore.Bucket
	kmsgSyncer   *kmsg.Syncer

	getTimeNowFunc      func() time.Time
	getThresholdsFunc   func() infiniband.ExpectedPortStates
	getClassDevicesFunc func() (infinibandclass.Devices, error)

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		checkInterval:  defaultCheckInterval,
		requestTimeout: defaultRequestTimeout,

		nvmlInstance:   gpudInstance.NVMLInstance,
		toolOverwrites: gpudInstance.NVIDIAToolOverwrites,

		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: GetDefaultExpectedPortStates,
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.LoadDevices(gpudInstance.NVIDIAToolOverwrites.InfinibandClassRootDir)
		},
	}

	if gpudInstance.DBRW != nil && gpudInstance.DBRO != nil {
		var err error
		c.ibPortsStore, err = infinibandstore.New(gpudInstance.RootCtx, gpudInstance.DBRW, gpudInstance.DBRO)
		if err != nil {
			return nil, err
		}
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
		ticker := time.NewTicker(c.checkInterval)
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
	if err != nil {
		log.Logger.Warnw("error loading infiniband class devices", "devices", len(cr.ClassDevices), "error", err)
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error loading infiniband class devices"
		cr.err = err
		return cr
	}

	var sysClassIBPorts []infiniband.IBPort
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

			sysClassIBPorts = append(sysClassIBPorts, ibport)
		}
	}

	if c.ibPortsStore != nil && len(sysClassIBPorts) > 0 {
		err := c.ibPortsStore.Insert(c.getTimeNowFunc(), sysClassIBPorts)
		if err != nil {
			log.Logger.Warnw("error inserting ib ports into store", "error", err)
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error inserting ib ports into store"
			cr.err = err
			return cr
		}
		if err := c.ibPortsStore.Scan(); err != nil {
			log.Logger.Warnw("error scanning ib ports from store", "error", err)
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error scanning ib ports from store"
			cr.err = err
			return cr
		}
	}

	// no data, skip the evaluation
	if len(sysClassIBPorts) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonNoIbPortData
		log.Logger.Warnw(cr.reason)
		return cr
	}

	evaluateHealthStateWithThresholds(thresholds, sysClassIBPorts, cr)

	// regardless of thresholds, check ib flap/drop events
	// returns until events are marked as processed/discarded
	// by set healthy event
	if c.ibPortsStore != nil {
		// we DO NOT discard past events until the user explicitly
		// inspected and set healthy, in order to not miss critical events
		// this will return empty, once the user inspected and set healthy (to be tombstoned)
		events, err := c.ibPortsStore.LastEvents(zeroTime)
		if err != nil {
			log.Logger.Warnw("error getting ib flap/drop events", "error", err)
		}

		// above "ibPortsStore.LastEvents" only returns the last event per device and per port
		// while keeping the details about "when" the event first happened (e.g., flap happened at x)
		// thus, this infiniband event means, the ib port flap/drop happened at x
		// and may still happen (thus I am emitting the event!)
		// to signal that the hw inspection is required and set healthy is required

		if len(events) > 0 {
			// we convert ib events to gpud events again
			// so that gpud events have detail information
			// while /states have static information
			// since "ibPortsStore.LastEvents" returns only the last event
			// inserting them here as gpud events will have minimum redundancy
			// (e.g., timewindow moves forward for each iteration of check)
			gpudEvents := []eventstore.Event{}
			ibDropDevs := []string{}
			ibFlapDevs := []string{}
			for _, event := range events {
				gpudEvent := eventstore.Event{
					Component: Name,
					Time:      event.Time,
					Name:      event.EventType,
					Type:      string(apiv1.EventTypeWarning),
					Message:   event.EventReason,
				}

				switch event.EventType {
				case infinibandstore.EventTypeIbPortDrop:
					log.Logger.Warnw(event.EventReason)
					gpudEvents = append(gpudEvents, gpudEvent)
					ibDropDevs = append(ibDropDevs, event.Port.Device)

				case infinibandstore.EventTypeIbPortFlap:
					log.Logger.Warnw(event.EventReason)
					gpudEvents = append(gpudEvents, gpudEvent)
					ibFlapDevs = append(ibFlapDevs, event.Port.Device)

				default:
					log.Logger.Warnw("unknown ib event type", "event", event)
				}
			}

			if len(gpudEvents) > 0 {
				sort.Slice(gpudEvents, func(i, j int) bool {
					return gpudEvents[i].Time.Before(gpudEvents[j].Time)
				})
				sort.Strings(ibDropDevs)
				sort.Strings(ibFlapDevs)

				if cr.reason == reasonNoIbPortIssue {
					// Current ports are healthy but there are historical events
					// Only clear the "ok; no infiniband port issue" message if there are
					// actual event descriptions that would make it confusing
					cr.reason = ""
				}

				if cr.reason != "" {
					// e.g., ib port health state violates its expected state/rate threholds
					cr.reason += "; "
				}

				if len(ibDropDevs) > 0 {
					cr.reason += "device(s) down too long: " + strings.Join(ibDropDevs, ", ")
				}
				if len(ibFlapDevs) > 0 {
					if len(ibDropDevs) > 0 {
						cr.reason += "; "
					}
					cr.reason += "device(s) flapping between ACTIVE<>DOWN: " + strings.Join(ibFlapDevs, ", ")
				}

				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.suggestedActions = &apiv1.SuggestedActions{
					RepairActions: []apiv1.RepairActionType{
						apiv1.RepairActionTypeHardwareInspection,
					},
				}

				for _, gpudEvent := range gpudEvents {
					cctx, ccancel := context.WithTimeout(c.ctx, c.requestTimeout)
					prev, err := c.eventBucket.Find(cctx, gpudEvent)
					ccancel()

					if err != nil {
						log.Logger.Warnw("error finding event", "error", err)
					} else if prev == nil {
						// new event
						cctx, ccancel := context.WithTimeout(c.ctx, c.requestTimeout)
						err = c.eventBucket.Insert(cctx, gpudEvent)
						ccancel()
						if err != nil {
							log.Logger.Warnw("error inserting event", "error", err)
						}
					} else {
						log.Logger.Infow("event already exists -- skipped inserting", "event", gpudEvent)
					}
				}
			}
		}
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ClassDevices infinibandclass.Devices `json:"class_devices"`

	// current unhealthy ib ports that are problematic
	// (down/polling/disabled, below expected ib port thresholds)
	unhealthyIBPorts []infiniband.IBPort `json:"-"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the suggested actions for the last check
	suggestedActions *apiv1.SuggestedActions
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

	out := ""

	if len(cr.ClassDevices) > 0 {
		buf := bytes.NewBuffer(nil)
		cr.ClassDevices.RenderTable(buf)

		out += buf.String() + "\n\n"
	}

	return out
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

var zeroTime = time.Time{}
