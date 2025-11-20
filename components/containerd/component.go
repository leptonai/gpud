// Package containerd tracks the current containerd status.
package containerd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	componentkubelet "github.com/leptonai/gpud/components/kubelet"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/systemd"
)

// Name is the ID of the containerd component.
const (
	Name                        = "containerd"
	DanglingDegradedThreshold   = 5
	DanglingUnhealthyThreshold  = 10
	defaultContainerdConfigPath = "/etc/containerd/config.toml"

	defaultActivenssCheckUptimeThreshold = 5 * time.Minute

	// Containerd config strings to check for NVIDIA runtime configuration
	containerdConfigNvidiaDefaultRuntime = `default_runtime_name = "nvidia"`
	containerdConfigNvidiaRuntimePlugin  = `plugins."io.containerd.grpc.v1.cri".containerd.runtimes.nvidia`
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance

	getTimeNowFunc                    func() time.Time
	containerToolkitCreationThreshold time.Duration
	getContainerdConfigFunc           func() ([]byte, error)

	checkDependencyInstalledFunc  func() bool
	checkSocketExistsFunc         func() bool
	checkContainerdRunningFunc    func(context.Context) bool
	checkServiceActiveFunc        func(context.Context) (bool, error)
	getContainerdUptimeFunc       func() (*time.Duration, error)
	activenssCheckUptimeThreshold time.Duration

	listAllSandboxesFunc func(ctx context.Context, endpoint string) ([]PodSandbox, error)

	endpoint string

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstance: gpudInstance.NVMLInstance,

		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		containerToolkitCreationThreshold: 10 * time.Minute,
		getContainerdConfigFunc: func() ([]byte, error) {
			return os.ReadFile(defaultContainerdConfigPath)
		},

		checkDependencyInstalledFunc: checkContainerdInstalled,
		checkSocketExistsFunc:        CheckSocketExists,
		checkContainerdRunningFunc:   CheckContainerdRunning,
		checkServiceActiveFunc: func(ctx context.Context) (bool, error) {
			return systemd.IsActive("containerd")
		},
		getContainerdUptimeFunc: func() (*time.Duration, error) {
			return systemd.GetUptime("containerd")
		},
		activenssCheckUptimeThreshold: defaultActivenssCheckUptimeThreshold,

		listAllSandboxesFunc: ListAllSandboxes,

		endpoint: DefaultContainerRuntimeEndpoint,
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

// checkContainerdActiveness checks if containerd is active and running.
// It returns true if containerd is active and ready, false otherwise.
// If false is returned, the checkResult cr will be populated with the appropriate health state and reason.
func (c *component) checkContainerdActiveness(cr *checkResult) bool {
	if c.checkSocketExistsFunc != nil && !c.checkSocketExistsFunc() {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "containerd installed but socket file does not exist"
		return false
	}

	if c.checkContainerdRunningFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
		running := c.checkContainerdRunningFunc(cctx)
		ccancel()
		if !running {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "containerd installed but not running"
			return false
		}
	}

	if c.checkServiceActiveFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
		cr.ContainerdServiceActive, cr.err = c.checkServiceActiveFunc(cctx)
		ccancel()
		if !cr.ContainerdServiceActive || cr.err != nil {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "containerd installed but service is not active"
			return false
		}
	}

	return true
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking containerd pods", "endpoint", c.endpoint)
	cr := &checkResult{
		ts: c.getTimeNowFunc(),
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
	if ok := c.checkContainerdActiveness(cr); !ok {
		log.Logger.Warnw(
			"containerd is not active, but checking uptime",
			"health", cr.health,
			"reason", cr.reason,
		)

		// not active, but need to handle edge case where containerd is just installed
		// and has not fully started yet
		if c.getContainerdUptimeFunc != nil {
			uptime, err := c.getContainerdUptimeFunc()
			if err != nil {
				log.Logger.Warnw("error getting containerd uptime", "error", err)
				return cr
			}
			if uptime == nil {
				log.Logger.Warnw("containerd uptime is nil")
				return cr
			}

			if uptime.Seconds() < c.activenssCheckUptimeThreshold.Seconds() {
				// set it to healthy if uptime is less than the threshold
				log.Logger.Warnw("containerd is not active, but has not been running for long enough",
					"uptime", *uptime,
					"threshold", c.activenssCheckUptimeThreshold,
				)
				cr.health = apiv1.HealthStateTypeHealthy
				cr.reason += "; has not been running for long enough"
			}
		}

		return cr
	}
	// now that we know containerd is active, we can check its states

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "ok"
	if c.listAllSandboxesFunc != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
		cr.Pods, cr.err = c.listAllSandboxesFunc(cctx, c.endpoint)
		ccancel()
		if cr.err != nil {
			if IsErrUnimplemented(cr.err) {
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
		_, kubeletPods, err := componentkubelet.ListPodsFromKubeletReadOnlyPort(cctx, componentkubelet.DefaultKubeletReadOnlyPort)
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
			return cr
		} else if danglingCount != 0 {
			cr.reason = fmt.Sprintf("node has %v dangling pods", danglingCount)
		}
	}
	log.Logger.Debugw(cr.reason, "count", len(cr.Pods))

	if c.nvmlInstance != nil &&
		c.nvmlInstance.NVMLExists() &&
		c.nvmlInstance.ProductName() != "" &&
		len(cr.Pods) > 0 &&
		c.getContainerdConfigFunc != nil {

		// check "nvidia-container-toolkit-daemonset" pod
		// whose containers include "nvidia-container-toolkit-ctr"
		// which performs containerd.toml configuration updates
		// if the gpu-operator is running successfully
		toolkitCtrCreatedAt := time.Time{}

		for _, pod := range cr.Pods {
			if !strings.Contains(pod.Name, "nvidia-container-toolkit-daemonset") {
				continue
			}
			if pod.State != "SANDBOX_READY" {
				continue
			}
			toolkitCtrCreatedAt = time.Unix(0, pod.CreatedAt)
			break
		}

		if toolkitCtrCreatedAt.IsZero() {
			reason := "nvidia GPUs found but nvidia-container-toolkit pod is not found"

			cr.appendReason(reason)
			log.Logger.Warnw(reason)

		} else {
			now := c.getTimeNowFunc()
			elapsed := now.Sub(toolkitCtrCreatedAt)

			// been running long enough
			if elapsed > c.containerToolkitCreationThreshold {
				config, err := c.getContainerdConfigFunc()
				if err != nil {
					reason := "error getting containerd config"

					cr.appendReason(reason)
					log.Logger.Warnw(reason)

				} else if !bytes.Contains(config, []byte(containerdConfigNvidiaDefaultRuntime)) ||
					!bytes.Contains(config, []byte(containerdConfigNvidiaRuntimePlugin)) {
					reason := fmt.Sprintf("nvidia-container-toolkit pod is running but %s is missing NVIDIA runtime configuration", defaultContainerdConfigPath)

					cr.appendReason(reason)
					log.Logger.Warnw(reason)

					cr.health = apiv1.HealthStateTypeUnhealthy
				} else {
					log.Logger.Debugw("containerd config contains nvidia")
				}
			} else {
				log.Logger.Debugw("nvidia-container-toolkit pod is running but not long enough", "elapsed", elapsed)
			}
		}
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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

func (cr *checkResult) appendReason(reason string) {
	if cr == nil || reason == "" {
		return
	}

	if cr.reason == "" || cr.reason == "ok" {
		cr.reason = reason
		return
	}

	cr.reason += "; " + reason
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

func danglingPodCount(containerdPods []PodSandbox, kubeletPods []componentkubelet.PodStatus) int {
	var danglingCount int
	if len(kubeletPods) == 0 {
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
