// Package fabricmanager tracks the NVIDIA fabric manager version and its activeness.
// And streams the fabric manager logs for any errors and events.
package fabricmanager

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	netutil "github.com/leptonai/gpud/pkg/netutil"
)

const Name = "accelerator-nvidia-fabric-manager"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkFMExistsFunc func() bool
	checkFMActiveFunc func() bool

	eventBucket      eventstore.Bucket
	logLineProcessor *logLineProcessor

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)

	var llp *logLineProcessor
	if checkFMExists() {
		w, err := newWatcher(defaultWatchCommands)
		if err != nil {
			ccancel()
			return nil, err
		}
		llp = newLogLineProcessor(cctx, w, Match, eventBucket)
	}

	return &component{
		ctx:    cctx,
		cancel: ccancel,

		checkFMExistsFunc: checkFMExists,
		checkFMActiveFunc: checkFMActive,

		eventBucket:      eventBucket,
		logLineProcessor: llp,
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

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	if c.logLineProcessor != nil {
		return c.logLineProcessor.getEvents(ctx, since)
	}
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.logLineProcessor != nil {
		c.logLineProcessor.close()
	}
	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking power")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	if !c.checkFMExistsFunc() {
		d.FabricManagerActive = false
		d.healthy = true
		d.reason = "nv-fabricmanager executable not found"
		return
	}

	active := c.checkFMActiveFunc()
	if !active {
		d.FabricManagerActive = false
		d.healthy = false
		d.reason = "nv-fabricmanager found but fabric manager service is not active"
		return
	}

	d.FabricManagerActive = true
	d.healthy = true
	d.reason = "fabric manager found and active"
}

// checkFMExists returns true if the fabric manager executable is found in the system.
func checkFMExists() bool {
	p, err := exec.LookPath("nv-fabricmanager")
	if err != nil {
		return false
	}
	return p != ""
}

// FM_CMD_PORT_NUMBER=6666
// ref. https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html#the-fabric-manager-api-tcp-port
const defaultFabricManagerPort = 6666

// checkFMActive returns true if the fabric manager is active by checking its listening port.
// alternatively, we can check dbus connection to see if the systemd  "nvidia-fabricmanager" service is active
func checkFMActive() bool {
	return netutil.IsPortOpen(defaultFabricManagerPort)
}

type Data struct {
	// FabricManagerActive is true if the fabric manager is active.
	// By default, it checks the "nv-fabricmanager" default listening port 6666.
	FabricManagerActive bool `json:"fabric_manager_active"`

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
