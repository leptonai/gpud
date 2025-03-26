// Package remappedrows tracks the NVIDIA per-GPU remapped rows.
package remappedrows

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	metrics "github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows/metrics"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// Name is the ID of the remapped rows component.
const Name = "accelerator-nvidia-remapped-rows"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvml.InstanceV2
	eventBucket  eventstore.Bucket
	gatherer     prometheus.Gatherer

	getRemappedRowsFunc func(uuid string, dev device.Device) (nvml.RemappedRows, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, nvmlInstance nvml.InstanceV2, eventBucket eventstore.Bucket) (components.Component, error) {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:                 cctx,
		cancel:              ccancel,
		nvmlInstance:        nvmlInstance,
		eventBucket:         eventBucket,
		getRemappedRowsFunc: nvml.GetRemappedRows,
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
		ProductName:                       c.nvmlInstance.ProductName(),
		MemoryErrorManagementCapabilities: c.nvmlInstance.GetMemoryErrorManagementCapabilities(),
		RemappedRows:                      nil,

		ts: time.Now().UTC(),
	}
	metrics.SetLastUpdateUnixSeconds(float64(d.ts.Unix()))
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	devices := c.nvmlInstance.Devices()
	d.RemappedRows = make(map[string]nvml.RemappedRows)
	for uuid, dev := range devices {
		remappedRows, err := c.getRemappedRowsFunc(uuid, dev)
		if err != nil {
			d.err = fmt.Errorf("failed to get remapped rows for %s: %w", uuid, err)
			continue
		}
		d.RemappedRows[uuid] = remappedRows

		b, _ := json.Marshal(d)
		if remappedRows.RemappingPending {
			log.Logger.Warnw("inserting event for remapping pending", "uuid", uuid)

			cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
			err = c.eventBucket.Insert(
				cctx,
				components.Event{
					Time:    metav1.Time{Time: d.ts},
					Name:    "row_remapping_pending",
					Type:    common.EventTypeWarning,
					Message: fmt.Sprintf("%s detected pending row remapping", uuid),
					ExtraInfo: map[string]string{
						"gpu_id":   uuid,
						"data":     string(b),
						"encoding": "json",
					},
					SuggestedActions: nil,
				},
			)
			ccancel()
			if err != nil {
				d.err = fmt.Errorf("failed to insert event for remapping pending for %s: %w", uuid, err)
				continue
			}
		}

		if remappedRows.RemappingFailed {
			log.Logger.Warnw("inserting event for remapping failed", "uuid", uuid)

			cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
			err = c.eventBucket.Insert(
				cctx,
				components.Event{
					Time:    metav1.Time{Time: d.ts},
					Name:    "row_remapping_failed",
					Type:    common.EventTypeWarning,
					Message: fmt.Sprintf("%s detected failed row remapping", uuid),
					ExtraInfo: map[string]string{
						"gpu_id":   uuid,
						"data":     string(b),
						"encoding": "json",
					},
					SuggestedActions: nil,
				},
			)
			ccancel()
			if err != nil {
				d.err = fmt.Errorf("failed to insert event for remapping failed for %s: %w", uuid, err)
				continue
			}
		}

		if err := metrics.SetRemappedDueToUncorrectableErrors(c.ctx, uuid, uint32(remappedRows.RemappedDueToCorrectableErrors), d.ts); err != nil {
			d.err = fmt.Errorf("failed to set metrics for remapped due to uncorrectable errors for %s: %w", uuid, err)
			continue
		}
		if err := metrics.SetRemappingPending(c.ctx, uuid, remappedRows.RemappingPending, d.ts); err != nil {
			d.err = fmt.Errorf("failed to set metrics for remapping pending for %s: %w", uuid, err)
			continue
		}
		if err := metrics.SetRemappingFailed(c.ctx, uuid, remappedRows.RemappingFailed, d.ts); err != nil {
			d.err = fmt.Errorf("failed to set metrics for remapping failed for %s: %w", uuid, err)
			continue
		}
	}
}

type Data struct {
	// ProductName is the product name of the GPU.
	ProductName string `json:"product_name"`
	// MemoryErrorManagementCapabilities contains the memory error management capabilities of the GPU.
	MemoryErrorManagementCapabilities nvml.MemoryErrorManagementCapabilities `json:"memory_error_management_capabilities"`
	// RemappedRows maps from GPU UUID to the remapped rows data.
	RemappedRows map[string]nvml.RemappedRows `json:"remapped_rows"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
}

func (d *Data) getReason() string {
	if d == nil {
		return "no remapped rows data"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to get remapped rows data -- %s", d.err)
	}

	if !d.MemoryErrorManagementCapabilities.RowRemapping {
		return fmt.Sprintf("%q does not support row remapping", d.ProductName)
	}

	reasons := []string{}
	for uuid, remappedRows := range d.RemappedRows {
		if remappedRows.QualifiesForRMA() {
			reasons = append(reasons, fmt.Sprintf("%s qualifies for RMA (row remapping failed, remapped due to %d uncorrectable error(s))", uuid, remappedRows.RemappedDueToUncorrectableErrors))
		}
		if remappedRows.RequiresReset() {
			reasons = append(reasons, fmt.Sprintf("%s needs reset (detected pending row remapping)", uuid))
		}
	}
	if len(reasons) == 0 {
		return "no issue detected"
	}
	return strings.Join(reasons, "; ")
}

func (d *Data) getHealth() (string, bool) {
	healthy := d == nil || d.err == nil

	if d != nil && healthy && d.MemoryErrorManagementCapabilities.RowRemapping {
		for _, remappedRows := range d.RemappedRows {
			if remappedRows.QualifiesForRMA() {
				healthy = false
				break
			}
			if remappedRows.RequiresReset() {
				healthy = false
				break
			}
		}
	}

	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
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
		Name:   "row_remapping",
		Reason: d.getReason(),
		Error:  d.getError(),
	}
	state.Health, state.Healthy = d.getHealth()

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
