// Package peermem monitors the peermem module status.
// Optional, enabled if the host has NVIDIA GPUs.
package peermem

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_peermem_id "github.com/leptonai/gpud/components/accelerator/nvidia/peermem/id"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/dmesg"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/log"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	"github.com/leptonai/gpud/pkg/query"
)

func New(ctx context.Context, cfg nvidia_common.Config) (components.Component, error) {
	eventsStore, err := events_db.NewStore(
		cfg.Query.State.DBRW,
		cfg.Query.State.DBRO,
		events_db.CreateDefaultTableName(nvidia_peermem_id.Name),
		3*24*time.Hour,
	)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	logLineProcessor, err := dmesg.NewLogLineProcessor(cctx, Match, eventsStore)
	if err != nil {
		ccancel()
		return nil, err
	}

	// TODO: deprecate shared poller in favor of its own "lsmod" poller for peermem
	if nvidia_query.GetDefaultPoller() == nil {
		ccancel()
		return nil, nvidia_query.ErrDefaultPollerNotSet
	}

	cfg.Query.SetDefaultsIfNotSet()
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, nvidia_peermem_id.Name)

	return &component{
		rootCtx:          ctx,
		cancel:           ccancel,
		logLineProcessor: logLineProcessor,
		eventsStore:      eventsStore,
		poller:           nvidia_query.GetDefaultPoller(),
	}, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx          context.Context
	cancel           context.CancelFunc
	logLineProcessor *dmesg.LogLineProcessor
	eventsStore      events_db.Store
	poller           query.Poller
}

func (c *component) Name() string { return nvidia_peermem_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.LastSuccess()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", nvidia_peermem_id.Name)
		return []components.State{
			{
				Name:    nvidia_peermem_id.Name,
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

	if len(allOutput.LsmodPeermemErrors) > 0 {
		cs := make([]components.State, 0)
		for _, e := range allOutput.LsmodPeermemErrors {
			cs = append(cs, components.State{
				Name:    nvidia_peermem_id.Name,
				Healthy: false,
				Error:   e,
				Reason:  "lsmod peermem query failed with " + e,
			})
		}
		return cs, nil
	}
	output := ToOutput(allOutput)
	return output.States()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventsStore.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.cancel()

	// safe to call stop multiple times
	_ = c.poller.Stop(nvidia_peermem_id.Name)

	c.logLineProcessor.Close()
	c.eventsStore.Close()

	return nil
}
