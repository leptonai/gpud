// Package pod tracks the current pods from the kubelet read-only port.
package pod

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
)

// Name is the ID of the kubernetes pod component.
const Name = "kubelet"

const (
	defaultFailedCountThreshold = 5
	defaultKubeletReadOnlyPort  = 10255
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkDependencyInstalled func() bool
	checkKubeletRunning      func() bool
	kubeletReadOnlyPort      int

	failedCount          int
	failedCountThreshold int

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		checkDependencyInstalled: checkKubeletInstalled,
		checkKubeletRunning: func() bool {
			return netutil.IsPortOpen(defaultKubeletReadOnlyPort)
		},
		kubeletReadOnlyPort:  defaultKubeletReadOnlyPort,
		failedCount:          0,
		failedCountThreshold: defaultFailedCountThreshold,
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
	log.Logger.Infow("checking kubelet pods")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	if c.checkDependencyInstalled == nil || !c.checkDependencyInstalled() {
		// "kubelet" is not installed, thus not needed to check its activeness
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = "kubelet is not installed"
		return d
	}

	if c.checkKubeletRunning == nil || !c.checkKubeletRunning() {
		// "kubelet" is not running, thus not needed to check its activeness
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = "kubelet is installed but not running"
		return d
	}

	// below are the checks in case "kubelet" is installed and running, thus requires activeness checks
	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	d.NodeName, d.Pods, d.err = listPodsFromKubeletReadOnlyPort(cctx, c.kubeletReadOnlyPort)
	ccancel()

	if d.err != nil {
		c.failedCount++
	} else {
		c.failedCount = 0
	}

	if d.err == nil {
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = fmt.Sprintf("total %d pods (node %s)", len(d.Pods), d.NodeName)
	}

	if c.failedCount >= c.failedCountThreshold {
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = fmt.Sprintf("list pods from kubelet read-only port failed %d time(s)", c.failedCount)
	}

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	// KubeletServiceActive is true if the kubelet service is active.
	KubeletServiceActive bool `json:"kubelet_service_active"`
	// NodeName is the name of the node.
	NodeName string `json:"node_name,omitempty"`
	// Pods is the list of pods on the node.
	Pods []PodStatus `json:"pods,omitempty"`

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
		for _, container := range pod.ContainerStatuses {
			state := "unknown"
			if container.State.Running != nil {
				state = "running"
			} else if container.State.Terminated != nil {
				state = "terminated"
			} else if container.State.Waiting != nil {
				state = "waiting"
			}
			table.Append([]string{pod.Namespace, pod.Name, container.Name, state})
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

	if len(d.Pods) == 0 { // no pod found yet
		return apiv1.HealthStates{state}
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
