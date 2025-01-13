// Package xid tracks the NVIDIA GPU Xid errors scanning the dmesg
// and using the NVIDIA Management Library (NVML).
// See Xid messages https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages.
package xid

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/components"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	nvidia_xid_sxid_state "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid-sxid-state"
	common_dmesg "github.com/leptonai/gpud/components/common/dmesg"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, nvidia_component_error_xid_id.Name)

	common_dmesg.SetDefaultLogPoller(ctx, cfg.Query.State.DBRW, cfg.Query.State.DBRO)
	common_dmesg.GetDefaultLogPoller().Start(cctx, cfg.Query, common_dmesg.Name)

	return &component{
		cfg:     cfg,
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  getDefaultPoller(),
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	cfg     Config
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  query.Poller
}

func (c *component) Name() string { return nvidia_component_error_xid_id.Name }

// Just checks if the xid poller is working.
func (c *component) States(_ context.Context) ([]components.State, error) {
	return []components.State{
		{
			Name:    StateNameErrorXid,
			Healthy: true,
			Reason:  "xid event polling is working",
		},
	}, nil
}

const (
	EventNameErroXid = "error_xid"

	EventKeyErroXidUnixSeconds    = "unix_seconds"
	EventKeyErroXidData           = "data"
	EventKeyErroXidEncoding       = "encoding"
	EventValueErroXidEncodingJSON = "json"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	events, err := nvidia_xid_sxid_state.ReadEvents(ctx, c.cfg.Query.State.DBRO, nvidia_xid_sxid_state.WithSince(since))
	if err != nil {
		return nil, err
	}

	if len(events) == 0 {
		log.Logger.Debugw("no event found", "component", c.Name(), "since", humanize.Time(since))
		return nil, nil
	}

	log.Logger.Debugw("found events", "component", c.Name(), "since", humanize.Time(since), "count", len(events))
	convertedEvents := make([]components.Event, 0, len(events))
	for _, event := range events {
		if xidDetail := event.ToXidDetail(); xidDetail != nil {
			msg := fmt.Sprintf("xid %d detected by %s (%s)",
				event.EventID,
				event.DataSource,
				humanize.Time(time.Unix(event.UnixSeconds, 0)),
			)
			xidBytes, _ := xidDetail.JSON()

			convertedEvents = append(convertedEvents, components.Event{
				Time:    metav1.Time{Time: time.Unix(event.UnixSeconds, 0).UTC()},
				Name:    EventNameErroXid,
				Type:    components.EventTypeCritical,
				Message: msg,
				ExtraInfo: map[string]string{
					EventKeyErroXidUnixSeconds: strconv.FormatInt(event.UnixSeconds, 10),
					EventKeyErroXidData:        string(xidBytes),
					EventKeyErroXidEncoding:    EventValueErroXidEncodingJSON,
				},
			})
			continue
		}
	}

	return convertedEvents, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(nvidia_component_error_xid_id.Name)

	common_dmesg.GetDefaultLogPoller().Stop(common_dmesg.Name)

	c.cancel()
	return nil
}
