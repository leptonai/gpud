// Package nvlink monitors the NVIDIA per-GPU nvlink devices.
package nvlink

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
)

const Name = "accelerator-nvidia-nvlink"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance      nvidianvml.Instance
	getNVLinkFunc     func(uuid string, dev device.Device) (nvidianvml.NVLink, error)
	getThresholdsFunc func() ExpectedLinkStates

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:               cctx,
		cancel:            ccancel,
		nvmlInstance:      gpudInstance.NVMLInstance,
		getNVLinkFunc:     nvidianvml.GetNVLink,
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
		ts: time.Now().UTC(),
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
		}

		featureEnabled := nvLink.Supported && len(nvLink.States) > 0 && nvLink.States.AllFeatureEnabled()
		if featureEnabled {
			metricFeatureEnabled.With(labels).Set(1.0)
		} else {
			metricFeatureEnabled.With(labels).Set(0.0)
			if nvLink.Supported {
				cr.InactiveNVLinkUUIDs = append(cr.InactiveNVLinkUUIDs, uuid)
			}
		}
		metricReplayErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(nvLink.States.TotalReplayErrors()))
		metricRecoveryErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(nvLink.States.TotalRecoveryErrors()))
		metricCRCErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(nvLink.States.TotalCRCErrors()))
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

	if cr.health == apiv1.HealthStateTypeUnhealthy && cr.reason == "" {
		details := []string{}
		if len(cr.InactiveNVLinkUUIDs) > 0 {
			details = append(details, fmt.Sprintf("inactive=%s", strings.Join(cr.InactiveNVLinkUUIDs, ",")))
		}
		if len(cr.UnsupportedNVLinkUUIDs) > 0 {
			details = append(details, fmt.Sprintf("unsupported=%s", strings.Join(cr.UnsupportedNVLinkUUIDs, ",")))
		}
		cr.reason = fmt.Sprintf("nvlink issue detected%s", func() string {
			if len(details) == 0 {
				return ""
			}
			return fmt.Sprintf(" (%s)", strings.Join(details, "; "))
		}())
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	NVLinks                []nvidianvml.NVLink `json:"nvlinks,omitempty"`
	InactiveNVLinkUUIDs    []string            `json:"inactive_nvlink_uuids,omitempty"`
	UnsupportedNVLinkUUIDs []string            `json:"unsupported_nvlink_uuids,omitempty"`
	ExpectedLinkStates     *ExpectedLinkStates `json:"expected_link_states,omitempty"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
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

	if len(cr.NVLinks) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
