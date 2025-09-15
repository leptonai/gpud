// Package hwslowdown monitors NVIDIA GPU hardware clock events of all GPUs, such as HW Slowdown events.
package hwslowdown

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
)

const (
	Name = "accelerator-nvidia-hw-slowdown"

	// DefaultStateHWSlowdownEvaluationWindow is the window to evaluate the HW slowdown state.
	DefaultStateHWSlowdownEvaluationWindow = 10 * time.Minute

	// DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute is the threshold frequency of the HW slowdown events per minute.
	// If the evaluation window is 10 minutes and for the last 10-minute, 6 events are found, the state is considered unhealthy, where the ratio is 0.6 = 6 / 10.
	// This is to avoid false positives when the HW slowdown events are rare.
	DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute = 0.6
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getTimeNowFunc func() time.Time

	nvmlInstance                  nvidianvml.Instance
	getClockEventsSupportedFunc   func(dev device.Device) (bool, error)
	getClockEventsFunc            func(uuid string, dev device.Device) (nvidianvml.ClockEvents, error)
	getSystemDriverVersionFunc    func() (string, error)
	parseDriverVersionFunc        func(driverVersion string) (int, int, int, error)
	checkClockEventsSupportedFunc func(major int) bool

	gpuUUIDsWithHWSlowdown           map[string]any
	gpuUUIDsWithHWSlowdownThermal    map[string]any
	gpuUUIDsWithHWSlowdownPowerBrake map[string]any

	eventBucket eventstore.Bucket

	evaluationWindow time.Duration
	threshold        float64

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},

		nvmlInstance:                     gpudInstance.NVMLInstance,
		getClockEventsSupportedFunc:      nvidianvml.ClockEventsSupportedByDevice,
		getClockEventsFunc:               nvidianvml.GetClockEvents,
		gpuUUIDsWithHWSlowdown:           make(map[string]any),
		gpuUUIDsWithHWSlowdownThermal:    make(map[string]any),
		gpuUUIDsWithHWSlowdownPowerBrake: make(map[string]any),

		evaluationWindow: DefaultStateHWSlowdownEvaluationWindow,
		threshold:        DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,
	}

	if gpudInstance.NVMLInstance != nil && gpudInstance.NVMLInstance.NVMLExists() {
		c.getSystemDriverVersionFunc = func() (string, error) {
			return nvidianvml.GetSystemDriverVersion(gpudInstance.NVMLInstance.Library().NVML())
		}
		c.parseDriverVersionFunc = nvidianvml.ParseDriverVersion
		c.checkClockEventsSupportedFunc = nvidianvml.ClockEventsSupportedVersion
	}

	if gpudInstance.EventStore != nil {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}
	}

	if gpudInstance != nil && gpudInstance.FailureInjector != nil {
		for _, uuid := range gpudInstance.FailureInjector.GPUUUIDsWithHWSlowdown {
			c.gpuUUIDsWithHWSlowdown[uuid] = nil
		}
		for _, uuid := range gpudInstance.FailureInjector.GPUUUIDsWithHWSlowdownThermal {
			c.gpuUUIDsWithHWSlowdownThermal[uuid] = nil
		}
		for _, uuid := range gpudInstance.FailureInjector.GPUUUIDsWithHWSlowdownPowerBrake {
			c.gpuUUIDsWithHWSlowdownPowerBrake[uuid] = nil
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
	// hw slowdown events are ONLY used internally within this package
	// solely to evaluate the suggested actions
	// so we don't need to return any events externally
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu clock events for hw slowdown")

	cr := &checkResult{
		ts: c.getTimeNowFunc(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

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

	if c.getSystemDriverVersionFunc != nil {
		driverVersion, err := c.getSystemDriverVersionFunc()
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting driver version"
			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}

		major, _, _, err := c.parseDriverVersionFunc(driverVersion)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error parsing driver version"
			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}
		if !c.checkClockEventsSupportedFunc(major) {
			cr.health = apiv1.HealthStateTypeHealthy
			cr.reason = fmt.Sprintf("clock events not supported for driver version %s", driverVersion)
			return cr
		}
	}

	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		supported, err := c.getClockEventsSupportedFunc(dev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting clock events supported"
			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}

		if !supported {
			cr.health = apiv1.HealthStateTypeHealthy
			cr.reason = "clock events not supported"
			return cr
		}

		clockEvents, err := c.getClockEventsFunc(uuid, dev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting clock events"
			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}

		// Check for injected HW slowdown failures
		if _, ok := c.gpuUUIDsWithHWSlowdown[uuid]; ok {
			log.Logger.Warnw("marking HW slowdown to inject failures", "uuid", uuid)
			clockEvents.HWSlowdown = true
			clockEvents.HWSlowdownReasons = append(clockEvents.HWSlowdownReasons, "HW slowdown injected for testing")
		}

		if _, ok := c.gpuUUIDsWithHWSlowdownThermal[uuid]; ok {
			log.Logger.Warnw("marking HW slowdown thermal to inject failures", "uuid", uuid)
			clockEvents.HWSlowdownThermal = true
			clockEvents.HWSlowdownReasons = append(clockEvents.HWSlowdownReasons, "HW slowdown thermal injected for testing")
		}

		if _, ok := c.gpuUUIDsWithHWSlowdownPowerBrake[uuid]; ok {
			log.Logger.Warnw("marking HW slowdown power brake to inject failures", "uuid", uuid)
			clockEvents.HWSlowdownPowerBrake = true
			clockEvents.HWSlowdownReasons = append(clockEvents.HWSlowdownReasons, "HW slowdown power brake injected for testing")
		}

		if clockEvents.HWSlowdown {
			metricHWSlowdown.With(prometheus.Labels{"uuid": uuid}).Set(float64(1))
		} else {
			metricHWSlowdown.With(prometheus.Labels{"uuid": uuid}).Set(float64(0))
		}

		if clockEvents.HWSlowdownThermal {
			metricHWSlowdownThermal.With(prometheus.Labels{"uuid": uuid}).Set(float64(1))
		} else {
			metricHWSlowdownThermal.With(prometheus.Labels{"uuid": uuid}).Set(float64(0))
		}

		if clockEvents.HWSlowdownPowerBrake {
			metricHWSlowdownPowerBrake.With(prometheus.Labels{"uuid": uuid}).Set(float64(1))
		} else {
			metricHWSlowdownPowerBrake.With(prometheus.Labels{"uuid": uuid}).Set(float64(0))
		}

		cr.ClockEvents = append(cr.ClockEvents, clockEvents)

		ev := clockEvents.Event()
		if ev == nil {
			// no clock event found, skip
			continue
		}

		if c.eventBucket != nil {
			log.Logger.Infow("inserting clock events to db", "gpu_uuid", uuid)

			cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
			found, err := c.eventBucket.Find(cctx, *ev)
			ccancel()
			if err != nil {
				cr.err = err
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error finding clock events"
				log.Logger.Warnw(cr.reason, "error", cr.err)
				return cr
			}
			if found != nil {
				log.Logger.Infow("clock event already found in db", "gpu_uuid", uuid)
				continue
			}

			if err := c.eventBucket.Insert(c.ctx, *ev); err != nil {
				cr.err = err
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error inserting clock events"
				log.Logger.Warnw(cr.reason, "error", cr.err)
				return cr
			}
			log.Logger.Infow("inserted clock events to db", "gpu_uuid", uuid)
		}
	}

	if c.evaluationWindow == 0 {
		// no time window to evaluate /state
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no time window to evaluate states"
		return cr
	}

	if c.eventBucket == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no event bucket"
		return cr
	}

	since := c.getTimeNowFunc().Add(-c.evaluationWindow)
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	latestEvents, err := c.eventBucket.Get(cctx, since)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting clock events from db"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}

	if len(latestEvents) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no clock events found"
		return cr
	}

	eventsByMinute := make(map[int]struct{})
	for _, event := range latestEvents {
		minute := int(event.Time.Unix() / 60) // unix seconds to minutes
		eventsByMinute[minute] = struct{}{}
	}
	totalEvents := len(eventsByMinute)
	minutes := c.evaluationWindow.Minutes()
	freqPerMin := float64(totalEvents) / minutes

	if freqPerMin < c.threshold {
		// hw slowdown events happened but within its threshold
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("hw slowdown events frequency per minute %.2f (total events per minute count %d) is less than threshold %.2f for the last %s", freqPerMin, totalEvents, c.threshold, c.evaluationWindow)
		return cr
	}

	// hw slowdown events happened and beyond its threshold
	cr.health = apiv1.HealthStateTypeUnhealthy
	cr.reason = fmt.Sprintf("hw slowdown events frequency per minute %.2f (total events per minute count %d) exceeded threshold %.2f for the last %s", freqPerMin, totalEvents, c.threshold, c.evaluationWindow)
	cr.suggestedActions = &apiv1.SuggestedActions{
		// Hardware slowdown are often caused by GPU overheating or power supply unit (PSU) failing, please do a hardware inspection to mitigate the issue
		RepairActions: []apiv1.RepairActionType{
			apiv1.RepairActionTypeHardwareInspection,
		},
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ClockEvents []nvidianvml.ClockEvents `json:"clock_events,omitempty"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
	// tracks the suggested actions of the last check
	suggestedActions *apiv1.SuggestedActions
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if len(cr.ClockEvents) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"GPU UUID", "HW Slowdown", "HW Slowdown Thermal", "HW Slowdown Power Brake", "Reasons"})
	for _, event := range cr.ClockEvents {
		table.Append([]string{event.UUID, fmt.Sprintf("%t", event.HWSlowdown), fmt.Sprintf("%t", event.HWSlowdownThermal), fmt.Sprintf("%t", event.HWSlowdownPowerBrake), strings.Join(event.HWSlowdownReasons, ", ")})
	}
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
		Reason:           cr.reason,
		Error:            cr.getError(),
		Health:           cr.health,
		SuggestedActions: cr.suggestedActions,
	}

	if len(cr.ClockEvents) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
