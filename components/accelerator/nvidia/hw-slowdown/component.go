// Package hwslowdown monitors NVIDIA GPU hardware clock events of all GPUs, such as HW Slowdown events.
package hwslowdown

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	metrics "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/metrics"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

const (
	// DefaultStateHWSlowdownEvaluationWindow is the window to evaluate the HW slowdown state.
	DefaultStateHWSlowdownEvaluationWindow = 10 * time.Minute

	// DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute is the threshold frequency of the HW slowdown events per minute.
	// If the evaluation window is 10 minutes and for the last 10-minute, 6 events are found, the state is considered unhealthy, where the ratio is 0.6 = 6 / 10.
	// This is to avoid false positives when the HW slowdown events are rare.
	DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute = 0.6
)

// Name is the name of the NVIDIA GPU hardware slowdown component.
const Name = "accelerator-nvidia-hw-slowdown"

var _ components.Component = &component{}

type component struct {
	stateHWSlowdownEvaluationWindow                  time.Duration
	stateHWSlowdownEventsThresholdFrequencyPerMinute float64

	ctx    context.Context
	cancel context.CancelFunc

	nvmlLib     nvml_lib.Library
	eventBucket eventstore.Bucket
	gatherer    prometheus.Gatherer

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, nvmlLib nvml_lib.Library, eventBucket eventstore.Bucket) (components.Component, error) {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		stateHWSlowdownEvaluationWindow:                  DefaultStateHWSlowdownEvaluationWindow,
		stateHWSlowdownEventsThresholdFrequencyPerMinute: DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,

		ctx:         cctx,
		cancel:      ccancel,
		nvmlLib:     nvmlLib,
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
	if c.stateHWSlowdownEvaluationWindow == 0 {
		log.Logger.Debugw("no time window to evaluate /states", "component", c.Name())
		return []components.State{
			{
				Name:    "hw_slowdown",
				Healthy: true,
			},
		}, nil
	}

	since := time.Now().UTC().Add(-c.stateHWSlowdownEvaluationWindow)
	events, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		log.Logger.Debugw("no event found for /states", "component", c.Name(), "since", humanize.Time(since))
		return []components.State{
			{
				Name:    "hw_slowdown",
				Healthy: true,
			},
		}, nil
	}

	eventsByMinute := make(map[int]struct{})
	for _, event := range events {
		minute := int(event.Time.Unix() / 60) // unix seconds to minutes
		eventsByMinute[minute] = struct{}{}
	}
	totalEvents := len(eventsByMinute)
	minutes := c.stateHWSlowdownEvaluationWindow.Minutes()
	freqPerMin := float64(totalEvents) / minutes

	if freqPerMin < c.stateHWSlowdownEventsThresholdFrequencyPerMinute {
		log.Logger.Debugw("hw slowdown events count is less than threshold", "component", c.Name(), "since", humanize.Time(since), "count", len(eventsByMinute), "threshold", c.stateHWSlowdownEventsThresholdFrequencyPerMinute)
		return []components.State{
			{
				Name:    "hw_slowdown",
				Healthy: true,
				Reason:  fmt.Sprintf("hw slowdown events frequency per minute %.2f (total events per minute count %d) is less than threshold %.2f for the last %s", freqPerMin, len(eventsByMinute), c.stateHWSlowdownEventsThresholdFrequencyPerMinute, c.stateHWSlowdownEvaluationWindow),
			},
		}, nil
	}

	return []components.State{
		{
			Name:    "hw_slowdown",
			Healthy: false,
			Reason: fmt.Sprintf("hw slowdown events frequency per minute %.2f (total events per minute count %d) exceeded threshold %.2f for the last %s",
				freqPerMin, len(eventsByMinute), c.stateHWSlowdownEventsThresholdFrequencyPerMinute, c.stateHWSlowdownEvaluationWindow),
			SuggestedActions: &common.SuggestedActions{
				RepairActions: []common.RepairActionType{
					common.RepairActionTypeHardwareInspection,
				},
				Descriptions: []string{
					"Hardware slowdown are often caused by GPU overheating or power supply unit (PSU) failing, please do a hardware inspection to mitigate the issue",
				},
			},
		},
	}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	hwSlowdown, err := metrics.ReadHWSlowdown(ctx, since)
	if err != nil {
		return nil, err
	}
	hwSlowdownThermal, err := metrics.ReadHWSlowdownThermal(ctx, since)
	if err != nil {
		return nil, err
	}
	hwSlowdownPowerBrake, err := metrics.ReadHWSlowdownPowerBrake(ctx, since)
	if err != nil {
		return nil, err
	}

	ms := make([]components.Metric, 0, len(hwSlowdown)+len(hwSlowdownThermal)+len(hwSlowdownPowerBrake))
	for _, m := range hwSlowdown {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range hwSlowdownThermal {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range hwSlowdownPowerBrake {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}

	return ms, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()
	c.eventBucket.Close()

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return metrics.Register(reg, dbRW, dbRO, tableName)
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking remapped rows")

	d := Data{
		ts: time.Now().UTC(),
	}
	metrics.SetLastUpdateUnixSeconds(float64(d.ts.Unix()))
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	cctx, ccancel := context.WithTimeout(c.ctx, 5*time.Second)
	ccancel()

	_ = cctx
}

type Data struct {

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
}
