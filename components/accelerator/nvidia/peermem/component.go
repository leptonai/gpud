// Package peermem monitors the peermem module status.
// Optional, enabled if the host has NVIDIA GPUs.
package peermem

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_peermem_id "github.com/leptonai/gpud/components/accelerator/nvidia/peermem/id"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	"github.com/leptonai/gpud/components/dmesg"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.SetDefaultPoller(cfg.Query.State.DB)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, nvidia_peermem_id.Name)

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.GetDefaultPoller(),
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  query.Poller
}

func (c *component) Name() string { return nvidia_peermem_id.Name }

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

const (
	// repeated messages may indicate more persistent issue on the inter-GPU communication
	// e.g.,
	// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
	// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
	// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
	EventNamePeermemInvalidContextFromDmesg = "peermem_invalid_context_from_dmesg"

	EventKeyPeermemInvalidContextFromDmesgUnixSeconds = "unix_seconds"
	EventKeyPeermemInvalidContextFromDmesgLogLine     = "log_line"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	dmesgC, err := components.GetComponent(dmesg.Name)
	if err != nil {
		return nil, err
	}

	var dmesgComponent *dmesg.Component
	if o, ok := dmesgC.(interface{ Unwrap() interface{} }); ok {
		if unwrapped, ok := o.Unwrap().(*dmesg.Component); ok {
			dmesgComponent = unwrapped
		} else {
			return nil, fmt.Errorf("expected *dmesg.Component, got %T", dmesgC)
		}
	}
	dmesgTailResults, err := dmesgComponent.TailScan()
	if err != nil {
		return nil, err
	}

	// dedup by minute level
	seenMinute := make(map[int64]struct{})
	events := make([]components.Event, 0)
	for _, logItem := range dmesgTailResults.TailScanMatched {
		if logItem.Error != nil {
			continue
		}
		if logItem.Matched == nil {
			continue
		}
		if logItem.Matched.Name != dmesg.EventNvidiaPeermemInvalidContext {
			continue
		}

		minute := logItem.Time.Unix() / 60
		if _, ok := seenMinute[minute]; ok {
			continue
		}
		seenMinute[minute] = struct{}{}

		events = append(events, components.Event{
			Time: logItem.Time,
			Name: EventNamePeermemInvalidContextFromDmesg,
			Type: components.EventTypeCritical,
			ExtraInfo: map[string]string{
				EventKeyPeermemInvalidContextFromDmesgUnixSeconds: strconv.FormatInt(logItem.Time.Unix(), 10),
				EventKeyPeermemInvalidContextFromDmesgLogLine:     logItem.Line,
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
	_ = c.poller.Stop(nvidia_peermem_id.Name)

	return nil
}
