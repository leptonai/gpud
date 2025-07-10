// Package containerd tracks the current containerd status.
package containerd

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
	pkgcontainerd "github.com/leptonai/gpud/pkg/containerd"
	"github.com/leptonai/gpud/pkg/kubelet"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/systemd"
)

// Name is the ID of the containerd component.
const (
	Name                       = "containerd"
	DanglingDegradedThreshold  = 5
	DanglingUnhealthyThreshold = 10
)

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

func (c *component) Tags() []string {
	return []string{
		"container",
		Name,
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

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "ok"
	if c.listAllSandboxesFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
		cr.Pods, cr.err = c.listAllSandboxesFunc(cctx, c.endpoint)
		ccancel()
		if cr.err != nil {
			if pkgcontainerd.IsErrUnimplemented(cr.err) {
				cr.health = apiv1.HealthStateTypeHealthy
				cr.reason = "containerd installed and active but containerd CRI is not enabled"
			} else {
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error listing pod sandbox status"
				log.Logger.Warnw(cr.reason, "error", cr.err)
			}
			return cr
		}

		var danglingCount int
		cctx, ccancel = context.WithTimeout(c.ctx, 30*time.Second)
		_, kubeletPods, err := kubelet.ListPodsFromKubeletReadOnlyPort(cctx, kubelet.DefaultKubeletReadOnlyPort)
		ccancel()
		if err != nil {
			log.Logger.Errorf("error listing pods from kubelet: %v", err)
		} else {
			danglingCount = danglingPodCount(cr.Pods, kubeletPods)
		}
		if danglingCount > DanglingUnhealthyThreshold {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = fmt.Sprintf("node has %v dangling pods, unhealthy threshold %v", danglingCount, DanglingUnhealthyThreshold)
			cr.suggestedAction = &apiv1.SuggestedActions{
				Description:   "too many dangling pod",
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem},
			}
			return cr
		} else if danglingCount > DanglingDegradedThreshold {
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("node has %v dangling pods, consider reboot system to recover, degraded threshold %v", danglingCount, DanglingDegradedThreshold)
			cr.suggestedAction = &apiv1.SuggestedActions{
				Description: "too many dangling pod",
			}
			return cr
		} else if danglingCount != 0 {
			cr.reason = fmt.Sprintf("node has %v dangling pods", danglingCount)
		}
	}
	log.Logger.Debugw(cr.reason, "count", len(cr.Pods))

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
	reason          string
	suggestedAction *apiv1.SuggestedActions
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
		Time:             metav1.NewTime(cr.ts),
		Component:        Name,
		Name:             Name,
		Reason:           cr.reason,
		Error:            cr.getError(),
		Health:           cr.health,
		SuggestedActions: cr.suggestedAction,
	}

	if len(cr.Pods) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}

func danglingPodCount(containerdPods []pkgcontainerd.PodSandbox, kubeletPods []kubelet.PodStatus) int {
	var danglingCount int
	if kubeletPods == nil {
		return danglingCount
	}
	podMap := make(map[string]struct{})
	for _, pod := range kubeletPods {
		podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		podMap[podKey] = struct{}{}
	}
	for _, pod := range containerdPods {
		if pod.State != "SANDBOX_READY" {
			continue
		}
		podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		if _, ok := podMap[podKey]; !ok {
			danglingCount++
		}
	}

	return danglingCount
}
