// Package peermem monitors the peermem module status.
// Optional, enabled if the host has NVIDIA GPUs.
package peermem

import (
	"context"
	"encoding/json"
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
	querypeermem "github.com/leptonai/gpud/pkg/nvidia-query/peermem"
)

const Name = "accelerator-nvidia-peermem"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.InstanceV2

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	checkLsmodPeermemModuleFunc func(ctx context.Context) (*querypeermem.LsmodPeermemModuleOutput, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstance: gpudInstance.NVMLInstance,

		checkLsmodPeermemModuleFunc: querypeermem.CheckLsmodPeermemModule,
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

	return c, nil
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

	if c.nvmlInstance == nil {
		d.health = apiv1.StateTypeHealthy
		d.reason = "NVIDIA NVML instance is nil"
		return d
	}
	if !c.nvmlInstance.NVMLExists() {
		d.health = apiv1.StateTypeHealthy
		d.reason = "NVIDIA NVML is not loaded"
		return d
	}

	var err error
	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	d.PeerMemModuleOutput, err = c.checkLsmodPeermemModuleFunc(cctx)
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

	return fmt.Sprintf("ibcore using peermem module: %t", d.PeerMemModuleOutput.IbcoreUsingPeermemModule)
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
