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
	pkgcontainerd "github.com/leptonai/gpud/pkg/containerd"
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
	listAllSandboxesFunc         func(ctx context.Context, endpoint string) ([]pkgcontainerd.PodSandbox, error)

	endpoint string

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		checkDependencyInstalledFunc: pkgcontainerd.CheckContainerdInstalled,
		checkSocketExistsFunc:        pkgcontainerd.CheckSocketExists,
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return systemd.IsActive("containerd")
		},
		checkContainerdRunningFunc: pkgcontainerd.CheckContainerdRunning,

		listAllSandboxesFunc: pkgcontainerd.ListAllSandboxes,

		endpoint: pkgcontainerd.DefaultContainerRuntimeEndpoint,
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
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()
	return lastCheckResult.getLastHealthStates()
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
	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// assume "containerd" is not installed, thus not needed to check its activeness
	if c.checkDependencyInstalledFunc == nil || !c.checkDependencyInstalledFunc() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "containerd not installed"
		return cr
	}

	// below are the checks in case "containerd" is installed, thus requires activeness checks
	if c.checkSocketExistsFunc != nil && !c.checkSocketExistsFunc() {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "containerd installed but socket file does not exist"
		return cr
	}

	if c.checkContainerdRunningFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
		running := c.checkContainerdRunningFunc(cctx)
		ccancel()
		if !running {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "containerd installed but not running"
			return cr
		}
	}

	if c.checkServiceActiveFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
		cr.ContainerdServiceActive, cr.err = c.checkServiceActiveFunc(cctx)
		ccancel()
		if !cr.ContainerdServiceActive || cr.err != nil {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "containerd installed but service is not active"
			return cr
		}
	}

	if c.listAllSandboxesFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
		cr.Pods, cr.err = c.listAllSandboxesFunc(cctx, c.endpoint)
		ccancel()
		if cr.err != nil {
			cr.health = apiv1.HealthStateTypeUnhealthy

			st, ok := status.FromError(cr.err)
			if ok {
				// this is the error from "ListSandboxStatus"
				// e.g.,
				// rpc error: code = Unimplemented desc = unknown service runtime.v1.RuntimeService
				if st.Code() == codes.Unimplemented {
					cr.health = apiv1.HealthStateTypeHealthy
					cr.reason = "containerd installed and active but containerd CRI is not enabled"
				} else {
					cr.reason = fmt.Sprintf("failed gRPC call to the containerd socket %s", st.Message())
				}
			} else {
				cr.reason = fmt.Sprintf("error listing pod sandbox status: %v", cr.err)
			}

			return cr
		}
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("found %d pod sandbox(es)", len(cr.Pods))

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// ContainerdServiceActive is true if the containerd service is active.
	ContainerdServiceActive bool `json:"containerd_service_active"`

	// Pods is the list of pods on the node.
	Pods []pkgcontainerd.PodSandbox `json:"pods,omitempty"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if len(cr.Pods) == 0 {
		return "no pod found"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.SetHeader([]string{"Namespace", "Pod", "Container", "State"})
	for _, pod := range cr.Pods {
		for _, container := range pod.Containers {
			table.Append([]string{pod.Namespace, pod.Name, container.Name, container.State})
		}
	}

	table.Render()

	return buf.String()
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthState() apiv1.HealthStateType {
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

func (cr *checkResult) getLastHealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	b, _ := json.Marshal(cr)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
