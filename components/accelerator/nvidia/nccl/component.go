// Package nccl monitors the NCCL status.
// Optional, enabled if the host has NVIDIA GPUs.
package nccl

import (
	"context"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

const Name = "accelerator-nvidia-nccl"

var _ apiv1.Component = &component{}

type component struct {
	kmsgSyncer  *kmsg.Syncer
	eventBucket eventstore.Bucket
}

func New(ctx context.Context, eventStore eventstore.Store) (apiv1.Component, error) {
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

func (c *component) States(ctx context.Context) ([]apiv1.State, error) {
	return []apiv1.State{
		{
			Healthy: true,
			Reason:  "no issue",
		},
	}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]apiv1.Event, error) {
	if c.eventBucket != nil {
		return c.eventBucket.Get(ctx, since)
	}
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	if c.kmsgSyncer != nil {
		c.kmsgSyncer.Close()
	}
	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}
