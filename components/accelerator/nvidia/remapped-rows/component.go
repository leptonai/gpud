// Package remappedrows tracks the NVIDIA per-GPU remapped rows.
package remappedrows

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	metrics "github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows/metrics"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

// Name is the ID of the remapped rows component.
const Name = "accelerator-nvidia-remapped-rows"

var _ components.Component = &component{}

type component struct {
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

	remappedDueToUncorrectableErrors, err := metrics.ReadRemappedDueToUncorrectableErrors(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read remapped due to uncorrectable errors: %w", err)
	}
	remappingPending, err := metrics.ReadRemappingPending(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read remapping pending: %w", err)
	}
	remappingFailed, err := metrics.ReadRemappingFailed(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read remapping failed: %w", err)
	}

	ms := make([]components.Metric, 0, len(remappedDueToUncorrectableErrors)+len(remappingPending)+len(remappingFailed))
	for _, m := range remappedDueToUncorrectableErrors {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range remappingPending {
		ms = append(ms, components.Metric{
			Metric: m,
			ExtraInfo: map[string]string{
				"gpu_id": m.MetricSecondaryName,
			},
		})
	}
	for _, m := range remappingFailed {
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

func (d *Data) getReason() string {
	if d == nil {
		return "no memory data"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to get memory data -- %s", d.err)
	}
	return ""
}

func (d *Data) getHealth() (string, bool) {
	healthy := d == nil || d.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
}

func (d *Data) getStates() ([]components.State, error) {
	state := components.State{
		Name:   Name,
		Reason: d.getReason(),
	}
	state.Health, state.Healthy = d.getHealth()

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
