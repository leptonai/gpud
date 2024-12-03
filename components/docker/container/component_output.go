package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"

	docker_types "github.com/docker/docker/api/types"
	docker_container "github.com/docker/docker/api/types/container"
	docker_client "github.com/docker/docker/client"
)

type Output struct {
	Containers []DockerContainer `json:"containers,omitempty"`

	// Sets true if the docker client calls due to the version incompatibility issue.
	// e.g.,
	// "Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"
	IsErrDockerClientVersionNewerThanDaemon bool   `json:"is_err_docker_client_version_newer_than_daemon,omitempty"`
	DockerClientError                       string `json:"docker_client_error,omitempty"`

	DockerPidFound bool `json:"docker_pid_found,omitempty"`

	// In case the docker daemon is not running
	// 'Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?'.
	ConnectionError string `json:"connection_error,omitempty"`
	Message         string `json:"message,omitempty"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func ParseOutputJSON(data []byte) (*Output, error) {
	o := new(Output)
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

const (
	StateNameDockerContainer = "docker_container"

	StateKeyDockerContainerData           = "data"
	StateKeyDockerContainerEncoding       = "encoding"
	StateValueDockerContainerEncodingJSON = "json"

	PodNameLabel      = "io.kubernetes.pod.name"
	PodNamespaceLabel = "io.kubernetes.pod.namespace"
)

func ParseStateDockerContainer(m map[string]string) ([]DockerContainer, error) {
	var containers []DockerContainer
	data := m[StateKeyDockerContainerData]
	if err := json.Unmarshal([]byte(data), &containers); err != nil {
		return nil, err
	}
	return containers, nil
}

func (o *Output) describeReason() string {
	if o.IsErrDockerClientVersionNewerThanDaemon {
		if o.DockerClientError == "" {
			return "not supported; docker client version is newer than the daemon version (needs upgrading docker daemon in the host)"
		}
		return fmt.Sprintf("not supported; %s (needs upgrading docker daemon in the host)", o.DockerClientError)
	}
	if o.ConnectionError != "" {
		return fmt.Sprintf("connection error to docker daemon -- %s", o.ConnectionError)
	}
	return fmt.Sprintf("total %d containers", len(o.Containers))
}

func (o *Output) States(cfg Config) ([]components.State, error) {
	healthy := o.ConnectionError == ""
	if cfg.IgnoreConnectionErrors {
		healthy = true
	}

	b, _ := o.JSON()
	return []components.State{{
		Name:    StateNameDockerContainer,
		Healthy: healthy,
		Reason:  o.describeReason(),
		ExtraInfo: map[string]string{
			StateKeyDockerContainerData:     string(b),
			StateKeyDockerContainerEncoding: StateValueDockerContainerEncodingJSON,
		},
	}}, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	o := &Output{}
	for _, state := range states {
		switch state.Name {
		case StateNameDockerContainer:
			containers, err := ParseStateDockerContainer(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Containers = containers

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return o, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(Name, cfg.Query, CreateGet(cfg))
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(Name)
			} else {
				components_metrics.SetGetSuccess(Name)
			}
		}()

		// check if a process named "docker" is running
		dockerRunning := false
		if err := exec.Command("pidof", "docker").Run(); err == nil {
			dockerRunning = true
		} else {
			log.Logger.Warnw("docker process not found, assuming docker is not running", "error", err)
		}

		// "ctx" here is the root level, create one with shorter timeouts
		// to not block on this checks
		cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
		dockerContainers, err := ListContainers(cctx)
		ccancel()
		if err != nil {
			if IsErrDockerClientVersionNewerThanDaemon(err) {
				return &Output{
					IsErrDockerClientVersionNewerThanDaemon: true,
					DockerClientError:                       err.Error(),
				}, nil
			}

			o := &Output{
				DockerPidFound: dockerRunning,
				Message:        "failed to list containers from docker -- " + err.Error(),
			}

			// e.g.,
			// Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
			if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") || strings.Contains(err.Error(), "the docker daemon running") {
				o.ConnectionError = err.Error()
			}

			return o, nil
		}

		var containers []DockerContainer
		for _, c := range dockerContainers {
			containers = append(containers, ConvertToDockerContainer(c))
		}
		return &Output{Containers: containers, DockerPidFound: dockerRunning}, nil
	}
}

// If not run, fails with:
// "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"
func ListContainers(ctx context.Context) ([]docker_types.Container, error) {
	cli, err := docker_client.NewClientWithOpts(docker_client.FromEnv)
	if err != nil {
		return nil, err
	}
	containers, err := cli.ContainerList(ctx, docker_container.ListOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func ConvertToDockerContainer(resp docker_types.Container) DockerContainer {
	ret := DockerContainer{
		ID:           resp.ID,
		Name:         strings.Join(resp.Names, ","),
		Image:        resp.Image,
		CreatedAt:    resp.Created,
		State:        resp.State,
		PodName:      "",
		PodNamespace: "",
	}
	if podName, ok := resp.Labels[PodNameLabel]; ok {
		ret.PodName = podName
	}
	if podNamespace, ok := resp.Labels[PodNamespaceLabel]; ok {
		ret.PodNamespace = podNamespace
	}
	return ret
}

// If not run, fails with:
// "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"
func IsDockerRunning() bool {
	cli, err := docker_client.NewClientWithOpts(docker_client.FromEnv)
	if err != nil {
		return false
	}
	_, err = cli.Ping(context.Background())
	return err == nil
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

// IsErrDockerClientVersionNewerThanDaemon returns true if the docker client version is newer than the daemon version.
// e.g.,
// "Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"
func IsErrDockerClientVersionNewerThanDaemon(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "client version") && strings.Contains(err.Error(), "is too new")
}
