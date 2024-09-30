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

		if ev.Detail != nil {
			if ev.Detail.RequiredActions.ResetGPU {
				if o.RequiredActions == nil {
					o.RequiredActions = &common.RequiredActions{}
				}
				o.RequiredActions.ResetGPU = true
				o.RequiredActions.Descriptions = append(o.RequiredActions.Descriptions,
					fmt.Sprintf("GPU reset required due to XID %d (name %s)", ev.Detail.XID, ev.Detail.Name))
			}
			if ev.Detail.RequiredActions.RebootSystem {
				if o.RequiredActions == nil {
					o.RequiredActions = &common.RequiredActions{}
				}
				o.RequiredActions.RebootSystem = true
				o.RequiredActions.Descriptions = append(o.RequiredActions.Descriptions,
					fmt.Sprintf("system reboot required due to XID %d (name %s)", ev.Detail.XID, ev.Detail.Name))
			}
		}
	}

	last, err := c.poller.Last()
	if err != nil {
		return nil, err
	}
	if last == nil || last.Output == nil { // no data
		log.Logger.Debugw("no xid data -- this is normal when nvml has not received any registered xid events yet")
	} else {
		xidEvent, ok := last.Output.(*nvidia_query_nvml.XidEvent)
		if !ok {
			return nil, fmt.Errorf("invalid output type: %T, expected nvidia_query_nvml.XidEvent", last.Output)
		}
		if xidEvent != nil {
			if xidEvent.Xid > 0 {
				o.NVMLXidEvent = xidEvent
			}
			if xidEvent.Detail != nil {
				if xidEvent.Detail.RequiredActions.ResetGPU {
					if o.RequiredActions == nil {
						o.RequiredActions = &common.RequiredActions{}
					}
					o.RequiredActions.ResetGPU = true
					o.RequiredActions.Descriptions = append(o.RequiredActions.Descriptions,
						fmt.Sprintf("GPU reset required for %s due to XID %d (name %s)", xidEvent.DeviceUUID, xidEvent.Detail.XID, xidEvent.Detail.Name))
				}
				if xidEvent.Detail.RequiredActions.RebootSystem {
					if o.RequiredActions == nil {
						o.RequiredActions = &common.RequiredActions{}
					}
					o.RequiredActions.RebootSystem = true
					o.RequiredActions.Descriptions = append(o.RequiredActions.Descriptions,
						fmt.Sprintf("System reboot required for %s due to XID %d (name %s)", xidEvent.DeviceUUID, xidEvent.Detail.XID, xidEvent.Detail.Name))
				}
			}
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

		if ev.Detail != nil {
			if ev.Detail.RequiredActions.ResetGPU {
				if o.RequiredActionsPerLogLine == nil {
					o.RequiredActionsPerLogLine = make(map[string]*common.RequiredActions)
				}

				actions := &common.RequiredActions{}
				if v, ok := o.RequiredActionsPerLogLine[ev.LogItem.Line]; ok {
					actions = v
				}

				actions.ResetGPU = true
				actions.Descriptions = append(actions.Descriptions,
					fmt.Sprintf("GPU reset required due to XID %d (name %s)", ev.Detail.XID, ev.Detail.Name))

				o.RequiredActionsPerLogLine[ev.LogItem.Line] = actions
			}

			if ev.Detail.RequiredActions.RebootSystem {
				if o.RequiredActionsPerLogLine == nil {
					o.RequiredActionsPerLogLine = make(map[string]*common.RequiredActions)
				}

				actions := &common.RequiredActions{}
				if v, ok := o.RequiredActionsPerLogLine[ev.LogItem.Line]; ok {
					actions = v
				}

				actions.RebootSystem = true
				actions.Descriptions = append(actions.Descriptions,
					fmt.Sprintf("system reboot required due to XID %d (name %s)", ev.Detail.XID, ev.Detail.Name))

				o.RequiredActionsPerLogLine[ev.LogItem.Line] = actions
			}
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
