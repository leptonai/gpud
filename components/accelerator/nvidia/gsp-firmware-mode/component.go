// Package gspfirmwaremode tracks the NVIDIA GSP firmware mode.
package gspfirmwaremode

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	nvidia_gsp_firmware_mode_id "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode/id"
	"github.com/leptonai/gpud/internal/query"
	"github.com/leptonai/gpud/log"
	nvidia_query "github.com/leptonai/gpud/nvidia-query"
)

func New(ctx context.Context, cfg nvidia_common.Config) (components.Component, error) {
	if nvidia_query.GetDefaultPoller() == nil {
		return nil, nvidia_query.ErrDefaultPollerNotSet
	}

	cfg.Query.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, nvidia_gsp_firmware_mode_id.Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.GetDefaultPoller(),
	}, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  query.Poller
}

func (c *component) Name() string { return nvidia_gsp_firmware_mode_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.LastSuccess()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", nvidia_gsp_firmware_mode_id.Name)
		return []components.State{
			{
				Name:    nvidia_gsp_firmware_mode_id.Name,
				Healthy: true,
				Error:   query.ErrNoData.Error(),
				Reason:  query.ErrNoData.Error(),
			},
		}, nil
	}
	if err != nil {
		return nil, err
	}

	allOutput, ok := last.Output.(*nvidia_query.Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T", last.Output)
	}
	if lerr := c.poller.LastError(); lerr != nil {
		log.Logger.Warnw("last query failed -- returning cached, possibly stale data", "error", lerr)
	}
	lastSuccessPollElapsed := time.Now().UTC().Sub(allOutput.Time)
	if lastSuccessPollElapsed > 2*c.poller.Config().Interval.Duration {
		log.Logger.Warnw("last poll is too old", "elapsed", lastSuccessPollElapsed, "interval", c.poller.Config().Interval.Duration)
	}

	output := ToOutput(allOutput)
	return output.States()
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

	// safe to call stop multiple times
	_ = c.poller.Stop(nvidia_gsp_firmware_mode_id.Name)

	return nil
}
