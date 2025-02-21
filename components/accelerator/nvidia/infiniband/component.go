// Package infiniband monitors the infiniband status of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package infiniband

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_infiniband_id "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/id"
	"github.com/leptonai/gpud/pkg/common"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

var (
	defaultExpectedPortStatesMu sync.RWMutex
	defaultExpectedPortStates   = infiniband.ExpectedPortStates{
		AtLeastPorts: 0,
		AtLeastRate:  0,
	}
)

func GetDefaultExpectedPortStates() infiniband.ExpectedPortStates {
	defaultExpectedPortStatesMu.RLock()
	defer defaultExpectedPortStatesMu.RUnlock()
	return defaultExpectedPortStates
}

func SetDefaultExpectedPortStates(states infiniband.ExpectedPortStates) {
	log.Logger.Infow("setting default expected port states", "at_least_ports", states.AtLeastPorts, "at_least_rate", states.AtLeastRate)

	defaultExpectedPortStatesMu.Lock()
	defer defaultExpectedPortStatesMu.Unlock()
	defaultExpectedPortStates = states
}

func New(ctx context.Context, toolOverwrites nvidia_common.ToolOverwrites) components.Component {
	return &component{
		toolOverwrites: toolOverwrites,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	toolOverwrites nvidia_common.ToolOverwrites
}

func (c *component) Name() string { return nvidia_infiniband_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return c.getStates(ctx, GetDefaultExpectedPortStates())
}

func (c *component) getStates(ctx context.Context, thresholds infiniband.ExpectedPortStates) ([]components.State, error) {
	// in rare cases, some machines have "ibstat" installed that returns empty output
	// not failing the ibstat check, thus we need manual check on the thresholds here
	// before we call the ibstat command
	if thresholds.AtLeastPorts <= 0 && thresholds.AtLeastRate <= 0 {
		return []components.State{
			{
				Name:   "ibstat",
				Health: components.StateHealthy,
				//TODO: depreciate Healthy field
				Healthy: true,
				Reason:  msgThresholdNotSetSkipped,
			},
		}, nil
	}

	o, err := infiniband.GetIbstatOutput(ctx, []string{c.toolOverwrites.IbstatCommand})
	if errors.Is(err, infiniband.ErrNoIbstatCommand) {
		return []components.State{
			{
				Name:   "ibstat",
				Health: components.StateUnhealthy,
				//TODO: depreciate Healthy field
				Healthy: false,
				Reason:  fmt.Sprintf("ibstat threshold set but %s", err),
			},
		}, nil
	}

	if err != nil {
		return nil, err
	}
	reason, healthy, err := evaluate(o, thresholds)
	if err != nil {
		return nil, err
	}

	var healthState = components.StateHealthy
	var suggestedActions *common.SuggestedActions
	if !healthy {
		healthState = components.StateUnhealthy
		suggestedActions = &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeHardwareInspection,
			},
			Descriptions: []string{
				"potential infiniband switch/hardware issue needs immediate attention",
			},
		}

		log.Logger.Warnw("ibstat issue found", "reason", reason, "output", o.Raw)
	}

	return []components.State{
		{
			Name:             "ibstat",
			Healthy:          healthy,
			Health:           healthState,
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

	return nil
}

var (
	msgThresholdNotSetSkipped = "ports or rate threshold not set, skipping"
	msgNoIbIssueFound         = "no infiniband issue found (in ibstat)"
)

// Returns the output evaluation reason and its healthy-ness.
// We DO NOT auto-detect infiniband devices/PCI buses, strictly rely on the user-specified config.
func evaluate(o *infiniband.IbstatOutput, cfg infiniband.ExpectedPortStates) (string, bool, error) {
	// nothing specified for this machine, gpud MUST skip the ib check
	if cfg.AtLeastPorts <= 0 && cfg.AtLeastRate <= 0 {
		return msgThresholdNotSetSkipped, true, nil
	}

	atLeastPorts := cfg.AtLeastPorts
	atLeastRate := cfg.AtLeastRate
	if err := o.Parsed.CheckPortsAndRate(atLeastPorts, atLeastRate); err != nil {
		return err.Error(), false, nil
	}

	return msgNoIbIssueFound, true, nil
}
