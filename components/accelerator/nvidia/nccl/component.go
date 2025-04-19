// Package nccl monitors the NCCL status.
// Optional, enabled if the host has NVIDIA GPUs.
package nccl

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-nccl"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.InstanceV2

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	readAllKmsg func(context.Context) ([]kmsg.Message, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: gpudInstance.NVMLInstance,
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		if os.Geteuid() == 0 {
			c.kmsgSyncer, err = kmsg.NewSyncer(cctx, Match, c.eventBucket)
			if err != nil {
				ccancel()
				return nil, err
			}
		}
	}

	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		c.readAllKmsg = kmsg.ReadAll
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	// do not need periodic kmsg checks since it already has a watcher
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	return apiv1.HealthStates{
		{
			Component: Name,
			Health:    apiv1.HealthStateTypeHealthy,
			Reason:    "no issue",
		},
	}
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

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu nccl")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = "NVIDIA NVML instance is nil"
		return d
	}
	if !c.nvmlInstance.NVMLExists() {
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = "NVIDIA NVML is not loaded"
		return d
	}

	if c.readAllKmsg == nil {
		d.reason = "kmsg reader is not set"
		d.health = apiv1.HealthStateTypeHealthy
		return d
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	kmsgs, err := c.readAllKmsg(cctx)
	ccancel()
	if err != nil {
		d.err = err
		d.reason = fmt.Sprintf("failed to read kmsg: %v", err)
		d.health = apiv1.HealthStateTypeUnhealthy
		return d
	}

	for _, kmsg := range kmsgs {
		ev, _ := Match(kmsg.Message)
		if ev == "" {
			continue
		}
		d.MatchedKmsgs = append(d.MatchedKmsgs, kmsg)
	}

	d.reason = fmt.Sprintf("matched %d kmsg(s)", len(d.MatchedKmsgs))
	d.health = apiv1.HealthStateTypeHealthy

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	MatchedKmsgs []kmsg.Message `json:"matched_kmsgs"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (d *Data) String() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("matched %d kmsg(s)", len(d.MatchedKmsgs))
}

func (d *Data) Summary() string {
	if d == nil {
		return ""
	}
	return d.reason
}

func (d *Data) HealthState() apiv1.HealthStateType {
	if d == nil {
		return ""
	}
	return d.health
}
