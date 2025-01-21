// Package sxid tracks the NVIDIA GPU SXid errors scanning the dmesg.
// See fabric manager documentation https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf.
package sxid

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_component_error_sxid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid/id"
	nvidia_query_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/query/sxid"
	"github.com/leptonai/gpud/components/dmesg"
	"github.com/leptonai/gpud/log"
)

func New() components.Component {
	return &component{}
}

var _ components.Component = (*component)(nil)

type component struct{}

func (c *component) Name() string { return nvidia_component_error_sxid_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return []components.State{{
		Name:    StateNameErrorSXid,
		Healthy: true,
		Reason:  "sxid monitoring working",
	}}, nil
}

// tailScan fetches the latest output from the dmesg
// it is ok to call this function multiple times for the following reasons (thus shared with events method)
// 1) dmesg "TailScan" is cheap (just tails the last x number of lines)
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
		if logItem.Matched.Name != dmesg.EventNvidiaNVSwitchSXid {
			continue
		}

		ev, err := nvidia_query_sxid.ParseDmesgLogLine(logItem.Time, logItem.Line)
		if err != nil {
			return nil, err
		}
		o.DmesgErrors = append(o.DmesgErrors, ev)
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
	return nil
}
