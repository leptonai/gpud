// Package infiniband monitors the infiniband status of the system.
// Optional, enabled if the host has NVIDIA GPUs.
package infiniband

import (
	"context"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	nvidia_infiniband_id "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/id"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query/infiniband"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/log"
)

var (
	defaultExpectedPortStatesMu sync.RWMutex
	defaultExpectedPortStates   = ExpectedPortStates{
		AtLeastPorts: 0,
		AtLeastRate:  0,
	}
)

func GetDefaultExpectedPortStates() ExpectedPortStates {
	defaultExpectedPortStatesMu.RLock()
	defer defaultExpectedPortStatesMu.RUnlock()
	return defaultExpectedPortStates
}

func SetDefaultExpectedPortStates(states ExpectedPortStates) {
	log.Logger.Infow("setting default expected port states", "at_least_ports", states.AtLeastPorts, "at_least_rate", states.AtLeastRate)

	defaultExpectedPortStatesMu.Lock()
	defer defaultExpectedPortStatesMu.Unlock()
	defaultExpectedPortStates = states
}

// Configures the expected state of the ports.
type ExpectedPortStates struct {
	// The minimum number of ports.
	// If not set, it defaults to the number of GPUs.
	AtLeastPorts int `json:"at_least_ports"`

	// The expected rate in Gb/sec.
	AtLeastRate int `json:"at_least_rate"`
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
	o, err := infiniband.GetIbstatOutput(ctx, []string{c.toolOverwrites.IbstatCommand})
	if err != nil {
		return nil, err
	}
	reason, healthy, err := evaluate(o, GetDefaultExpectedPortStates())
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

	return msgNoIbIssueFound, true, nil
}
