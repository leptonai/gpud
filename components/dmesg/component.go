// Package dmesg scans and watches dmesg outputs for errors,
// as specified in the configuration (e.g., regex match NVIDIA GPU errors).
package dmesg

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/log"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
	query_log "github.com/leptonai/gpud/poller/log"
	poller_log_common "github.com/leptonai/gpud/poller/log/common"
	poller_log_tail "github.com/leptonai/gpud/poller/log/tail"
)

const Name = "dmesg"

func New(ctx context.Context, cfg Config, processMatched poller_log_common.ProcessMatchedFunc) (components.Component, error) {
	if err := cfg.Log.Validate(); err != nil {
		return nil, err
	}
	cfg.Log.SetDefaultsIfNotSet()

	if err := createDefaultLogPoller(ctx, cfg, processMatched); err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	defaultLogPoller.Start(cctx, cfg.Log.Query, Name)

	return &Component{
		cfg:            &cfg,
		rootCtx:        ctx,
		cancel:         ccancel,
		logPoller:      defaultLogPoller,
		processMatched: processMatched,
	}, nil
}

var _ components.Component = (*Component)(nil)

type Component struct {
	cfg            *Config
	rootCtx        context.Context
	cancel         context.CancelFunc
	logPoller      query_log.Poller
	processMatched poller_log_common.ProcessMatchedFunc
}

func (c *Component) Name() string { return Name }

func (c *Component) TailScan() (*State, error) {
	s := &State{
		File:         c.logPoller.File(),
		LastSeekInfo: c.logPoller.SeekInfo(),
	}

	if c.cfg != nil && c.cfg.Log.Scan != nil {
		items, err := c.logPoller.TailScan(
			c.rootCtx,
			poller_log_tail.WithDedup(true),
			poller_log_tail.WithFile(c.cfg.Log.Scan.File),
			poller_log_tail.WithCommands(c.cfg.Log.Scan.Commands),
			poller_log_tail.WithLinesToTail(c.cfg.Log.Scan.LinesToTail),
			poller_log_tail.WithExtractTime(pkg_dmesg.ParseISOtimeWithError),
			poller_log_tail.WithProcessMatched(c.processMatched), // used for backfilling
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

// The dmesg component fetches the latest state from the dmesg tail scanner,
// rather than querying the log poller, which watches for the realtime dmesg streaming outputs.
// This is because the tail scanner is cheaper and can read historical logs
// in case the dmesg log watcher had restarted. It is more important that dmesg
// state calls DOES NOT miss any logs than having the logs available real-time.
// The real-time dmesg events can be fetched via the events API.
func (c *Component) States(ctx context.Context) ([]components.State, error) {
	s, err := c.TailScan()
	if err != nil {
		return nil, err
	}
	return s.States(), nil
}

// The dmesg component events returns the realtime events from the dmesg log poller.
// Returns `github.com/leptonai/gpud/poller.ErrNoData` if there is no event found.
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
