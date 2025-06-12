// Package docker tracks the current docker status.
package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgdocker "github.com/leptonai/gpud/pkg/docker"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/systemd"
)

// Name is the ID of the Docker component.
const Name = "docker"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkDependencyInstalledFunc func() bool
	checkServiceActiveFunc       func() (bool, error)
	checkDockerRunningFunc       func(context.Context) bool
	listContainersFunc           func(context.Context) ([]pkgdocker.DockerContainer, error)

	// In case the docker daemon is not running, we ignore such errors as
	// 'Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?'.
	ignoreConnectionErrors bool

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		checkDependencyInstalledFunc: pkgdocker.CheckDockerInstalled,
		checkServiceActiveFunc: func() (bool, error) {
			return systemd.IsActive("docker")
		},
		checkDockerRunningFunc: pkgdocker.CheckDockerRunning,
		listContainersFunc:     pkgdocker.ListContainers,

		ignoreConnectionErrors: true,
	}
	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		"container",
		"docker",
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
	log.Logger.Infow("checking docker containers")
	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// assume "docker" is not installed, thus not needed to check its activeness
	if c.checkDependencyInstalledFunc == nil || !c.checkDependencyInstalledFunc() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "docker not installed"
		return cr
	}

	// below are the checks in case "docker" is installed, thus requires activeness checks
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	running := c.checkDockerRunningFunc(cctx)
	ccancel()
	if !running {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "docker installed but docker is not running"
		return cr
	}

	if c.checkServiceActiveFunc != nil {
		cr.DockerServiceActive, cr.err = c.checkServiceActiveFunc()
		if !cr.DockerServiceActive || cr.err != nil {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "docker installed but docker service is not active or failed to check"
			log.Logger.Errorw(cr.reason, "error", cr.err)
			return cr
		}
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 30*time.Second)
	cr.Containers, cr.err = c.listContainersFunc(cctx)
	ccancel()

	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error listing containers"

		if pkgdocker.IsErrDockerClientVersionNewerThanDaemon(cr.err) {
			cr.reason = "not supported; needs upgrading docker daemon in the host"
		}

		// e.g.,
		// Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
		if strings.Contains(cr.err.Error(), "Cannot connect to the Docker daemon") || strings.Contains(cr.err.Error(), "the docker daemon running") {
			if c.ignoreConnectionErrors {
				cr.health = apiv1.HealthStateTypeHealthy
			} else {
				cr.health = apiv1.HealthStateTypeUnhealthy
			}
			cr.reason = "connection error to docker daemon"
		}
	} else {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "ok"
		log.Logger.Debugw("ok", "count", len(cr.Containers))
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// DockerServiceActive is true if the docker service is active.
	DockerServiceActive bool `json:"docker_service_active"`

	// Containers is the list of containers.
	Containers []pkgdocker.DockerContainer `json:"containers,omitempty"`

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
	if len(cr.Containers) == 0 {
		return "no container found"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"ID", "Name", "Image", "State"})
	for _, container := range cr.Containers {
		table.Append([]string{container.ID, container.Name, container.Image, container.State})
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

	if len(cr.Containers) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
