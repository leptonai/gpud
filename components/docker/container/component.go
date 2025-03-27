// Package container tracks the current containers from the docker runtime.
package container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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

	checkDependencyInstalled func() bool
	checkServiceActive       func(context.Context) (bool, error)

	// In case the docker daemon is not running, we ignore such errors as
	// 'Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?'.
	ignoreConnectionErrors bool

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, ignoreConnectionErrors bool) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:                      cctx,
		cancel:                   cancel,
		checkDependencyInstalled: checkDockerInstalled,
		checkServiceActive: func(ctx context.Context) (bool, error) {
			return systemd.CheckServiceActive(ctx, "docker")
		},
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

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates(c.ignoreConnectionErrors)
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
	if c.checkDependencyInstalled == nil || !c.checkDependencyInstalled() {
		return
	}

	// below are the checks in case "docker" is installed, thus requires activeness checks
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	running := checkDockerRunning(cctx)
	ccancel()
	if !running {
		d.err = errors.New("docker is installed but docker is not running")
		return
	}

	if c.checkServiceActive != nil {
		var err error
		cctx, ccancel = context.WithTimeout(c.ctx, 15*time.Second)
		d.DockerServiceActive, err = c.checkServiceActive(cctx)
		ccancel()
		if !d.DockerServiceActive || err != nil {
			d.err = fmt.Errorf("docker is installed but docker service is not active or failed to check (error %v)", err)
			return
		}
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 30*time.Second)
	d.Containers, d.err = listContainers(cctx)
	ccancel()

	// e.g.,
	// Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
	if d.err != nil && (strings.Contains(d.err.Error(), "Cannot connect to the Docker daemon") || strings.Contains(d.err.Error(), "the docker daemon running")) {
		d.connErr = true
	}
}

type Data struct {
	// DockerServiceActive is true if the docker service is active.
	DockerServiceActive bool `json:"docker_service_active"`

	// Containers is the list of containers.
	Containers []DockerContainer `json:"containers,omitempty"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
	// set to true if the error is the connection error to the docker daemon
	connErr bool `json:"-"`
}

func (d *Data) getReason() string {
	if d == nil || len(d.Containers) == 0 {
		return "no container found or docker is not running"
	}

	if d.err != nil {
		if isErrDockerClientVersionNewerThanDaemon(d.err) {
			return fmt.Sprintf("not supported; %s (needs upgrading docker daemon in the host)", d.err)
		}

		if d.connErr {
			return fmt.Sprintf("connection error to docker daemon -- %s", d.err)
		}

		return fmt.Sprintf("failed to list containers -- %s", d.err)
	}

	return fmt.Sprintf("total %d containers", len(d.Containers))
}

func (d *Data) getHealth(ignoreConnErr bool) (string, bool) {
	healthy := d == nil || d.err == nil
	if d != nil && d.err != nil && d.connErr && ignoreConnErr {
		healthy = true
	}
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates(ignoreConnErr bool) ([]components.State, error) {
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
	state.Health, state.Healthy = d.getHealth(ignoreConnErr)

	if len(d.Containers) == 0 { // no container found yet
		return []components.State{state}, nil
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
