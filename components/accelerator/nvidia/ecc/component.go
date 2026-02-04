// Package ecc tracks the NVIDIA per-GPU ECC errors and other ECC related information.
package ecc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

const Name = "accelerator-nvidia-ecc"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getTimeNowFunc func() time.Time

	nvmlInstance          nvidianvml.Instance
	getECCModeEnabledFunc func(uuid string, dev device.Device) (ECCMode, error)
	getECCErrorsFunc      func(uuid string, dev device.Device, eccModeEnabledCurrent bool) (ECCErrors, error)

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
		nvmlInstance:          gpudInstance.NVMLInstance,
		getECCModeEnabledFunc: GetECCModeEnabled,
		getECCErrorsFunc:      GetECCErrors,
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
	log.Logger.Infow("checking nvidia gpu ecc")

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

	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		eccMode, err := c.getECCModeEnabledFunc(uuid, dev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting ECC mode"

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

			log.Logger.Warnw(cr.reason, "uuid", uuid, "error", cr.err)
			return cr
		}
		cr.ECCModes = append(cr.ECCModes, eccMode)

		eccErrors, err := c.getECCErrorsFunc(uuid, dev, eccMode.EnabledCurrent)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting ECC errors"

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

			log.Logger.Warnw(cr.reason, "uuid", uuid, "error", cr.err)
			return cr
		}
		cr.ECCErrors = append(cr.ECCErrors, eccErrors)

		metricAggregateTotalCorrected.With(prometheus.Labels{"uuid": uuid}).Set(float64(eccErrors.Aggregate.Total.Corrected))
		metricAggregateTotalUncorrected.With(prometheus.Labels{"uuid": uuid}).Set(float64(eccErrors.Aggregate.Total.Uncorrected))
		metricVolatileTotalCorrected.With(prometheus.Labels{"uuid": uuid}).Set(float64(eccErrors.Volatile.Total.Corrected))
		metricVolatileTotalUncorrected.With(prometheus.Labels{"uuid": uuid}).Set(float64(eccErrors.Volatile.Total.Uncorrected))
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("all %d GPU(s) were checked, no ECC issue found", len(devs))

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ECCModes  []ECCMode   `json:"ecc_modes,omitempty"`
	ECCErrors []ECCErrors `json:"ecc_errors,omitempty"`

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
	if len(cr.ECCModes) == 0 {
		return "no data"
	}

	buf1 := bytes.NewBuffer(nil)
	table1 := tablewriter.NewWriter(buf1)
	table1.SetAlignment(tablewriter.ALIGN_CENTER)
	table1.SetHeader([]string{"GPU UUID", "GPU Bus ID", "Enabled Current", "Enabled Pending", "Supported"})
	for _, eccMode := range cr.ECCModes {
		table1.Append([]string{
			eccMode.UUID,
			eccMode.BusID,
			fmt.Sprintf("%t", eccMode.EnabledCurrent),
			fmt.Sprintf("%t", eccMode.EnabledPending),
			fmt.Sprintf("%t", eccMode.Supported),
		})
	}
	table1.Render()

	buf2 := bytes.NewBuffer(nil)
	table2 := tablewriter.NewWriter(buf2)
	table2.SetHeader([]string{"GPU UUID", "GPU Bus ID", "Aggregate Total Corrected", "Aggregate Total Uncorrected", "Volatile Total Corrected", "Volatile Total Uncorrected"})
	for _, eccErrors := range cr.ECCErrors {
		table2.Append([]string{
			eccErrors.UUID,
			eccErrors.BusID,
			fmt.Sprintf("%d", eccErrors.Aggregate.Total.Corrected),
			fmt.Sprintf("%d", eccErrors.Aggregate.Total.Uncorrected),
			fmt.Sprintf("%d", eccErrors.Volatile.Total.Corrected),
			fmt.Sprintf("%d", eccErrors.Volatile.Total.Uncorrected),
		})
	}
	table2.Render()

	return buf1.String() + "\n" + buf2.String()
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
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	// propagate suggested actions to health state if present
	if cr.suggestedActions != nil {
		state.SuggestedActions = cr.suggestedActions
	}

	if len(cr.ECCModes) > 0 && len(cr.ECCErrors) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
