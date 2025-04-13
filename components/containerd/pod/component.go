// Package pod tracks the current pods from the containerd CRI.
package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/systemd"
)

// Name is the ID of the containerd pod component.
const Name = "containerd-pod"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkDependencyInstalledFunc func() bool
	checkSocketExistsFunc        func() bool
	checkServiceActiveFunc       func(context.Context) (bool, error)
	checkContainerdRunningFunc   func(context.Context) bool
	listAllSandboxesFunc         func(ctx context.Context, endpoint string) ([]PodSandbox, error)

	endpoint string

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:    cctx,
		cancel: cancel,

		checkDependencyInstalledFunc: checkContainerdInstalled,
		checkSocketExistsFunc:        checkSocketExists,
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return systemd.IsActive("containerd")
		},
		checkContainerdRunningFunc: checkContainerdRunning,

		listAllSandboxesFunc: listAllSandboxes,

		endpoint: defaultContainerRuntimeEndpoint,
	}
	return c
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
	log.Logger.Infow("checking containerd pods", "endpoint", c.endpoint)
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	// assume "containerd" is not installed, thus not needed to check its activeness
	if c.checkDependencyInstalledFunc == nil || !c.checkDependencyInstalledFunc() {
		d.healthy = true
		d.reason = "containerd not installed"
		return
	}

	// below are the checks in case "containerd" is installed, thus requires activeness checks
	if c.checkSocketExistsFunc != nil && !c.checkSocketExistsFunc() {
		d.healthy = false
		d.reason = "containerd installed but socket file does not exist"
		return
	}

	if c.checkContainerdRunningFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
		running := c.checkContainerdRunningFunc(cctx)
		ccancel()
		if !running {
			d.healthy = false
			d.reason = "containerd installed but not running"
			return
		}
	}

	if c.checkServiceActiveFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
		d.ContainerdServiceActive, d.err = c.checkServiceActiveFunc(cctx)
		ccancel()
		if !d.ContainerdServiceActive || d.err != nil {
			d.healthy = false
			d.reason = "containerd installed but service is not active"
			return
		}
	}

	if c.listAllSandboxesFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
		d.Pods, d.err = c.listAllSandboxesFunc(cctx, c.endpoint)
		ccancel()
		if d.err != nil {
			d.healthy = false

			st, ok := status.FromError(d.err)
			if ok {
				// this is the error from "ListSandboxStatus"
				// e.g.,
				// rpc error: code = Unimplemented desc = unknown service runtime.v1.RuntimeService
				if st.Code() == codes.Unimplemented {
					d.reason = "containerd didn't enable CRI"
				} else {
					d.reason = fmt.Sprintf("failed gRPC call to the containerd socket %s", st.Message())
				}
			} else {
				d.reason = fmt.Sprintf("error listing pod sandbox status: %v", d.err)
			}

			return
		}
	}

	d.healthy = true
	d.reason = fmt.Sprintf("found %d pod sandbox(es)", len(d.Pods))
}

type Data struct {
	// ContainerdServiceActive is true if the containerd service is active.
	ContainerdServiceActive bool `json:"containerd_service_active"`

	// Pods is the list of pods on the node.
	Pods []PodSandbox `json:"pods,omitempty"`

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
				Name:              Name,
				Health:            apiv1.StateTypeHealthy,
				DeprecatedHealthy: true,
				Reason:            "no data yet",
			},
		}, nil
	}

	state := apiv1.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		DeprecatedHealthy: d.healthy,
		Health:            apiv1.StateTypeHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateTypeUnhealthy
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.State{state}, nil
}
