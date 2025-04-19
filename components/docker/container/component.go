// Package container tracks the current containers from the docker runtime.
package container

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/systemd"
)

// Name is the ID of the Docker container component.
const Name = "docker-container"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkDependencyInstalledFunc func() bool
	checkServiceActiveFunc       func() (bool, error)
	checkDockerRunningFunc       func(context.Context) bool
	listContainersFunc           func(context.Context) ([]DockerContainer, error)

	// In case the docker daemon is not running, we ignore such errors as
	// 'Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?'.
	ignoreConnectionErrors bool

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		checkDependencyInstalledFunc: checkDockerInstalled,
		checkServiceActiveFunc: func() (bool, error) {
			return systemd.IsActive("docker")
		},
		checkDockerRunningFunc: checkDockerRunning,
		listContainersFunc:     listContainers,

		ignoreConnectionErrors: true,
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
	log.Logger.Infow("checking docker containers")
	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	// assume "docker" is not installed, thus not needed to check its activeness
	if c.checkDependencyInstalledFunc == nil || !c.checkDependencyInstalledFunc() {
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = "docker not installed"
		return d
	}

	// below are the checks in case "docker" is installed, thus requires activeness checks
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	running := c.checkDockerRunningFunc(cctx)
	ccancel()
	if !running {
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = "docker installed but docker is not running"
		return d
	}

	if c.checkServiceActiveFunc != nil {
		d.DockerServiceActive, d.err = c.checkServiceActiveFunc()
		if !d.DockerServiceActive || d.err != nil {
			d.health = apiv1.HealthStateTypeUnhealthy
			d.reason = fmt.Sprintf("docker installed but docker service is not active or failed to check (error %v)", d.err)
			return d
		}
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 30*time.Second)
	d.Containers, d.err = c.listContainersFunc(cctx)
	ccancel()

	if d.err != nil {
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = fmt.Sprintf("error listing containers -- %s", d.err)

		if isErrDockerClientVersionNewerThanDaemon(d.err) {
			d.reason = fmt.Sprintf("not supported; %s (needs upgrading docker daemon in the host)", d.err)
		}

		// e.g.,
		// Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
		if strings.Contains(d.err.Error(), "Cannot connect to the Docker daemon") || strings.Contains(d.err.Error(), "the docker daemon running") {
			if c.ignoreConnectionErrors {
				d.health = apiv1.HealthStateTypeHealthy
			} else {
				d.health = apiv1.HealthStateTypeUnhealthy
			}
			d.reason = fmt.Sprintf("connection error to docker daemon -- %s", d.err)
		}
	} else {
		d.health = apiv1.HealthStateTypeHealthy
		d.reason = fmt.Sprintf("total %d container(s)", len(d.Containers))
	}

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	// DockerServiceActive is true if the docker service is active.
	DockerServiceActive bool `json:"docker_service_active"`

	// Containers is the list of containers.
	Containers []DockerContainer `json:"containers,omitempty"`

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
	if len(d.Containers) == 0 {
		return "no container found"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"ID", "Name", "Image", "State"})
	for _, container := range d.Containers {
		table.Append([]string{container.ID, container.Name, container.Image, container.State})
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

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
