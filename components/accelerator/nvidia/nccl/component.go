// Package nccl monitors the NCCL status.
// Optional, enabled if the host has NVIDIA GPUs.
package nccl

import (
	"context"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

const (
	Name = "accelerator-nvidia-nccl"
)

var _ components.Component = &component{}

type component struct {
	kmsgSyncer  *kmsg.Syncer
	eventBucket eventstore.Bucket
}

func New(ctx context.Context, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		return nil, err
	}

	kmsgSyncer, err := kmsg.NewSyncer(ctx, Match, eventBucket)
	if err != nil {
		return nil, err
	}

	return &component{
		kmsgSyncer:  kmsgSyncer,
		eventBucket: eventBucket,
	}, nil
}

func (c *component) Name() string { return Name }

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
	c.kmsgSyncer.Close()
	c.eventBucket.Close()
	return nil
}
