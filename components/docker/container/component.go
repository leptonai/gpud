// Package container tracks the current containers from the docker runtime.
package container

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	docker_types "github.com/docker/docker/api/types"
	docker_container "github.com/docker/docker/api/types/container"
	docker_client "github.com/docker/docker/client"

	"github.com/leptonai/gpud/components"
	docker_container_id "github.com/leptonai/gpud/components/docker/container/id"
	pkg_file "github.com/leptonai/gpud/pkg/file"
	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	// In case the docker daemon is not running, we ignore such errors as
	// 'Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?'.
	ignoreConnectionErrors bool

	lastMu   sync.RWMutex
	lastData Data
}

func New(ctx context.Context, ignoreConnectionErrors bool) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:                    cctx,
		cancel:                 cancel,
		ignoreConnectionErrors: ignoreConnectionErrors,
	}
	return c
}

var _ components.Component = &component{}

func (c *component) Name() string { return docker_container_id.Name }

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

	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	d.DockerPidFound = process.CheckRunningByPid(cctx, "docker")
	ccancel()

	cctx, ccancel = context.WithTimeout(c.ctx, 30*time.Second)
	d.Containers, d.err = listContainers(cctx)
	ccancel()

	// e.g.,
	// Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
	if d.err != nil && (strings.Contains(d.err.Error(), "Cannot connect to the Docker daemon") || strings.Contains(d.err.Error(), "the docker daemon running")) {
		d.connErr = true
	}

	if d.err != nil {
		components_metrics.SetGetFailed(docker_container_id.Name)
	} else {
		components_metrics.SetGetSuccess(docker_container_id.Name)
	}

	c.lastMu.Lock()
	c.lastData = d
	c.lastMu.Unlock()
}

type Data struct {
	// DockerPidFound is true if the docker pid is found.
	DockerPidFound bool `json:"docker_pid_found,omitempty"`
	// Containers is the list of containers.
	Containers []DockerContainer `json:"containers,omitempty"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
	// set to true if the error is the connection error to the docker daemon
	connErr bool `json:"-"`
}

func (d *Data) Reason() string {
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
	healthy := d.err == nil
	if d.err != nil && d.connErr && ignoreConnErr {
		healthy = true
	}
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
}

func (d *Data) getStates(ignoreConnErr bool) ([]components.State, error) {
	state := components.State{
		Name:   docker_container_id.Name,
		Reason: d.Reason(),
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

// If docker daemon is not running, fails with:
// "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"
func listContainers(ctx context.Context) ([]DockerContainer, error) {
	cli, err := docker_client.NewClientWithOpts(docker_client.FromEnv)
	if err != nil {
		return nil, err
	}
	cs, err := cli.ContainerList(ctx, docker_container.ListOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}
	log.Logger.Debugw("listed containers", "containers", len(cs))

	containers := make([]DockerContainer, 0, len(cs))
	for _, c := range cs {
		containers = append(containers, convertToDockerContainer(c))
	}
	return containers, nil
}

const (
	podNameLabel      = "io.kubernetes.pod.name"
	podNamespaceLabel = "io.kubernetes.pod.namespace"
)

func convertToDockerContainer(resp docker_types.Container) DockerContainer {
	ret := DockerContainer{
		ID:           resp.ID,
		Name:         strings.Join(resp.Names, ","),
		Image:        resp.Image,
		CreatedAt:    resp.Created,
		State:        resp.State,
		PodName:      "",
		PodNamespace: "",
	}
	if podName, ok := resp.Labels[podNameLabel]; ok {
		ret.PodName = podName
	}
	if podNamespace, ok := resp.Labels[podNamespaceLabel]; ok {
		ret.PodNamespace = podNamespace
	}
	return ret
}

// If not run, fails with:
// "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"
func isDockerRunning() bool {
	cli, err := docker_client.NewClientWithOpts(docker_client.FromEnv)
	if err != nil {
		return false
	}
	_, err = cli.Ping(context.Background())
	return err == nil
}

// Returns true if docker component can be supported.
func CheckDockerRunning(ctx context.Context) bool {
	p, err := pkg_file.LocateExecutable("docker")
	if err == nil {
		log.Logger.Debugw("docker found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("docker not found in PATH -- fallback to docker run checks", "error", err)

	if isDockerRunning() {
		log.Logger.Debugw("auto-detected docker -- configuring docker container component")
		return true
	}
	return false
}

func (s DockerContainer) JSON() ([]byte, error) {
	return json.Marshal(s)
}

type DockerContainer struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	Image        string `json:"image,omitempty"`
	CreatedAt    int64  `json:"created_at,omitempty"`
	State        string `json:"state,omitempty"`
	PodName      string `json:"pod_name,omitempty"`
	PodNamespace string `json:"pod_namespace,omitempty"`
}

// isErrDockerClientVersionNewerThanDaemon returns true if the docker client version is newer than the daemon version.
// e.g.,
// "Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"
func isErrDockerClientVersionNewerThanDaemon(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "client version") && strings.Contains(err.Error(), "is too new")
}
