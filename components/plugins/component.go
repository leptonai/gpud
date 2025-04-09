// Package plugins provides a component for running plugins.
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/plugins"
)

var _ components.Component = &component{}

type component struct {
	componentName string
	plugin        *plugins.Plugin

	ctx    context.Context
	cancel context.CancelFunc

	eventBucket eventstore.Bucket

	lastMu   sync.RWMutex
	lastData *Data
}

func New(plugin *plugins.Plugin, eventStore eventstore.Store) (components.Component, error) {
	if err := plugin.Validate(); err != nil {
		return nil, err
	}

	eventBucket, err := eventStore.Bucket(plugin.ComponentName())
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &component{
		componentName: plugin.ComponentName(),
		plugin:        plugin,

		ctx:    ctx,
		cancel: cancel,

		eventBucket: eventBucket,
	}, nil
}

func (c *component) Name() string { return c.plugin.Name }

func (c *component) Start() error {
	if c.plugin == nil {
		return nil
	}
	if c.plugin.Interval.Duration == 0 {
		c.CheckOnce()
		return nil
	}

	go func() {
		ticker := time.NewTicker(c.plugin.Interval.Duration)
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
	return lastData.getStates(c.componentName)
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	if c.eventBucket != nil {
		return c.eventBucket.Get(ctx, since)
	}
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component", "component", c.componentName)

	c.cancel()

	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking info")
	d := Data{
		componentName: c.componentName,
		ts:            time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	cctx, cancel := context.WithTimeout(c.ctx, c.plugin.Timeout.Duration)
	d.err = c.plugin.CheckOnce(cctx)
	cancel()
	if d.err != nil {
		d.healthy = false
		d.reason = fmt.Sprintf("error running check")
		return
	}

	d.healthy = true
	d.reason = "success"
}

type Data struct {
	// TODO

	componentName string

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

func (d *Data) getStates(componentName string) ([]components.State, error) {
	if d == nil {
		return []components.State{
			{
				Name:    componentName,
				Health:  components.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := components.State{
		Name:   d.componentName,
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
