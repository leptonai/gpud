// Package fabricmanager tracks the NVIDIA fabric manager version and its activeness.
// And streams the fabric manager logs for any errors and events.
package fabricmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	fabric_manager_log "github.com/leptonai/gpud/components/accelerator/nvidia/query/fabric-manager-log"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/components/query"
	query_log "github.com/leptonai/gpud/components/query/log"
	"github.com/leptonai/gpud/log"
)

const Name = "accelerator-nvidia-fabric-manager"

func New(ctx context.Context, cfg Config) (components.Component, error) {
	cfg.Query.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.SetDefaultPoller(
		nvidia_query.WithDBRW(cfg.Log.Query.State.DBRW),
		nvidia_query.WithDBRO(cfg.Log.Query.State.DBRO),
		nvidia_query.WithNvidiaSMICommand(cfg.NvidiaSMICommand),
		nvidia_query.WithNvidiaSMIQueryCommand(cfg.NvidiaSMIQueryCommand),
		nvidia_query.WithIbstatCommand(cfg.IbstatCommand),
		nvidia_query.WithInfinibandClassDirectory(cfg.InfinibandClassDirectory),
	)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, Name)

	if err := cfg.Log.Validate(); err != nil {
		ccancel()
		return nil, err
	}
	cfg.Log.SetDefaultsIfNotSet()

	if err := fabric_manager_log.CreateDefaultPoller(ctx, cfg.Log); err != nil {
		ccancel()
		return nil, err
	}
	fabric_manager_log.GetDefaultPoller().Start(cctx, cfg.Query, Name)

	return &component{
		rootCtx:   ctx,
		cancel:    ccancel,
		poller:    nvidia_query.GetDefaultPoller(),
		logPoller: fabric_manager_log.GetDefaultPoller(),
	}, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx   context.Context
	cancel    context.CancelFunc
	poller    query.Poller
	logPoller query_log.Poller
}

func (c *component) Name() string { return Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.LastSuccess()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", Name)
		return []components.State{
			{
				Name:    Name,
				Healthy: true,
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

	if !allOutput.FabricManagerExists {
		return []components.State{
			{
				Name:    Name,
				Healthy: true,
				Reason:  "fabric manager does not exist",
			},
		}, nil
	}
	if allOutput.FabricManagerExists && len(allOutput.FabricManagerErrors) > 0 {
		cs := make([]components.State, 0)
		for _, e := range allOutput.FabricManagerErrors {
			cs = append(cs, components.State{
				Name:    Name,
				Healthy: false,
				Error:   e,
				Reason:  "fabric manager query failed with " + e,
				ExtraInfo: map[string]string{
					nvidia_query.StateKeyFabricManagerExists: fmt.Sprintf("%v", allOutput.FabricManagerExists),
				},
			})
		}
		return cs, nil
	}
	output := ToOutput(allOutput)
	return output.States()
}

const (
	EventKeyFabricManagerNVSwitchLogUnixSeconds = "fabricmanager_nvswitch_log_unix_seconds"
	EventKeyFabricManagerNVSwitchLogLine        = "fabricmanager_nvswitch_log_line"
	EventKeyFabricManagerNVSwitchLogFilter      = "fabricmanager_nvswitch_log_filter"
	EventKeyFabricManagerNVSwitchLogError       = "fabricmanager_nvswitch_log_error"
)

// Returns `github.com/leptonai/gpud/components/query.ErrNoData` if there is no event found.
func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	items, err := c.logPoller.Find(since)
	if err != nil {
		return nil, err
	}

	evs := make([]components.Event, 0)
	for _, ev := range items {
		b, _ := ev.Matched.JSON()
		es := ""
		if ev.Error != nil {
			es = *ev.Error
		}
		evs = append(evs, components.Event{
			Time: ev.Time,
			Name: Name,
			Type: common.EventTypeCritical,
			ExtraInfo: map[string]string{
				EventKeyFabricManagerNVSwitchLogUnixSeconds: fmt.Sprintf("%d", ev.Time.Unix()),
				EventKeyFabricManagerNVSwitchLogLine:        ev.Line,
				EventKeyFabricManagerNVSwitchLogFilter:      string(b),
				EventKeyFabricManagerNVSwitchLogError:       es,
			},
		})
	}
	if len(evs) == 0 {
		return nil, nil
	}
	return evs, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	_ = c.poller.Stop(Name)
	c.logPoller.Stop(Name)

	return nil
}
