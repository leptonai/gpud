// Package ecc tracks the NVIDIA per-GPU ECC errors and other ECC related information.
package ecc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/prometheus/client_golang/prometheus"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-ecc"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstanceV2        nvml.InstanceV2
	getECCModeEnabledFunc func(uuid string, dev device.Device) (nvidianvml.ECCMode, error)
	getECCErrorsFunc      func(uuid string, dev device.Device, eccModeEnabledCurrent bool) (nvidianvml.ECCErrors, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, nvmlInstanceV2 nvml.InstanceV2) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstanceV2:        nvmlInstanceV2,
		getECCModeEnabledFunc: nvidianvml.GetECCModeEnabled,
		getECCErrorsFunc:      nvidianvml.GetECCErrors,
	}
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

func (c *component) HealthStates(ctx context.Context) (apiv1.HealthStates, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking ecc")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	devs := c.nvmlInstanceV2.Devices()
	for uuid, dev := range devs {
		eccMode, err := c.getECCModeEnabledFunc(uuid, dev)
		if err != nil {
			log.Logger.Errorw("error getting ECC mode for device", "uuid", uuid, "error", err)
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting ECC mode for device %s", uuid)
			return
		}
		d.ECCModes = append(d.ECCModes, eccMode)

		eccErrors, err := c.getECCErrorsFunc(uuid, dev, eccMode.EnabledCurrent)
		if err != nil {
			log.Logger.Errorw("error getting ECC errors for device", "uuid", uuid, "error", err)
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting ECC errors for device %s", uuid)
			return
		}
		d.ECCErrors = append(d.ECCErrors, eccErrors)

		metricAggregateTotalCorrected.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(eccErrors.Aggregate.Total.Corrected))
		metricAggregateTotalUncorrected.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(eccErrors.Aggregate.Total.Uncorrected))
		metricVolatileTotalCorrected.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(eccErrors.Volatile.Total.Corrected))
		metricVolatileTotalUncorrected.With(prometheus.Labels{pkgmetrics.MetricLabelKey: uuid}).Set(float64(eccErrors.Volatile.Total.Uncorrected))
	}

	d.healthy = true
	d.reason = fmt.Sprintf("all %d GPU(s) were checked, no ECC issue found", len(devs))
}

type Data struct {
	ECCModes  []nvidianvml.ECCMode   `json:"ecc_modes,omitempty"`
	ECCErrors []nvidianvml.ECCErrors `json:"ecc_errors,omitempty"`

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

func (d *Data) getHealthStates() (apiv1.HealthStates, error) {
	if d == nil {
		return []apiv1.HealthState{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "no data yet",
			},
		}, nil
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Health: apiv1.StateTypeHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateTypeUnhealthy
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.HealthState{state}, nil
}
