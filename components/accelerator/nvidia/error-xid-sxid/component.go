// Package errorxidsxid implements NVIDIA GPU driver Xid/SXid error detector.
package errorxidsxid

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	nvidia_error_xid_sxid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error-xid-sxid/id"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_xid_sxid_state "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid-sxid-state"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func New(ctx context.Context, cfg nvidia_common.Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()

	// this starts the Xid poller via "nvml.StartDefaultInstance"
	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.SetDefaultPoller(
		nvidia_query.WithDBRW(cfg.Query.State.DBRW),
		nvidia_query.WithDBRO(cfg.Query.State.DBRO),
		nvidia_query.WithNvidiaSMICommand(cfg.NvidiaSMICommand),
		nvidia_query.WithNvidiaSMIQueryCommand(cfg.NvidiaSMIQueryCommand),
		nvidia_query.WithIbstatCommand(cfg.IbstatCommand),
		nvidia_query.WithInfinibandClassDirectory(cfg.InfinibandClassDirectory),
	)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, nvidia_error_xid_sxid_id.Name)

	return &component{
		cfg:     cfg,
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.GetDefaultPoller(),
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	cfg     nvidia_common.Config
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  query.Poller
}

func (c *component) Name() string { return nvidia_error_xid_sxid_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return nil, nil
}

const (
	EventNameErroXid  = "error_xid"
	EventNameErroSXid = "error_sxid"

	EventKeyUnixSeconds    = "unix_seconds"
	EventKeyData           = "data"
	EventKeyEncoding       = "encoding"
	EventValueEncodingJSON = "json"
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
				Type:    common.EventTypeCritical,
				Message: msg,
				ExtraInfo: map[string]string{
					EventKeyUnixSeconds: strconv.FormatInt(event.UnixSeconds, 10),
					EventKeyData:        string(xidBytes),
					EventKeyEncoding:    EventValueEncodingJSON,
				},
			})
			continue
		}

		if sxidDetail := event.ToSXidDetail(); sxidDetail != nil {
			msg := fmt.Sprintf("sxid %d detected by %s (%s)",
				event.EventID,
				event.DataSource,
				humanize.Time(time.Unix(event.UnixSeconds, 0)),
			)
			sxidBytes, _ := sxidDetail.JSON()

			convertedEvents = append(convertedEvents, components.Event{
				Time:    metav1.Time{Time: time.Unix(event.UnixSeconds, 0).UTC()},
				Name:    EventNameErroSXid,
				Type:    common.EventTypeCritical,
				Message: msg,
				ExtraInfo: map[string]string{
					EventKeyUnixSeconds: strconv.FormatInt(event.UnixSeconds, 10),
					EventKeyData:        string(sxidBytes),
					EventKeyEncoding:    EventValueEncodingJSON,
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
	_ = c.poller.Stop(nvidia_error_xid_sxid_id.Name)

	return nil
}
