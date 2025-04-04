// Package pod tracks the current pods from the containerd CRI.
package pod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

	checkDependencyInstalled func() bool
	checkServiceActive       func(context.Context) (bool, error)

	endpoint string

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:                      cctx,
		cancel:                   cancel,
		checkDependencyInstalled: checkContainerdInstalled,
		checkServiceActive: func(ctx context.Context) (bool, error) {
			return systemd.CheckServiceActive(ctx, "containerd")
		},
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

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

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
	if c.checkDependencyInstalled == nil || !c.checkDependencyInstalled() {
		return
	}

	// below are the checks in case "containerd" is installed, thus requires activeness checks
	if !checkSocketExists() {
		d.err = errors.New("containerd is installed but containerd socket file does not exist")
		return
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	running := checkContainerdRunning(cctx)
	ccancel()
	if !running {
		d.err = errors.New("containerd is installed but containerd is not running")
		return
	}

	if c.checkServiceActive != nil {
		var err error
		cctx, ccancel = context.WithTimeout(c.ctx, 15*time.Second)
		d.ContainerdServiceActive, err = c.checkServiceActive(cctx)
		ccancel()
		if !d.ContainerdServiceActive || err != nil {
			d.err = fmt.Errorf("containerd is installed but containerd service is not active or failed to check (error %v)", err)
			return
		}
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 30*time.Second)
	d.Pods, d.err = listSandboxStatus(cctx, c.endpoint)
	ccancel()
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
}

func (d *Data) getReason() string {
	if d == nil || len(d.Pods) == 0 {
		r := "no pod sandbox found or containerd is not running"
		if d.err != nil {
			r += fmt.Sprintf(", error: %v", d.err)
		}
		return r
	}

	reason := fmt.Sprintf("total %d pod sandboxe(s)", len(d.Pods))

	if d.err != nil {
		st, ok := status.FromError(d.err)
		if ok {
			// this is the error from "ListSandboxStatus"
			// e.g.,
			// rpc error: code = Unimplemented desc = unknown service runtime.v1.RuntimeService
			if st.Code() == codes.Unimplemented {
				reason = "containerd didn't enable CRI"
			} else {
				reason = fmt.Sprintf("failed gRPC call to the containerd socket %s", st.Message())
			}
		} else {
			reason = fmt.Sprintf("failed to list pod sandbox status %v", d.err)
		}
	}

	return reason
}

func (d *Data) getHealth() (string, bool) {
	if d != nil && d.err != nil {
		return components.StateUnhealthy, false
	}
	return components.StateHealthy, true
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
		Reason: d.getReason(),
		Error:  d.getError(),
	}
	state.Health, state.Healthy = d.getHealth()

	if d == nil || len(d.Pods) == 0 { // no pod found yet
		return []components.State{state}, nil
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
