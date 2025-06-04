// Package fabricmanager tracks the NVIDIA fabric manager version and its activeness.
// And streams the fabric manager logs for any errors and events.
package fabricmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	netutil "github.com/leptonai/gpud/pkg/netutil"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-fabric-manager"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance

	checkFMExistsFunc func() bool
	checkFMActiveFunc func() bool

	eventBucket      eventstore.Bucket
	logLineProcessor *logLineProcessor

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstance: gpudInstance.NVMLInstance,

		checkFMExistsFunc: checkFMExists,
		checkFMActiveFunc: checkFMActive,
	}

	if gpudInstance.EventStore != nil {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}
	}

	if c.checkFMExistsFunc() && c.eventBucket != nil {
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

func (c *component) Tags() []string {
	return []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}
}

func (c *component) IsSupported() bool {
	if c.nvmlInstance == nil {
		return false
	}
	return c.nvmlInstance.NVMLExists() && c.nvmlInstance.ProductName() != ""
}

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
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()
	return lastCheckResult.HealthStates()
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

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}

	if !c.nvmlInstance.FabricManagerSupported() {
		cr.FabricManagerActive = false
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = c.nvmlInstance.ProductName() + " does not support fabric manager"
		return cr
	}

	if !c.checkFMExistsFunc() {
		cr.FabricManagerActive = false
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "nv-fabricmanager executable not found"
		return cr
	}

	active := c.checkFMActiveFunc()
	if !active {
		cr.FabricManagerActive = false

		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "nv-fabricmanager found but fabric manager service is not active"

		deviceCnt := len(c.nvmlInstance.Devices())
		if deviceCnt <= 2 {
			cr.health = apiv1.HealthStateTypeHealthy
			cr.reason = fmt.Sprintf("only %d GPU(s) detected, skipping fabric manager check", deviceCnt)
		}

		return cr
	}

	cr.FabricManagerActive = true
	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "fabric manager found and active"

	return cr
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

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	if cr.FabricManagerActive {
		return "fabric manager is active"
	}

	return "fabric manager is not active"
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{"data": string(b)}
	return apiv1.HealthStates{state}
}
