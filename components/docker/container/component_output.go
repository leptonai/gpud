package container

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/leptonai/gpud/components"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"

	docker_types "github.com/docker/docker/api/types"
	docker_container "github.com/docker/docker/api/types/container"
	docker_client "github.com/docker/docker/client"
)

type Output struct {
	Containers []DockerContainer `json:"containers,omitempty"`
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
	return fmt.Sprintf("total %d containers", len(o.Containers))
}

func (o *Output) States() ([]components.State, error) {
	b, _ := o.JSON()
	return []components.State{{
		Name:    StateNameDockerContainer,
		Healthy: true,
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

		dockerContainers, err := ListContainers(ctx)
		if err != nil {
			return nil, err
		}
		var containers []DockerContainer
		for _, c := range dockerContainers {
			containers = append(containers, ConvertToDockerContainer(c))
		}
		return &Output{Containers: containers}, nil
	}
}

// If not run, fails with:
// "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"
func ListContainers(ctx context.Context) ([]docker_types.Container, error) {
	cli, err := docker_client.NewClientWithOpts(docker_client.FromEnv)
	if err != nil {
		return nil, err
	}
	containers, err := cli.ContainerList(ctx, docker_container.ListOptions{})
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
