// Package xid tracks the NVIDIA GPU Xid errors scanning the dmesg
// and using the NVIDIA Management Library (NVML).
// See Xid messages https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages.
package xid

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/components/dmesg"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
)

const Name = "accelerator-nvidia-error-xid"

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, Name)

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

func (c *component) Name() string { return Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
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
	dmesgState, err := dmesgComponent.State()
	if err != nil {
		return nil, err
	}

	o := &Output{}
	for _, logItem := range dmesgState.TailScanMatched {
		if logItem.Error != nil {
			continue
		}
		if logItem.Matched == nil {
			continue
		}
		if logItem.Matched.Name != dmesg.EventNvidiaNVRMXid {
			continue
		}

		ev, err := nvidia_query_xid.ParseDmesgLogLine(logItem.Line)
		if err != nil {
			return nil, err
		}
		o.DmesgErrors = append(o.DmesgErrors, ev)

		if ev.Detail != nil && ev.Detail.SuggestedActions != nil && len(ev.Detail.SuggestedActions.RepairActions) > 0 {
			if o.SuggestedActions == nil {
				o.SuggestedActions = &common.SuggestedActions{}
			}
			o.SuggestedActions.Add(ev.Detail.SuggestedActions)
		}
	}

	last, err := c.poller.Last()
	if err != nil {
		return nil, err
	}
	if last == nil && err != nil && err != query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", Name)
		return []components.State{
			{
				Name:    Name,
				Healthy: false,
				Error:   query.ErrNoData.Error(),
				Reason:  query.ErrNoData.Error(),
			},
		}, nil
	}
	if last.Error != nil {
		return []components.State{
			{
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}
	if last.Output == nil {
		return []components.State{
			{
				Healthy: false,
				Reason:  "no output",
			},
		}, nil
	}

	ev, ok := last.Output.(*nvidia_query_nvml.XidEvent)
	if !ok {
		return nil, fmt.Errorf("invalid output type: %T, expected nvidia_query_nvml.XidEvent", last.Output)
	}
	if ev != nil {
		if ev.Xid > 0 {
			o.NVMLXidEvent = ev
		}
		if ev.Detail != nil && ev.Detail.SuggestedActions != nil && len(ev.Detail.SuggestedActions.RepairActions) > 0 {
			if o.SuggestedActions == nil {
				o.SuggestedActions = &common.SuggestedActions{}
			}
			o.SuggestedActions.Add(ev.Detail.SuggestedActions)
		}
	}
	return o.States()
}

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
	dmesgState, err := dmesgComponent.State()
	if err != nil {
		return nil, err
	}

	o := &Output{}
	for _, logItem := range dmesgState.TailScanMatched {
		if logItem.Error != nil {
			continue
		}
		if logItem.Matched == nil {
			continue
		}
		if logItem.Matched.Name != dmesg.EventNvidiaNVRMXid {
			continue
		}

		ev, err := nvidia_query_xid.ParseDmesgLogLine(logItem.Line)
		if err != nil {
			return nil, err
		}
		o.DmesgErrors = append(o.DmesgErrors, ev)

		if ev.Detail != nil && ev.Detail.SuggestedActions != nil && len(ev.Detail.SuggestedActions.RepairActions) > 0 {
			if o.SuggestedActionsPerLogLine == nil {
				o.SuggestedActionsPerLogLine = make(map[string]*common.SuggestedActions)
			}
			o.SuggestedActionsPerLogLine[ev.LogItem.Line] = ev.Detail.SuggestedActions
		}
	}
	return o.Events(), nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(Name)

	return nil
}
