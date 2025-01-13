// Package sxid tracks the NVIDIA GPU SXid errors scanning the dmesg.
// See fabric manager documentation https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf.
package sxid

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_component_error_sxid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid/id"
	nvidia_xid_sxid_state "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid-sxid-state"
	common_dmesg "github.com/leptonai/gpud/components/common/dmesg"
	"github.com/leptonai/gpud/log"

	"github.com/dustin/go-humanize"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)

	if runtime.GOOS == "linux" {
		if perr := common_dmesg.SetDefaultLogPoller(ctx, cfg.Query.State.DBRW, cfg.Query.State.DBRO); perr != nil {
			log.Logger.Warnw("failed to set default log poller", "error", perr)
		} else {
			common_dmesg.GetDefaultLogPoller().Start(cctx, cfg.Query, common_dmesg.Name)
		}
	}

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		cfg:     cfg,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc
	cfg     Config
}

func (c *component) Name() string { return nvidia_component_error_sxid_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return []components.State{{
		Name:    StateNameErrorSXid,
		Healthy: true,
		Reason:  "sxid monitoring working",
	}}, nil
}

const (
	EventNameErroSXid = "error_sxid"

	EventKeyErroSXidUnixSeconds    = "unix_seconds"
	EventKeyErroSXidData           = "data"
	EventKeyErroSXidEncoding       = "encoding"
	EventValueErroSXidEncodingJSON = "json"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}

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
				Type:    components.EventTypeCritical,
				Message: msg,
				ExtraInfo: map[string]string{
					EventKeyErroSXidUnixSeconds: strconv.FormatInt(event.UnixSeconds, 10),
					EventKeyErroSXidData:        string(sxidBytes),
					EventKeyErroSXidEncoding:    EventValueErroSXidEncodingJSON,
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

	if runtime.GOOS == "linux" {
		_ = common_dmesg.GetDefaultLogPoller().Stop(common_dmesg.Name)
	}

	c.cancel()
	return nil
}
