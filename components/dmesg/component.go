package dmesg

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	query_log "github.com/leptonai/gpud/components/query/log"
	query_log_tail "github.com/leptonai/gpud/components/query/log/tail"
	"github.com/leptonai/gpud/log"
)

const (
	Name        = "dmesg"
	Description = "Scans and watches the /var/log/dmesg file for errors, as specified in the configuration (e.g., regex match NVIDIA GPU errors)."
)

var Tags = []string{"dmesg", "log", "error"}

func New(ctx context.Context, cfg Config) (components.Component, error) {
	if err := cfg.Log.Validate(); err != nil {
		return nil, err
	}
	cfg.Log.SetDefaultsIfNotSet()

	if err := createDefaultLogPoller(ctx, cfg); err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	GetDefaultLogPoller().Start(cctx, cfg.Log.Query, Name)

	return &Component{
		cfg:       &cfg,
		rootCtx:   ctx,
		cancel:    ccancel,
		logPoller: GetDefaultLogPoller(),
	}, nil
}

var _ components.Component = (*Component)(nil)

type Component struct {
	cfg       *Config
	rootCtx   context.Context
	cancel    context.CancelFunc
	logPoller query_log.Poller
}

func (c *Component) Name() string { return Name }

func (c *Component) State() (*State, error) {
	s := &State{
		File:         c.logPoller.File(),
		LastSeekInfo: c.logPoller.SeekInfo(),
	}

	if c.cfg != nil && c.cfg.Log.Scan != nil {
		items, err := c.logPoller.TailScan(
			c.rootCtx,
			query_log_tail.WithFile(c.cfg.Log.Scan.File),
			query_log_tail.WithCommands(c.cfg.Log.Scan.Commands),
			query_log_tail.WithLinesToTail(c.cfg.Log.Scan.LinesToTail),
		)
		if err != nil {
			return nil, err
		}
		if len(items) > 0 {
			s.TailScanMatched = items
		}
	}

	return s, nil
}

func (c *Component) States(ctx context.Context) ([]components.State, error) {
	s, err := c.State()
	if err != nil {
		return nil, err
	}
	return s.States(), nil
}

func (c *Component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	items, err := c.logPoller.Find(since)
	if err != nil {
		return nil, err
	}
	ev := &Event{Matched: items}
	return ev.Events(), nil
}

func (c *Component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	return nil, nil
}

func (c *Component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	c.logPoller.Stop(Name)

	return nil
}
