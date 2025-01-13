// Package powersupply tracks the power supply/usage on the host.
package powersupply

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"time"

	"github.com/leptonai/gpud/components"
	common_dmesg "github.com/leptonai/gpud/components/common/dmesg"
	power_supply_id "github.com/leptonai/gpud/components/power-supply/id"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, power_supply_id.Name)

	if perr := common_dmesg.SetDefaultLogPoller(ctx, cfg.Query.State.DBRW, cfg.Query.State.DBRO); perr != nil {
		log.Logger.Warnw("failed to set default log poller", "error", perr)
	} else {
		common_dmesg.GetDefaultLogPoller().Start(cctx, cfg.Query, common_dmesg.Name)
	}

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  getDefaultPoller(),
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  query.Poller
}

func (c *component) Name() string { return power_supply_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", power_supply_id.Name)
		return []components.State{
			{
				Name:    power_supply_id.Name,
				Healthy: true,
				Reason:  query.ErrNoData.Error(),
			},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if last.Error != nil {
		return []components.State{
			{
				Name:    power_supply_id.Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Name:    power_supply_id.Name,
				Healthy: true,
				Reason:  "no output",
			},
		}, nil
	}

	output, ok := last.Output.(*Output)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T", last.Output)
	}
	return output.States()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}

	if common_dmesg.GetDefaultLogPoller() == nil {
		return nil, nil
	}
	select {
	case <-common_dmesg.GetDefaultLogPoller().WaitStart():
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	logItems, err := common_dmesg.GetDefaultLogPoller().Find(since)
	if err != nil {
		return nil, err
	}

	events := make([]components.Event, 0)
	for _, logItem := range logItems {
		if logItem.Error != nil {
			continue
		}
		if logItem.Matched == nil {
			continue
		}
		if logItem.Matched.Name != common_dmesg.EventPowerSupplyInsufficientPowerOnPCIe {
			continue
		}

		events = append(events, components.Event{
			Time:    logItem.Time,
			Name:    common_dmesg.EventPowerSupplyInsufficientPowerOnPCIe,
			Type:    components.EventTypeCritical,
			Message: "Insufficient power on the PCIe slot",
			ExtraInfo: map[string]string{
				"unix_seconds": strconv.FormatInt(logItem.Time.Unix(), 10),
				"log_line":     logItem.Line,
			},
		})
	}

	return events, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	_ = c.poller.Stop(power_supply_id.Name)

	if runtime.GOOS == "linux" {
		_ = common_dmesg.GetDefaultLogPoller().Stop(common_dmesg.Name)
	}

	c.cancel()
	return nil
}
