// Package nccl monitors the NCCL status.
// Optional, enabled if the host has NVIDIA GPUs.
package nccl

import (
	"context"
	"runtime"
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
	ctx    context.Context
	cancel context.CancelFunc

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		c.kmsgSyncer, err = kmsg.NewSyncer(cctx, Match, c.eventBucket)
		if err != nil {
			ccancel()
			return nil, err
		}
	}

	return c, nil
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
