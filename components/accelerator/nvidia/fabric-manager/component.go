// Package fabricmanager tracks the NVIDIA fabric manager version and its activeness.
// And streams the fabric manager logs for any errors and events.
package fabricmanager

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
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

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		checkFMExistsFunc: checkFMExists,
		checkFMActiveFunc: checkFMActive,
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}
	}

	if checkFMExists() && c.eventBucket != nil {
		w, err := newWatcher(defaultWatchCommands)
		if err != nil {
			ccancel()
			return nil, err
		}
		c.logLineProcessor = newLogLineProcessor(cctx, w, Match, c.eventBucket)
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
	if c.logLineProcessor == nil {
		return nil, nil
	}
	return c.logLineProcessor.getEvents(ctx, since)
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

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia fabric manager")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	if !c.checkFMExistsFunc() {
		d.FabricManagerActive = false
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = "nv-fabricmanager executable not found"
		return d
	}

	active := c.checkFMActiveFunc()
	if !active {
		d.FabricManagerActive = false
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = "nv-fabricmanager found but fabric manager service is not active"
		return d
	}

	d.FabricManagerActive = true
	d.health = apiv1.HealthStateTypeHealthy
	d.reason = "fabric manager found and active"

	return d
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

var _ components.CheckResult = &Data{}

type Data struct {
	// FabricManagerActive is true if the fabric manager is active.
	// By default, it checks the "nv-fabricmanager" default listening port 6666.
	FabricManagerActive bool `json:"fabric_manager_active"`

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

	if d.FabricManagerActive {
		return "fabric manager is active"
	}

	return "fabric manager is not active"
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
				Health: apiv1.HealthStateTypeHealthy,
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
