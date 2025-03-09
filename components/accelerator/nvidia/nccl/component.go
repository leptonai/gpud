// Package nccl monitors the NCCL status.
// Optional, enabled if the host has NVIDIA GPUs.
package nccl

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_nccl_id "github.com/leptonai/gpud/components/accelerator/nvidia/nccl/id"
	"github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

func New(ctx context.Context, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(nvidia_nccl_id.Name)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	logLineProcessor, err := dmesg.NewLogLineProcessor(cctx, Match, eventBucket)
	if err != nil {
		ccancel()
		return nil, err
	}

	kmsgWatcher, err := kmsg.StartWatch(Match)
	if err != nil {
		ccancel()
		return nil, err
	}

	return &component{
		rootCtx:          ctx,
		cancel:           ccancel,
		logLineProcessor: logLineProcessor,
		eventBucket:      eventBucket,
		kmsgWatcher:      kmsgWatcher,
	}, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx          context.Context
	cancel           context.CancelFunc
	logLineProcessor *dmesg.LogLineProcessor
	eventBucket      eventstore.Bucket

	// experimental
	kmsgWatcher kmsg.Watcher
}

func (c *component) Name() string { return nvidia_nccl_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return []components.State{
		{
			Healthy: true,
			Reason:  "no issue",
		},
	}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.cancel()

	c.logLineProcessor.Close()
	c.eventBucket.Close()

	if c.kmsgWatcher != nil {
		c.kmsgWatcher.Close()
	}

	return nil
}
