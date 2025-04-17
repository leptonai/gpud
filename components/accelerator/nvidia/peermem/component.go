// Package peermem monitors the peermem module status.
// Optional, enabled if the host has NVIDIA GPUs.
package peermem

import (
	"bytes"
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
	"github.com/olekukonko/tablewriter"
)

const Name = "accelerator-nvidia-peermem"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)

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
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getLastHealthStates()
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
	log.Logger.Infow("checking nvidia gpu peermem")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil || !c.nvmlInstance.NVMLExists() {
		d.reason = "NVIDIA NVML is not loaded"
		d.health = apiv1.StateTypeHealthy
		return d
	}

	var err error
	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	d.PeerMemModuleOutput, err = querypeermem.CheckLsmodPeermemModule(cctx)
	ccancel()
	if err != nil {
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error checking peermem: %s", err)
		return d
	}

	d.health = apiv1.StateTypeHealthy
	if d.PeerMemModuleOutput != nil && d.PeerMemModuleOutput.IbcoreUsingPeermemModule {
		d.reason = "ibcore successfully loaded peermem module"
	} else {
		d.reason = "ibcore is not using peermem module"
	}

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	PeerMemModuleOutput *querypeermem.LsmodPeermemModuleOutput `json:"peer_mem_module_output,omitempty"`

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
	if d.PeerMemModuleOutput == nil {
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.Render()

	return buf.String()
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

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getLastHealthStates() apiv1.HealthStates {
	if d == nil {
		return apiv1.HealthStates{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),
		Health: d.health,
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
