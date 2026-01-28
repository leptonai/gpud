// Package nvlink monitors the NVIDIA per-GPU nvlink devices.
package nvlink

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

const Name = "accelerator-nvidia-nvlink"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getTimeNowFunc func() time.Time

	nvmlInstance      nvidianvml.Instance
	getNVLinkFunc     func(uuid string, dev device.Device) (NVLink, error)
	getThresholdsFunc func() ExpectedLinkStates

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
		nvmlInstance:      gpudInstance.NVMLInstance,
		getNVLinkFunc:     GetNVLink,
		getThresholdsFunc: GetDefaultExpectedLinkStates,
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
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu nvlink")

	cr := &checkResult{
		ts: c.getTimeNowFunc(),
	}
	if c.getThresholdsFunc != nil {
		thresholds := c.getThresholdsFunc()
		cr.ExpectedLinkStates = &thresholds
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

	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		nvLink, err := c.getNVLinkFunc(uuid, dev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting nvlink"

			if errors.Is(err, nvmlerrors.ErrGPURequiresReset) {
				cr.reason = nvmlerrors.ErrGPURequiresReset.Error()
				cr.suggestedActions = &apiv1.SuggestedActions{
					Description: nvmlerrors.ErrGPURequiresReset.Error(),
					RepairActions: []apiv1.RepairActionType{
						apiv1.RepairActionTypeRebootSystem,
					},
				}
			}

			if errors.Is(err, nvmlerrors.ErrGPULost) {
				cr.reason = nvmlerrors.ErrGPULost.Error()
				cr.suggestedActions = &apiv1.SuggestedActions{
					Description: nvmlerrors.ErrGPULost.Error(),
					RepairActions: []apiv1.RepairActionType{
						apiv1.RepairActionTypeRebootSystem,
					},
				}
			}

			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}

		cr.NVLinks = append(cr.NVLinks, nvLink)

		labels := prometheus.Labels{"uuid": uuid}
		if nvLink.Supported {
			metricSupported.With(labels).Set(1.0)
		} else {
			metricSupported.With(labels).Set(0.0)
			cr.UnsupportedNVLinkUUIDs = append(cr.UnsupportedNVLinkUUIDs, uuid)
			continue
		}

		featureEnabled := len(nvLink.States) > 0 && nvLink.States.AllFeatureEnabled()
		if featureEnabled {
			metricFeatureEnabled.With(labels).Set(1.0)
			cr.ActiveNVLinkUUIDs = append(cr.ActiveNVLinkUUIDs, uuid)
		} else {
			metricFeatureEnabled.With(labels).Set(0.0)
			cr.InactiveNVLinkUUIDs = append(cr.InactiveNVLinkUUIDs, uuid)
		}
		metricReplayErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(nvLink.States.TotalReplayErrors()))
		metricRecoveryErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(nvLink.States.TotalRecoveryErrors()))
		metricCRCErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(nvLink.States.TotalCRCErrors()))
	}

	if len(cr.ActiveNVLinkUUIDs) > 0 {
		sort.Strings(cr.ActiveNVLinkUUIDs)
	}
	if len(cr.InactiveNVLinkUUIDs) > 0 {
		sort.Strings(cr.InactiveNVLinkUUIDs)
	}
	if len(cr.UnsupportedNVLinkUUIDs) > 0 {
		sort.Strings(cr.UnsupportedNVLinkUUIDs)
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("all %d GPU(s) were checked, no nvlink issue found", len(devs))

	evaluateHealthStateWithThresholds(cr)

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// NVLinks contains detailed NVLink information for all GPUs checked
	NVLinks []NVLink `json:"nvlinks,omitempty"`

	// ActiveNVLinkUUIDs lists GPUs where NVLink is supported AND all links have FeatureEnabled=true
	// (i.e., len(States) > 0 && States.AllFeatureEnabled() == true)
	// These GPUs have fully operational NVLink connectivity
	ActiveNVLinkUUIDs []string `json:"active_nvlink_uuids,omitempty"`

	// InactiveNVLinkUUIDs lists GPUs where NVLink is supported BUT at least one link has FeatureEnabled=false
	// This corresponds to nvidia-smi showing "all links are inActive"
	// Common causes: disabled via driver, fabric manager issues, NVSwitch connectivity problems
	InactiveNVLinkUUIDs []string `json:"inactive_nvlink_uuids,omitempty"`

	// UnsupportedNVLinkUUIDs lists GPUs that do not support NVLink at hardware/firmware level
	UnsupportedNVLinkUUIDs []string `json:"unsupported_nvlink_uuids,omitempty"`

	// ExpectedLinkStates defines the threshold for how many GPUs must have active NVLink
	// Used by evaluateHealthStateWithThresholds to determine if the system is healthy
	ExpectedLinkStates *ExpectedLinkStates `json:"expected_link_states,omitempty"`

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
	if len(cr.NVLinks) == 0 {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"GPU UUID", "GPU Bus ID", "NVLink Enabled", "NVLink Supported"})
	for _, nvlink := range cr.NVLinks {
		featureEnabled := nvlink.Supported && len(nvlink.States) > 0 && nvlink.States.AllFeatureEnabled()
		table.Append([]string{nvlink.UUID, nvlink.BusID, fmt.Sprintf("%t", featureEnabled), fmt.Sprintf("%t", nvlink.Supported)})
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

func (cr *checkResult) getSuggestedActions() *apiv1.SuggestedActions {
	if cr == nil {
		return nil
	}
	return cr.suggestedActions
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
		SuggestedActions: cr.getSuggestedActions(),
		Error:            cr.getError(),
		Health:           cr.health,
	}

	// propagate suggested actions to health state if present
	if cr.suggestedActions != nil {
		state.SuggestedActions = cr.suggestedActions
	}

	if len(cr.NVLinks) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
