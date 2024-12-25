// Package xid tracks the NVIDIA GPU Xid errors scanning the dmesg
// and using the NVIDIA Management Library (NVML).
// See Xid messages https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages.
package xid

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	"github.com/leptonai/gpud/components/dmesg"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, nvidia_component_error_xid_id.Name)

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

func (c *component) Name() string { return nvidia_component_error_xid_id.Name }

// Just checks if the xid poller is working.
func (c *component) States(_ context.Context) ([]components.State, error) {
	last, err := c.poller.LastSuccess()

	// no data yet from realtime xid poller
	// just return whatever we got from dmesg
	if err == query.ErrNoData {
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", nvidia_component_error_xid_id.Name)
		return []components.State{
			{
				Name:    StateNameErrorXid,
				Healthy: true,
				Reason:  "no xid error event",
			},
		}, nil
	}

	// something went wrong in the poller
	// just return an error to surface the issue
	if err != nil {
		return nil, err
	}
	if last.Error != nil {
		return nil, last.Error
	}

	return []components.State{
		{
			Name:    StateNameErrorXid,
			Healthy: true,
			Reason:  "xid event polling is working",
		},
	}, nil
}

// tailScan fetches the latest output from the dmesg and the NVML poller
// it is ok to call this function multiple times for the following reasons (thus shared with events method)
// 1) dmesg "TailScan" is cheap (just tails the last x number of lines)
// 2) NVML poller "Last" costs nothing, since we simply read the last state in the queue (no NVML call involved)
func (c *component) tailScan() (*Output, error) {
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

	o := &Output{}
	for _, logItem := range dmesgTailResults.TailScanMatched {
		if logItem.Error != nil {
			continue
		}
		if logItem.Matched == nil {
			continue
		}
		if logItem.Matched.Name != dmesg.EventNvidiaNVRMXid {
			continue
		}

		ev, err := nvidia_query_xid.ParseDmesgLogLine(logItem.Time, logItem.Line)
		if err != nil {
			return nil, err
		}
		o.DmesgErrors = append(o.DmesgErrors, ev)
	}

	last, err := c.poller.LastSuccess()

	// no data yet from realtime xid poller
	// just return whatever we got from dmesg
	if err == query.ErrNoData {
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", nvidia_component_error_xid_id.Name)
		return o, nil
	}

	// something went wrong in the poller
	// just return an error to surface the issue
	if err != nil {
		return nil, err
	}
	if last.Error != nil {
		return nil, last.Error
	}

	// no output from the poller
	// just return whatever we got from dmesg
	if last.Output == nil {
		return o, nil
	}

	ev, ok := last.Output.(*nvidia_query_nvml.XidEvent)
	if !ok { // shoild never happen
		return nil, fmt.Errorf("invalid output type: %T, expected nvidia_query_nvml.XidEvent", last.Output)
	}
	if ev != nil && ev.Xid > 0 {
		o.NVMLXidEvent = ev

		lastSuccessPollElapsed := time.Now().UTC().Sub(ev.Time.Time)
		if lastSuccessPollElapsed > 2*c.poller.Config().Interval.Duration {
			log.Logger.Warnw("last poll is too old", "elapsed", lastSuccessPollElapsed, "interval", c.poller.Config().Interval.Duration)
		}
	}

	return o, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	o, err := c.tailScan()
	if err != nil {
		return nil, err
	}
	return o.getEvents(since), nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(nvidia_component_error_xid_id.Name)

	return nil
}
