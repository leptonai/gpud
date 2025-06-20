// Package gpucounts monitors the GPU count of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package gpucounts

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	nvidiaquery "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/olekukonko/tablewriter"
)

const Name = "accelerator-nvidia-gpu-counts"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance

	getCountLspci     func(ctx context.Context) (int, error)
	getThresholdsFunc func() ExpectedGPUCounts

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: gpudInstance.NVMLInstance,
		getCountLspci: func(ctx context.Context) (int, error) {
			devs, err := nvidiaquery.ListPCIGPUs(ctx)
			if err != nil {
				return 0, err
			}
			return len(devs), nil
		},
		getThresholdsFunc: GetDefaultExpectedGPUCounts,
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
	log.Logger.Infow("checking nvidia gpu counts")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}

	thresholds := c.getThresholdsFunc()
	defer func() {
		// nothing specified for this machine, gpud MUST skip the gpu count check
		// do this in defer in order to check gpu count in gpud scan
		// where it does not set the thresholds
		if thresholds.IsZero() {
			cr.reason = reasonThresholdNotSetSkipped
			cr.health = apiv1.HealthStateTypeHealthy
		}

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
	cr.CountNVML = len(c.nvmlInstance.Devices())

	if c.getCountLspci != nil {
		cr.CountLspci, cr.err = c.getCountLspci(c.ctx)
		if cr.err != nil {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting count of lspci"
			log.Logger.Errorw(cr.reason, "error", cr.err)
			return cr
		}
	}

	if !thresholds.IsZero() {
		if cr.CountLspci != thresholds.Count {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = fmt.Sprintf("nvidia gpu count mismatch (lspci %d, expected %d)", cr.CountLspci, thresholds.Count)
			log.Logger.Errorw(cr.reason, "count_lspci", cr.CountLspci, "count_nvml", cr.CountNVML, "expected", thresholds.Count)
			return cr
		}
		if cr.CountNVML != thresholds.Count {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = fmt.Sprintf("nvidia gpu count mismatch (nvml %d, expected %d)", cr.CountNVML, thresholds.Count)
			log.Logger.Errorw(cr.reason, "count_lspci", cr.CountLspci, "count_nvml", cr.CountNVML, "expected", thresholds.Count)
			return cr
		}
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("nvidia gpu count matching thresholds (%d)", thresholds.Count)
		return cr
	}

	return cr
}

const reasonThresholdNotSetSkipped = "GPU count thresholds not set, skipping"

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ProductName string `json:"product_name"`
	CountLspci  int    `json:"count_lspci"`
	CountNVML   int    `json:"count_nvml"`

	// timestamp of the last check
	ts time.Time
	// error from the last check with "ibstat" command and other operations
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
	if cr.ProductName == "" {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"Product Name", cr.ProductName})
	table.Append([]string{"Count lspci", fmt.Sprintf("%d", cr.CountLspci)})
	table.Append([]string{"Count NVML", fmt.Sprintf("%d", cr.CountNVML)})
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
	if cr == nil {
		return ""
	}
	if cr.err != nil {
		return cr.err.Error()
	}
	return ""
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
		Health:           cr.health,
		Reason:           cr.reason,
		Error:            cr.getError(),
		SuggestedActions: cr.getSuggestedActions(),
		ExtraInfo: map[string]string{
			"product_name": cr.ProductName,
			"count_lspci":  fmt.Sprintf("%d", cr.CountLspci),
			"count_nvml":   fmt.Sprintf("%d", cr.CountNVML),
		},
	}

	return apiv1.HealthStates{state}
}
