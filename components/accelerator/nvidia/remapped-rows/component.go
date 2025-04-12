// Package remappedrows tracks the NVIDIA per-GPU remapped rows.
package remappedrows

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// Name is the ID of the remapped rows component.
const Name = "accelerator-nvidia-remapped-rows"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstanceV2      nvml.InstanceV2
	getRemappedRowsFunc func(uuid string, dev device.Device) (nvml.RemappedRows, error)

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

		nvmlInstanceV2:      nvmlInstanceV2,
		getRemappedRowsFunc: nvml.GetRemappedRows,

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

	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking remapped rows")
	d := Data{
		ProductName:                       c.nvmlInstanceV2.ProductName(),
		MemoryErrorManagementCapabilities: c.nvmlInstanceV2.GetMemoryErrorManagementCapabilities(),
		RemappedRows:                      nil,

		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	if !d.MemoryErrorManagementCapabilities.RowRemapping {
		d.healthy = true
		d.reason = fmt.Sprintf("%q does not support row remapping", d.ProductName)
		return
	}

	issues := make([]string, 0)

	devs := c.nvmlInstanceV2.Devices()
	for uuid, dev := range devs {
		remappedRows, err := c.getRemappedRowsFunc(uuid, dev)
		if err != nil {
			log.Logger.Errorw("error getting remapped rows", "uuid", uuid, "error", err)
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting remapped rows for %s", uuid)
			continue
		}
		d.RemappedRows = append(d.RemappedRows, remappedRows)

		metricUncorrectableErrors.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(remappedRows.RemappedDueToCorrectableErrors))

		if remappedRows.RemappingPending {
			metricRemappingPending.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(1.0))
		} else {
			metricRemappingPending.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(0.0))
		}

		if remappedRows.RemappingFailed {
			metricRemappingFailed.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(1.0))
		} else {
			metricRemappingFailed.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(0.0))
		}

		b, _ := json.Marshal(d)
		if remappedRows.RemappingPending {
			log.Logger.Warnw("inserting event for remapping pending", "uuid", uuid)

			cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
			err = c.eventBucket.Insert(
				cctx,
				apiv1.Event{
					Time:    metav1.Time{Time: d.ts},
					Name:    "row_remapping_pending",
					Type:    apiv1.EventTypeWarning,
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
				log.Logger.Errorw("error inserting event for remapping pending", "uuid", uuid, "error", err)
				d.err = err
				d.healthy = false
				d.reason = fmt.Sprintf("error inserting event for remapping pending for %s", uuid)
				continue
			}
		}

		if remappedRows.RemappingFailed {
			log.Logger.Warnw("inserting event for remapping failed", "uuid", uuid)

			cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
			err = c.eventBucket.Insert(
				cctx,
				apiv1.Event{
					Time:    metav1.Time{Time: d.ts},
					Name:    "row_remapping_failed",
					Type:    apiv1.EventTypeWarning,
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
				log.Logger.Errorw("error inserting event for remapping failed", "uuid", uuid, "error", err)
				d.err = err
				d.healthy = false
				d.reason = fmt.Sprintf("error inserting event for remapping failed for %s", uuid)
				continue
			}
		}

		if remappedRows.QualifiesForRMA() {
			issues = append(issues, fmt.Sprintf("%s qualifies for RMA (row remapping failed, remapped due to %d uncorrectable error(s))", uuid, remappedRows.RemappedDueToUncorrectableErrors))
		}
		if remappedRows.RequiresReset() {
			issues = append(issues, fmt.Sprintf("%s needs reset (detected pending row remapping)", uuid))
		}
	}

	if len(issues) > 0 {
		d.healthy = false
		d.reason = strings.Join(issues, ", ")
	} else {
		d.healthy = true
		d.reason = fmt.Sprintf("%d devices support remapped rows and found no issue", len(devs))
	}
}

type Data struct {
	// ProductName is the product name of the GPU.
	ProductName string `json:"product_name"`
	// MemoryErrorManagementCapabilities contains the memory error management capabilities of the GPU.
	MemoryErrorManagementCapabilities nvml.MemoryErrorManagementCapabilities `json:"memory_error_management_capabilities"`
	// RemappedRows maps from GPU UUID to the remapped rows data.
	RemappedRows []nvml.RemappedRows `json:"remapped_rows,omitempty"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	healthy bool
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
				Name:    Name,
				Health:  apiv1.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := apiv1.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  apiv1.StateHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.State{state}, nil
}
