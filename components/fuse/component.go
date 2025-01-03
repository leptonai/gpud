// Package fuse monitors the FUSE (Filesystem in Userspace).
package fuse

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	fuse_id "github.com/leptonai/gpud/components/fuse/id"
	"github.com/leptonai/gpud/components/fuse/metrics"
	"github.com/leptonai/gpud/components/fuse/state"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	"github.com/dustin/go-humanize"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, fuse_id.Name)

	return &component{
		cfg:     cfg,
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  getDefaultPoller(),
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	cfg      Config
	rootCtx  context.Context
	cancel   context.CancelFunc
	poller   query.Poller
	gatherer prometheus.Gatherer
}

func (c *component) Name() string { return fuse_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	last, err := c.poller.Last()
	if err == query.ErrNoData { // no data
		log.Logger.Debugw("nothing found in last state (no data collected yet)", "component", fuse_id.Name)
		return []components.State{
			{
				Name:    fuse_id.Name,
				Healthy: true,
				Reason:  query.ErrNoData.Error(),
			},
		}, nil
	}
	if err != nil {
		return nil, err
	}

	if last.Error != nil {
		return []components.State{
			{
				Name:    fuse_id.Name,
				Healthy: false,
				Error:   last.Error.Error(),
				Reason:  "last query failed",
			},
		}, nil
	}

	return []components.State{
		{
			Name:    fuse_id.Name,
			Healthy: true,
		},
	}, nil
}

const (
	EventNameFuseConnections = "fuse_connections"

	EventKeyUnixSeconds    = "unix_seconds"
	EventKeyData           = "data"
	EventKeyEncoding       = "encoding"
	EventValueEncodingJSON = "json"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	events, err := state.ReadEvents(ctx, c.cfg.Query.State.DBRO, state.WithSince(since))
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
		msgs := []string{}
		if event.CongestedPercentAgainstThreshold > c.cfg.CongestedPercentAgainstThreshold {
			msgs = append(msgs, fmt.Sprintf("congested percent against threshold %.2f exceeds threshold %.2f", event.CongestedPercentAgainstThreshold, c.cfg.CongestedPercentAgainstThreshold))
		}
		if event.MaxBackgroundPercentAgainstThreshold > c.cfg.MaxBackgroundPercentAgainstThreshold {
			msgs = append(msgs, fmt.Sprintf("max background percent against threshold %.2f exceeds threshold %.2f", event.MaxBackgroundPercentAgainstThreshold, c.cfg.MaxBackgroundPercentAgainstThreshold))
		}
		if len(msgs) == 0 {
			continue
		}

		eb, err := event.JSON()
		if err != nil {
			continue
		}

		convertedEvents = append(convertedEvents, components.Event{
			Time:    metav1.Time{Time: time.Unix(event.UnixSeconds, 0).UTC()},
			Name:    EventNameFuseConnections,
			Type:    components.EventTypeCritical,
			Message: strings.Join(msgs, ", "),
			ExtraInfo: map[string]string{
				EventKeyUnixSeconds: strconv.FormatInt(event.UnixSeconds, 10),
				EventKeyData:        string(eb),
				EventKeyEncoding:    EventValueEncodingJSON,
			},
		})
	}
	if len(convertedEvents) == 0 {
		return nil, nil
	}
	return convertedEvents, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	congestedPercents, err := metrics.ReadConnectionsCongestedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read congested percents: %w", err)
	}
	maxBackgroundPercents, err := metrics.ReadConnectionsMaxBackgroundPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read max background percents: %w", err)
	}
	ms := make([]components.Metric, 0, len(congestedPercents)+len(maxBackgroundPercents))
	for _, m := range congestedPercents {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range maxBackgroundPercents {
		ms = append(ms, components.Metric{Metric: m})
	}

	return ms, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.poller.Stop(fuse_id.Name)

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return metrics.Register(reg, dbRW, dbRO, tableName)
}
