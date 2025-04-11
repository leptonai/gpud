// Package hwslowdown monitors NVIDIA GPU hardware clock events of all GPUs, such as HW Slowdown events.
package hwslowdown

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/prometheus/client_golang/prometheus"

	components "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
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

	evaluationWindow time.Duration
	threshold        float64

	nvmlInstanceV2     nvml.InstanceV2
	getClockEventsFunc func(uuid string, dev device.Device) (nvidianvml.ClockEvents, error)

	eventBucket eventstore.Bucket

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, nvmlInstanceV2 nvml.InstanceV2, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		evaluationWindow: DefaultStateHWSlowdownEvaluationWindow,
		threshold:        DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,

		nvmlInstanceV2:     nvmlInstanceV2,
		getClockEventsFunc: nvidianvml.GetClockEvents,

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

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking clock events")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	devs := c.nvmlInstanceV2.Devices()
	for uuid, dev := range devs {
		clockEvents, err := c.getClockEventsFunc(uuid, dev)
		if err != nil {
			d.err = err
			d.reason = fmt.Sprintf("error getting clock events for gpu %s", uuid)
			return
		}

		if clockEvents.HWSlowdown {
			metricHWSlowdown.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(1))
		} else {
			metricHWSlowdown.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(0))
		}

		if clockEvents.HWSlowdownThermal {
			metricHWSlowdownThermal.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(1))
		} else {
			metricHWSlowdownThermal.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(0))
		}

		if clockEvents.HWSlowdownPowerBrake {
			metricHWSlowdownPowerBrake.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(1))
		} else {
			metricHWSlowdownPowerBrake.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(0))
		}

		d.ClockEvents = append(d.ClockEvents, clockEvents)

		ev := clockEvents.Event()
		if ev == nil {
			// no clock event found, skip
			continue
		}

		log.Logger.Infow("inserting clock events to db", "gpu_uuid", uuid)

		cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
		found, err := c.eventBucket.Find(cctx, *ev)
		ccancel()
		if err != nil {
			log.Logger.Errorw("failed to find clock events from db", "error", err, "gpu_uuid", uuid)
			d.err = err
			d.reason = fmt.Sprintf("error finding clock events for gpu %s", uuid)
			return
		}
		if found != nil {
			log.Logger.Infow("clock event already found in db", "gpu_uuid", uuid)
			continue
		}

		if err := c.eventBucket.Insert(c.ctx, *ev); err != nil {
			log.Logger.Errorw("failed to insert event", "error", err)
			d.err = err
			d.reason = fmt.Sprintf("error inserting clock events for gpu %s", uuid)
			return
		}
		log.Logger.Infow("inserted clock events to db", "gpu_uuid", uuid)
	}

	if c.evaluationWindow == 0 {
		// no time window to evaluate /state
		d.healthy = true
		d.reason = "no time window to evaluate states"
		return
	}

	since := time.Now().UTC().Add(-c.evaluationWindow)
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	latestEvents, err := c.eventBucket.Get(cctx, since)
	ccancel()
	if err != nil {
		log.Logger.Errorw("failed to get clock events from db", "error", err)
		d.err = err
		d.reason = fmt.Sprintf("error getting clock events from db: %s", err)
		return
	}

	if len(latestEvents) == 0 {
		d.healthy = true
		d.reason = "no clock events found"
		return
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
		d.healthy = true
		d.reason = fmt.Sprintf("hw slowdown events frequency per minute %.2f (total events per minute count %d) is less than threshold %.2f for the last %s", freqPerMin, totalEvents, c.threshold, c.evaluationWindow)
		return
	}

	// hw slowdown events happened and beyond its threshold
	d.healthy = false
	d.reason = fmt.Sprintf("hw slowdown events frequency per minute %.2f (total events per minute count %d) exceeded threshold %.2f for the last %s", freqPerMin, totalEvents, c.threshold, c.evaluationWindow)
	d.suggestedActions = &components.SuggestedActions{
		RepairActions: []components.RepairActionType{
			components.RepairActionTypeHardwareInspection,
		},
		Descriptions: []string{
			"Hardware slowdown are often caused by GPU overheating or power supply unit (PSU) failing, please do a hardware inspection to mitigate the issue",
		},
	}
}

type Data struct {
	ClockEvents []nvidianvml.ClockEvents `json:"clock_events,omitempty"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	healthy bool
	// tracks the reason of the last check
	reason string
	// tracks the suggested actions of the last check
	suggestedActions *components.SuggestedActions
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
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  components.StateHealthy,

		SuggestedActions: d.suggestedActions,
	}
	if !d.healthy {
		state.Health = components.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
