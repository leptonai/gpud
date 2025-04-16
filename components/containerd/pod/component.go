// Package pod tracks the current pods from the containerd CRI.
package pod

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
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

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		checkDependencyInstalledFunc: checkContainerdInstalled,
		checkSocketExistsFunc:        checkSocketExists,
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return systemd.IsActive("containerd")
		},
		checkContainerdRunningFunc: checkContainerdRunning,

		listAllSandboxesFunc: listAllSandboxes,

		endpoint: defaultContainerRuntimeEndpoint,
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
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking containerd pods", "endpoint", c.endpoint)
	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	// assume "containerd" is not installed, thus not needed to check its activeness
	if c.checkDependencyInstalledFunc == nil || !c.checkDependencyInstalledFunc() {
		d.health = apiv1.StateTypeHealthy
		d.reason = "containerd not installed"
		return d
	}

	// below are the checks in case "containerd" is installed, thus requires activeness checks
	if c.checkSocketExistsFunc != nil && !c.checkSocketExistsFunc() {
		d.health = apiv1.StateTypeUnhealthy
		d.reason = "containerd installed but socket file does not exist"
		return d
	}

	if c.checkContainerdRunningFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
		running := c.checkContainerdRunningFunc(cctx)
		ccancel()
		if !running {
			d.health = apiv1.StateTypeUnhealthy
			d.reason = "containerd installed but not running"
			return d
		}
	}

	if c.checkServiceActiveFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
		d.ContainerdServiceActive, d.err = c.checkServiceActiveFunc(cctx)
		ccancel()
		if !d.ContainerdServiceActive || d.err != nil {
			d.health = apiv1.StateTypeUnhealthy
			d.reason = "containerd installed but service is not active"
			return d
		}
	}

	if c.listAllSandboxesFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
		d.Pods, d.err = c.listAllSandboxesFunc(cctx, c.endpoint)
		ccancel()
		if d.err != nil {
			d.health = apiv1.StateTypeUnhealthy

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

			return d
		}
	}

	d.health = apiv1.StateTypeHealthy
	d.reason = fmt.Sprintf("found %d pod sandbox(es)", len(d.Pods))

	return d
}

var _ components.CheckResult = &Data{}

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
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (d *Data) String() string {
	if d == nil {
		return ""
	}
	if len(d.Pods) == 0 {
		return "no pod found"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.SetHeader([]string{"Namespace", "Pod", "Container", "State"})
	for _, pod := range d.Pods {
		for _, container := range pod.Containers {
			table.Append([]string{pod.Namespace, pod.Name, container.Name, container.State})
		}
	}

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
