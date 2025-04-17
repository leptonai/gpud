// Package nccl monitors the NCCL status.
// Optional, enabled if the host has NVIDIA GPUs.
package nccl

import (
	"context"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

const Name = "accelerator-nvidia-nccl"

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

func (c *component) HealthStates(ctx context.Context) (apiv1.HealthStates, error) {
	return apiv1.HealthStates{
		{
			Health: apiv1.StateTypeHealthy,
			Reason: "no issue",
		},
	}, nil
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if c.eventBucket == nil {
		return nil, nil
	}
	return c.eventBucket.Get(ctx, since)
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
