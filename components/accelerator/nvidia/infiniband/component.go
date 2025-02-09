// Package infiniband monitors the infiniband status of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package infiniband

import (
	"context"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_infiniband_id "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/id"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query/infiniband"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/log"
)

func New(ctx context.Context, cfg Config) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		cfg:    cfg,
	}
	go c.pollIbstat()
	return c
}

var _ components.Component = (*component)(nil)

type component struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	lastSuccessMu sync.RWMutex
	lastSuccess   *infiniband.IbstatOutput
}

func (c *component) Name() string { return nvidia_infiniband_id.Name }

func (c *component) Start() error { return nil }

var (
	msgThresholdNotSetSkipped = "ports or rate threshold not set, skipping"
	msgNoIbstatIssueFound     = "no infiniband issue found in ibstat"
)

// Returns the output evaluation reason and its healthy-ness.
// We DO NOT auto-detect infiniband devices/PCI buses, strictly rely on the user-specified config.
func evaluate(o *infiniband.IbstatOutput, cfg ExpectedPortStates) (string, bool, error) {
	// nothing specified for this machine, gpud MUST skip the ib check
	if cfg.AtLeastPorts <= 0 && cfg.AtLeastRate <= 0 {
		return msgThresholdNotSetSkipped, true, nil
	}

	atLeastPorts := cfg.AtLeastPorts
	atLeastRate := cfg.AtLeastRate
	if err := o.Parsed.CheckPortsAndRate(atLeastPorts, atLeastRate); err != nil {
		return err.Error(), false, nil
	}

	return msgNoIbstatIssueFound, true, nil
}

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastSuccessMu.RLock()
	lastSuccess := c.lastSuccess
	c.lastSuccessMu.RUnlock()

	if lastSuccess == nil {
		return []components.State{
			{
				Name:    "ibstat",
				Healthy: true,
				Health:  components.StateHealthy,
				Reason:  "no data",
			},
		}, nil
	}

	reason, healthy, err := evaluate(lastSuccess, GetDefaultExpectedPortStates())
	if err != nil {
		return nil, err
	}

	var suggestedActions *common.SuggestedActions = nil
	var health = components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
		suggestedActions = &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeHardwareInspection,
			},
			Descriptions: []string{
				"potential infiniband switch/hardware issue needs immediate attention",
			},
		}
	}

	return []components.State{
		{
			Name:             "ibstat",
			Healthy:          healthy,
			Health:           health,
			Reason:           reason,
			SuggestedActions: suggestedActions,
		},
	}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) pollIbstat() {
	ticker := time.NewTicker(c.cfg.PollInterval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
		}

		cctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
		defer cancel()

		o, err := infiniband.GetIbstatOutput(cctx, []string{c.cfg.IbstatCommand})
		if err != nil {
			log.Logger.Errorw("failed to poll ibstat", "error", err)
			continue
		}
		c.lastSuccess = o
	}
}
