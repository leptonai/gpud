// Package persistencemode tracks the NVIDIA persistence mode.
package persistencemode

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-persistence-mode"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance           nvml.InstanceV2
	getPersistenceModeFunc func(uuid string, dev device.Device) (nvidianvml.PersistenceMode, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, nvmlInstance nvml.InstanceV2) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:                    cctx,
		cancel:                 ccancel,
		nvmlInstance:           nvmlInstance,
		getPersistenceModeFunc: nvidianvml.GetPersistenceMode,
	}
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

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking persistence mode")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	devs := c.nvmlInstance.Devices()
	for uuid, dev := range devs {
		persistenceMode, err := c.getPersistenceModeFunc(uuid, dev)
		if err != nil {
			log.Logger.Errorw("error getting persistence mode for device", "uuid", uuid, "error", err)
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting persistence mode for device %s", uuid)
			return
		}

		d.PersistenceModes = append(d.PersistenceModes, persistenceMode)
	}

	d.healthy = true
	d.reason = fmt.Sprintf("all %d GPU(s) were checked, no persistence mode issue found", len(devs))
}

type Data struct {
	PersistenceModes []nvidianvml.PersistenceMode `json:"persistence_modes,omitempty"`

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

func (d *Data) getStates() ([]components.State, error) {
	if d == nil {
		return []components.State{
			{
				Name:    Name,
				Health:  components.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := components.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  components.StateHealthy,
	}
	if !d.healthy {
		state.Health = components.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
