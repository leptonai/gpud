// Package peermem monitors the peermem module status.
// Optional, enabled if the host has NVIDIA GPUs.
package peermem

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	querypeermem "github.com/leptonai/gpud/pkg/nvidia-query/peermem"
)

const Name = "accelerator-nvidia-peermem"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	kmsgSyncer  *kmsg.Syncer
	eventBucket eventstore.Bucket

	lastMu   sync.RWMutex
	lastData *Data
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

	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:         cctx,
		cancel:      ccancel,
		kmsgSyncer:  kmsgSyncer,
		eventBucket: eventBucket,
	}, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			c.CheckOnce()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) States(ctx context.Context) ([]apiv1.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]apiv1.Event, error) {
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.kmsgSyncer.Close()
	c.eventBucket.Close()
	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking peermem")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	var err error
	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	d.PeerMemModuleOutput, err = querypeermem.CheckLsmodPeermemModule(cctx)
	ccancel()
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error checking peermem: %s", err)
		return
	}

	d.healthy = true
	if d.PeerMemModuleOutput != nil && d.PeerMemModuleOutput.IbcoreUsingPeermemModule {
		d.reason = "ibcore successfully loaded peermem module"
	} else {
		d.reason = "ibcore is not using peermem module"
	}
}

type Data struct {
	PeerMemModuleOutput *querypeermem.LsmodPeermemModuleOutput `json:"peer_mem_module_output,omitempty"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	healthy bool
	// tracks the reason of the last check
	reason string
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates() ([]apiv1.State, error) {
	if d == nil {
		return []apiv1.State{
			{
				Name:    Name,
				Health:  apiv1.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := apiv1.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  apiv1.StateHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.State{state}, nil
}
