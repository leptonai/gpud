// Package kubelet tracks the current kubelet status.
package kubelet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/kubelet"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
)

// Name is the ID of the kubernetes pod component.
const Name = "kubelet"

const (
	defaultFailedCountThreshold = 5
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

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		checkDependencyInstalled: kubelet.CheckKubeletInstalled,
		checkKubeletRunning: func() bool {
			return netutil.IsPortOpen(kubelet.DefaultKubeletReadOnlyPort)
		},
		kubeletReadOnlyPort:  kubelet.DefaultKubeletReadOnlyPort,
		failedCount:          0,
		failedCountThreshold: defaultFailedCountThreshold,
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		"container",
		"kubelet",
	}
}

func (c *component) IsSupported() bool {
	return true
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
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking kubelet pods")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if c.checkDependencyInstalled == nil || !c.checkDependencyInstalled() {
		// "kubelet" is not installed, thus not needed to check its activeness
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "kubelet is not installed"
		return cr
	}

	if c.checkKubeletRunning == nil || !c.checkKubeletRunning() {
		// "kubelet" is not running, thus not needed to check its activeness
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "kubelet is installed but not running"
		return cr
	}

	// below are the checks in case "kubelet" is installed and running, thus requires activeness checks
	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	cr.NodeName, cr.Pods, cr.err = kubelet.ListPodsFromKubeletReadOnlyPort(cctx, c.kubeletReadOnlyPort)
	ccancel()

	if cr.err != nil {
		c.failedCount++
	} else {
		c.failedCount = 0
	}

	if cr.err == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("check success for node %s", cr.NodeName)
		log.Logger.Debugw(cr.reason, "node", cr.NodeName, "count", len(cr.Pods))
	}

	if c.failedCount >= c.failedCountThreshold {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "list pods from kubelet read-only port failed"
		log.Logger.Warnw(cr.reason, "failedCount", c.failedCount, "error", cr.err)
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// KubeletServiceActive is true if the kubelet service is active.
	KubeletServiceActive bool `json:"kubelet_service_active"`
	// NodeName is the name of the node.
	NodeName string `json:"node_name,omitempty"`
	// Pods is the list of pods on the node.
	Pods []kubelet.PodStatus `json:"pods,omitempty"`

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
	if len(cr.Pods) == 0 {
		return "no pod found"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.SetHeader([]string{"Namespace", "Pod", "Container", "State"})
	for _, pod := range cr.Pods {
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

	if len(cr.Pods) == 0 { // no pod found yet
		return apiv1.HealthStates{state}
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{"data": string(b)}
	return apiv1.HealthStates{state}
}
