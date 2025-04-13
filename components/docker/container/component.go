// Package container tracks the current containers from the docker runtime.
package container

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

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

func New(ctx context.Context, ignoreConnectionErrors bool) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:    cctx,
		cancel: cancel,

		checkDependencyInstalledFunc: checkDockerInstalled,
		checkServiceActiveFunc: func() (bool, error) {
			return systemd.IsActive("docker")
		},
		checkDockerRunningFunc: checkDockerRunning,
		listContainersFunc:     listContainers,

		ignoreConnectionErrors: ignoreConnectionErrors,
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
	log.Logger.Infow("checking docker containers")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	// assume "docker" is not installed, thus not needed to check its activeness
	if c.checkDependencyInstalledFunc == nil || !c.checkDependencyInstalledFunc() {
		return
	}

	// below are the checks in case "docker" is installed, thus requires activeness checks
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	running := c.checkDockerRunningFunc(cctx)
	ccancel()
	if !running {
		d.healthy = false
		d.reason = "docker installed but docker is not running"
		return
	}

	if c.checkServiceActiveFunc != nil {
		d.DockerServiceActive, d.err = c.checkServiceActiveFunc()
		if !d.DockerServiceActive || d.err != nil {
			d.healthy = false
			d.reason = fmt.Sprintf("docker installed but docker service is not active or failed to check (error %v)", d.err)
			return
		}
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 30*time.Second)
	d.Containers, d.err = c.listContainersFunc(cctx)
	ccancel()

	if d.err != nil {
		d.healthy = false
		d.reason = fmt.Sprintf("error listing containers -- %s", d.err)

		if isErrDockerClientVersionNewerThanDaemon(d.err) {
			d.reason = fmt.Sprintf("not supported; %s (needs upgrading docker daemon in the host)", d.err)
		}

		// e.g.,
		// Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
		if strings.Contains(d.err.Error(), "Cannot connect to the Docker daemon") || strings.Contains(d.err.Error(), "the docker daemon running") {
			d.healthy = c.ignoreConnectionErrors
			d.reason = fmt.Sprintf("connection error to docker daemon -- %s", d.err)
		}
	} else {
		d.healthy = true
		d.reason = fmt.Sprintf("total %d container(s)", len(d.Containers))
	}
}

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
