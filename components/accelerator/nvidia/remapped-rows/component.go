// Package remappedrows tracks the NVIDIA per-GPU remapped rows.
package remappedrows

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
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
)

// Name is the ID of the remapped rows component.
const Name = "accelerator-nvidia-remapped-rows"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance        nvidianvml.Instance
	getRemappedRowsFunc func(uuid string, dev device.Device) (nvidianvml.RemappedRows, error)

	suggestedActionsStore components.SuggestedActionsStore

	eventBucket eventstore.Bucket

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:                   cctx,
		cancel:                ccancel,
		nvmlInstance:          gpudInstance.NVMLInstance,
		getRemappedRowsFunc:   nvml.GetRemappedRows,
		suggestedActionsStore: gpudInstance.SuggestedActionsStore,
	}

	if gpudInstance.EventStore != nil {
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

	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu remapped rows")

	cr := &checkResult{
		ts: time.Now().UTC(),
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

	cr.ProductName = c.nvmlInstance.ProductName()
	cr.MemoryErrorManagementCapabilities = c.nvmlInstance.GetMemoryErrorManagementCapabilities()

	if !cr.MemoryErrorManagementCapabilities.RowRemapping {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("%q does not support row remapping", cr.ProductName)
		return cr
	}

	issues := make([]string, 0)

	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		remappedRows, err := c.getRemappedRowsFunc(uuid, dev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting remapped rows"
			log.Logger.Warnw(cr.reason, "uuid", uuid, "pciBusID", dev.PCIBusID(), "error", cr.err)
			continue
		}
		cr.RemappedRows = append(cr.RemappedRows, remappedRows)

		metricUncorrectableErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(remappedRows.RemappedDueToCorrectableErrors))

		if remappedRows.RemappingPending {
			metricRemappingPending.With(prometheus.Labels{"uuid": uuid}).Set(float64(1.0))
		} else {
			metricRemappingPending.With(prometheus.Labels{"uuid": uuid}).Set(float64(0.0))
		}

		if remappedRows.RemappingFailed {
			metricRemappingFailed.With(prometheus.Labels{"uuid": uuid}).Set(float64(1.0))
		} else {
			metricRemappingFailed.With(prometheus.Labels{"uuid": uuid}).Set(float64(0.0))
		}

		if c.eventBucket != nil && remappedRows.RemappingPending {
			log.Logger.Warnw("inserting event for remapping pending", "uuid", uuid)
			cr.suggestedActions = &apiv1.SuggestedActions{
				Description: "row remapping pending requires GPU reset or system reboot",
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			}
			if c.suggestedActionsStore != nil {
				components := c.suggestedActionsStore.HasSuggested(apiv1.RepairActionTypeHardwareInspection)
				if len(components) > 0 {
					log.Logger.Infow("row remapping pending with reboot suggested but other components suggested hardware inspection", "hwSuggestingComponents", components)
					cr.suggestedActions.Description = fmt.Sprintf("row remapping pending but suggest hardware inspection (detected by %s)", strings.Join(components, ", "))
					cr.suggestedActions.RepairActions = []apiv1.RepairActionType{
						apiv1.RepairActionTypeHardwareInspection,
					}
				}
			}

			cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
			cr.err = c.eventBucket.Insert(
				cctx,
				eventstore.Event{
					Time:    cr.ts,
					Name:    "row_remapping_pending",
					Type:    string(apiv1.EventTypeWarning),
					Message: fmt.Sprintf("%s detected pending row remapping", uuid),
				},
			)
			ccancel()
			if cr.err != nil {
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error inserting event for remapping pending"
				log.Logger.Warnw(cr.reason, "uuid", uuid, "pciBusID", dev.PCIBusID(), "error", cr.err)
			}
		}

		if c.eventBucket != nil && remappedRows.RemappingFailed {
			log.Logger.Warnw("inserting event for remapping failed", "uuid", uuid)
			cr.suggestedActions = &apiv1.SuggestedActions{
				Description: "row remapping failure requires hardware inspection",
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeHardwareInspection,
				},
			}
			if c.suggestedActionsStore != nil {
				c.suggestedActionsStore.Suggest(Name, apiv1.RepairActionTypeHardwareInspection, 10*time.Minute)
			}

			cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
			cr.err = c.eventBucket.Insert(
				cctx,
				eventstore.Event{
					Time:    cr.ts,
					Name:    "row_remapping_failed",
					Type:    string(apiv1.EventTypeWarning),
					Message: fmt.Sprintf("%s detected failed row remapping", uuid),
				},
			)
			ccancel()
			if cr.err != nil {
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error inserting event for remapping failed"
				log.Logger.Warnw(cr.reason, "uuid", uuid, "pciBusID", dev.PCIBusID(), "error", cr.err)
			}
		}

		if remappedRows.QualifiesForRMA() {
			issues = append(issues, fmt.Sprintf("%s qualifies for RMA (row remapping failed, remapped due to %d uncorrectable error(s))", dev.PCIBusID(), remappedRows.RemappedDueToUncorrectableErrors))
		}
		if remappedRows.RequiresReset() {
			issues = append(issues, fmt.Sprintf("%s needs reset (detected pending row remapping)", dev.PCIBusID()))
		}
	}

	if len(issues) > 0 {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = strings.Join(issues, ", ")
	} else {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("%d devices support remapped rows and found no issue", len(devs))
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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

	// suggested actions
	suggestedActions *apiv1.SuggestedActions
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if len(cr.RemappedRows) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"GPU UUID", "GPU Bus ID", "Remapped due to correctable errors", "Remapped due to uncorrectable errors", "Remapping pending", "Remapping failed"})
	for _, remappedRows := range cr.RemappedRows {
		table.Append([]string{
			remappedRows.UUID,
			remappedRows.BusID,
			fmt.Sprintf("%d", remappedRows.RemappedDueToCorrectableErrors),
			fmt.Sprintf("%d", remappedRows.RemappedDueToUncorrectableErrors),
			fmt.Sprintf("%v", remappedRows.RemappingPending),
			fmt.Sprintf("%v", remappedRows.RemappingFailed),
		})
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

	if len(cr.RemappedRows) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
