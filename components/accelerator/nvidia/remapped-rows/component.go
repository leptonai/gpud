// Package remappedrows tracks the NVIDIA per-GPU remapped rows.
package remappedrows

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// Name is the ID of the remapped rows component.
const Name = "accelerator-nvidia-remapped-rows"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance        nvidianvml.InstanceV2
	getRemappedRowsFunc func(uuid string, dev device.Device) (nvidianvml.RemappedRows, error)

	eventBucket eventstore.Bucket

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		nvmlInstance:        gpudInstance.NVMLInstance,
		getRemappedRowsFunc: nvml.GetRemappedRows,
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}
	}

	return c, nil
}

func (c *component) Name() string { return Name }

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
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getLastHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if c.eventBucket == nil {
		return nil, nil
	}
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

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu remapped rows")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		d.reason = "NVIDIA NVML instance is nil"
		d.health = apiv1.StateTypeHealthy
		return d
	}
	if !c.nvmlInstance.NVMLExists() {
		d.reason = "NVIDIA NVML is not loaded"
		d.health = apiv1.StateTypeHealthy
		return d
	}

	d.ProductName = c.nvmlInstance.ProductName()
	d.MemoryErrorManagementCapabilities = c.nvmlInstance.GetMemoryErrorManagementCapabilities()

	if !d.MemoryErrorManagementCapabilities.RowRemapping {
		d.health = apiv1.StateTypeHealthy
		d.reason = fmt.Sprintf("%q does not support row remapping", d.ProductName)
		return d
	}

	issues := make([]string, 0)

	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		remappedRows, err := c.getRemappedRowsFunc(uuid, dev)
		if err != nil {
			log.Logger.Errorw("error getting remapped rows", "uuid", uuid, "error", err)

			d.err = err
			d.health = apiv1.StateTypeUnhealthy
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
		if c.eventBucket != nil && remappedRows.RemappingPending {
			log.Logger.Warnw("inserting event for remapping pending", "uuid", uuid)

			cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
			err = c.eventBucket.Insert(
				cctx,
				apiv1.Event{
					Time:    metav1.Time{Time: d.ts},
					Name:    "row_remapping_pending",
					Type:    apiv1.EventTypeWarning,
					Message: fmt.Sprintf("%s detected pending row remapping", uuid),
					DeprecatedExtraInfo: map[string]string{
						"gpu_id":   uuid,
						"data":     string(b),
						"encoding": "json",
					},
					DeprecatedSuggestedActions: nil,
				},
			)
			ccancel()
			if err != nil {
				log.Logger.Errorw("error inserting event for remapping pending", "uuid", uuid, "error", err)
				d.err = err
				d.health = apiv1.StateTypeUnhealthy
				d.reason = fmt.Sprintf("error inserting event for remapping pending for %s", uuid)
				continue
			}
		}

		if c.eventBucket != nil && remappedRows.RemappingFailed {
			log.Logger.Warnw("inserting event for remapping failed", "uuid", uuid)

			cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
			err = c.eventBucket.Insert(
				cctx,
				apiv1.Event{
					Time:    metav1.Time{Time: d.ts},
					Name:    "row_remapping_failed",
					Type:    apiv1.EventTypeWarning,
					Message: fmt.Sprintf("%s detected failed row remapping", uuid),
					DeprecatedExtraInfo: map[string]string{
						"gpu_id":   uuid,
						"data":     string(b),
						"encoding": "json",
					},
					DeprecatedSuggestedActions: nil,
				},
			)
			ccancel()
			if err != nil {
				log.Logger.Errorw("error inserting event for remapping failed", "uuid", uuid, "error", err)
				d.err = err
				d.health = apiv1.StateTypeUnhealthy
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
		d.health = apiv1.StateTypeUnhealthy
		d.reason = strings.Join(issues, ", ")
	} else {
		d.health = apiv1.StateTypeHealthy
		d.reason = fmt.Sprintf("%d devices support remapped rows and found no issue", len(devs))
	}

	return d
}

var _ components.CheckResult = &Data{}

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
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (d *Data) String() string {
	if d == nil {
		return ""
	}
	if len(d.RemappedRows) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"GPU UUID", "Remapped due to correctable errors", "Remapped due to uncorrectable errors", "Remapping pending", "Remapping failed"})
	for _, remappedRows := range d.RemappedRows {
		table.Append([]string{
			remappedRows.UUID,
			fmt.Sprintf("%d", remappedRows.RemappedDueToCorrectableErrors),
			fmt.Sprintf("%d", remappedRows.RemappedDueToUncorrectableErrors),
			fmt.Sprintf("%v", remappedRows.RemappingPending),
			fmt.Sprintf("%v", remappedRows.RemappingFailed),
		})
	}
	table.Render()

	return buf.String()
}

func (d *Data) Summary() string {
	if d == nil {
		return ""
	}
	return d.reason
}

func (d *Data) HealthState() apiv1.HealthStateType {
	if d == nil {
		return ""
	}
	return d.health
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getLastHealthStates() apiv1.HealthStates {
	if d == nil {
		return apiv1.HealthStates{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),
		Health: d.health,
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
