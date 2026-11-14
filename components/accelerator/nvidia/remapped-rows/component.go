// Package remappedrows tracks the NVIDIA per-GPU remapped rows.
package remappedrows

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// Name is the ID of the remapped rows component.
const Name = "accelerator-nvidia-remapped-rows"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getTimeNowFunc func() time.Time

	nvmlInstance        nvidianvml.Instance
	getRemappedRowsFunc func(uuid string, dev device.Device) (RemappedRows, error)
	// getMemoryRepairStatusFunc reads bank-remapper and repair flags (Rubin only).
	getMemoryRepairStatusFunc func(uuid string, dev device.Device) (MemoryRepairStatus, error)

	gpuUUIDsWithRowRemappingPending map[string]any
	gpuUUIDsWithRowRemappingFailed  map[string]any

	eventBucket eventstore.Bucket

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

// New returns the NVIDIA remapped rows component.
func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance:                    gpudInstance.NVMLInstance,
		getRemappedRowsFunc:             GetRemappedRows,
		getMemoryRepairStatusFunc:       GetMemoryRepairStatus,
		gpuUUIDsWithRowRemappingPending: make(map[string]any),
		gpuUUIDsWithRowRemappingFailed:  make(map[string]any),
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
		for _, uuid := range gpudInstance.FailureInjector.GPUUUIDsWithRowRemappingPending {
			c.gpuUUIDsWithRowRemappingPending[uuid] = nil
		}
		for _, uuid := range gpudInstance.FailureInjector.GPUUUIDsWithRowRemappingFailed {
			c.gpuUUIDsWithRowRemappingFailed[uuid] = nil
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
	// Check for NVML initialization errors first.
	// This handles cases like "error getting device handle for index 'N': Unknown Error"
	// which corresponds to nvidia-smi showing "Unable to determine the device handle for GPU".
	if err := c.nvmlInstance.InitError(); err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("NVML initialization error: %v", err)
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
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

	// Bank remapping, channel/TPC repair and the unrepairable-memory flag are
	// part of Rubin qualification, where nvidia-smi documents that inactive row
	// remappings are expected because bank remapping or channel repair restored
	// availability. Older families keep their established row-remapping-only
	// behavior until these states are qualified on them separately.
	queryMemoryRepair := nvidiaproduct.IsRubin(c.nvmlInstance.ProductName(), c.nvmlInstance.Architecture())
	rowQueryFailed, repairQueryFailed := false, false

	devs := c.nvmlInstance.Devices()
	uuids := make([]string, 0, len(devs))
	for uuid := range devs {
		uuids = append(uuids, uuid)
	}
	sort.Strings(uuids)
	for _, uuid := range uuids {
		dev := devs[uuid]
		// Query repair state before row remapping so a row-query failure does
		// not hide an independent pending repair or unrepairable-memory flag.
		if queryMemoryRepair {
			issues = append(issues, c.checkMemoryRepair(cr, uuid, dev, &repairQueryFailed)...)
		}

		remappedRows, err := c.getRemappedRowsFunc(uuid, dev)
		if err != nil {
			rowQueryFailed = true
			cr.err = errors.Join(cr.err, err)
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting remapped rows"
			log.Logger.Warnw(cr.reason, "uuid", uuid, "pciBusID", dev.PCIBusID(), "error", cr.err)
			continue
		}

		metricUncorrectableErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(remappedRows.RemappedDueToCorrectableErrors))

		if remappedRows.RemappingPending {
			metricRemappingPending.With(prometheus.Labels{"uuid": uuid}).Set(float64(1.0))
		} else {
			if _, ok := c.gpuUUIDsWithRowRemappingPending[uuid]; ok {
				log.Logger.Warnw("marking row remapping pending to inject failures", "uuid", uuid)
				remappedRows.RemappingPending = true
			} else {
				log.Logger.Debugw("row remapping pending", "uuid", uuid)
			}
			metricRemappingPending.With(prometheus.Labels{"uuid": uuid}).Set(float64(0.0))
		}

		if remappedRows.RemappingFailed {
			metricRemappingFailed.With(prometheus.Labels{"uuid": uuid}).Set(float64(1.0))
		} else {
			if _, ok := c.gpuUUIDsWithRowRemappingFailed[uuid]; ok {
				log.Logger.Warnw("marking row remapping failed to inject failures", "uuid", uuid)
				remappedRows.RemappingFailed = true
			} else {
				log.Logger.Debugw("row remapping failed", "uuid", uuid)
			}
			metricRemappingFailed.With(prometheus.Labels{"uuid": uuid}).Set(float64(0.0))
		}

		cr.RemappedRows = append(cr.RemappedRows, remappedRows)

		if remappedRows.RemappingPending {
			log.Logger.Warnw("suggesting reboot for row remapping pending", "uuid", uuid)
			cr.suggestReboot("row remapping pending requires GPU reset or system reboot")

			// no need to track these as events
			// because NVML simply returns remapping pending
			// whenever there's a pending remapping
		}

		if remappedRows.RemappingFailed {
			log.Logger.Warnw("suggesting hw inspection for row remapping failed", "uuid", uuid)
			cr.suggestHardwareInspection("row remapping failure requires hardware inspection")

			// no need to track these as events
			// because NVML simply returns remapping pending
			// whenever there's a pending remapping
		}

		if remappedRows.QualifiesForRMA() {
			issues = append(issues, fmt.Sprintf("%s qualifies for RMA (row remapping failed, remapped due to %d uncorrectable error(s))", dev.PCIBusID(), remappedRows.RemappedDueToUncorrectableErrors))
		}
		if remappedRows.RequiresReset() {
			issues = append(issues, fmt.Sprintf("%s needs reset (detected pending row remapping)", dev.PCIBusID()))
		}
	}

	if cr.err != nil {
		switch {
		case rowQueryFailed && repairQueryFailed:
			issues = append(issues, "error getting remapped rows and memory repair status")
		case repairQueryFailed:
			issues = append(issues, "error getting memory repair status")
		default:
			issues = append(issues, "error getting remapped rows")
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

// checkMemoryRepair records one GPU's Rubin memory-repair state on cr and
// returns the issues it found.
func (c *component) checkMemoryRepair(cr *checkResult, uuid string, dev device.Device, queryFailed *bool) []string {
	st, err := c.getMemoryRepairStatusFunc(uuid, dev)
	if err != nil {
		// Partial telemetry must not read as healthy: join the error so the
		// check is Unhealthy, but still evaluate the flags that were read.
		*queryFailed = true
		cr.err = errors.Join(cr.err, err)
		log.Logger.Warnw("error getting memory repair status", "uuid", uuid, "pciBusID", dev.PCIBusID(), "error", err)
	}
	if err == nil || st.BankRemapperSupported || st.RepairStatusSupported || st.UnrepairableMemorySupported {
		cr.MemoryRepairStatuses = append(cr.MemoryRepairStatuses, st)
	}

	var issues []string
	if st.RepairPending() {
		// nvidia-smi: "If one of the repair flags is pending, check the GPU
		// Recovery action"; like a pending row remap, a GPU reset applies the
		// repair. GPUd has no GPU-reset action and already suggests a reboot for
		// pending row remaps, so use the same action here.
		log.Logger.Warnw("suggesting reboot for pending memory repair", "uuid", uuid,
			"bankRemappingPending", st.BankRemappingPending, "channelRepairPending", st.ChannelRepairPending, "tpcRepairPending", st.TPCRepairPending)
		cr.suggestReboot("pending bank remapping or channel/TPC repair requires GPU reset or system reboot")
		issues = append(issues, fmt.Sprintf("%s needs reset (detected pending bank remapping or channel/TPC repair)", dev.PCIBusID()))
	}
	if st.UnrepairableMemory {
		// NVML reports memory it can no longer repair in the field; a reset
		// cannot help, so this outranks the reboot for any pending repair.
		log.Logger.Warnw("suggesting hw inspection for unrepairable memory", "uuid", uuid)
		cr.suggestHardwareInspection("unrepairable memory requires hardware inspection")
		issues = append(issues, fmt.Sprintf("%s has unrepairable memory", dev.PCIBusID()))
	}
	// BankRemapperNoSpareGroups > 0 is only reported: exhausted spares in some
	// groups do not by themselves mean a repair failed; NVML signals that
	// through the unrepairable-memory flag.
	return issues
}

// suggestReboot sets a reboot action unless hardware inspection was already
// suggested, since an RMA-class finding must not be downgraded.
func (cr *checkResult) suggestReboot(description string) {
	if cr.suggestedActions != nil && cr.suggestedActions.RepairActions[0] == apiv1.RepairActionTypeHardwareInspection {
		return
	}
	cr.suggestedActions = &apiv1.SuggestedActions{
		Description:   description,
		RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem},
	}
}

// suggestHardwareInspection always wins over other actions (RMA takes precedence).
func (cr *checkResult) suggestHardwareInspection(description string) {
	cr.suggestedActions = &apiv1.SuggestedActions{
		Description:   description,
		RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
	}
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// ProductName is the product name of the GPU.
	ProductName string `json:"product_name"`
	// MemoryErrorManagementCapabilities contains the memory error management capabilities of the GPU.
	MemoryErrorManagementCapabilities nvidiaproduct.MemoryErrorManagementCapabilities `json:"memory_error_management_capabilities"`
	// RemappedRows maps from GPU UUID to the remapped rows data.
	RemappedRows []RemappedRows `json:"remapped_rows,omitempty"`
	// MemoryRepairStatuses holds per-GPU bank-remapper and repair flags (Rubin only).
	MemoryRepairStatuses []MemoryRepairStatus `json:"memory_repair_statuses,omitempty"`

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
	if len(cr.RemappedRows) == 0 && len(cr.MemoryRepairStatuses) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	if len(cr.RemappedRows) > 0 {
		cr.renderRemappedRows(buf)
	}
	if len(cr.MemoryRepairStatuses) > 0 {
		table := tablewriter.NewWriter(buf)
		table.SetAlignment(tablewriter.ALIGN_CENTER)
		table.SetHeader([]string{"GPU UUID", "GPU Bus ID", "Active bank remappings", "Inactive bank remappings", "Bank remapping pending", "No-spare bank groups", "Channel repair pending", "TPC repair pending", "Unrepairable memory"})
		for _, st := range cr.MemoryRepairStatuses {
			table.Append([]string{
				st.UUID,
				st.BusID,
				fmt.Sprintf("%d", st.ActiveBankRemappings),
				fmt.Sprintf("%d", st.InactiveBankRemappings),
				fmt.Sprintf("%v", st.BankRemappingPending),
				fmt.Sprintf("%d", st.BankRemapperNoSpareGroups),
				fmt.Sprintf("%v", st.ChannelRepairPending),
				fmt.Sprintf("%v", st.TPCRepairPending),
				fmt.Sprintf("%v", st.UnrepairableMemory),
			})
		}
		table.Render()
	}

	return buf.String()
}

func (cr *checkResult) renderRemappedRows(buf *bytes.Buffer) {
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

	if len(cr.RemappedRows) > 0 || len(cr.MemoryRepairStatuses) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
