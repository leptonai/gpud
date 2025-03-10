package container

import (
	"context"
	"encoding/json"
	"strings"

	docker_types "github.com/docker/docker/api/types"
	docker_container "github.com/docker/docker/api/types/container"
	docker_client "github.com/docker/docker/client"

	pkg_file "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

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

func checkDockerInstalled() bool {
	p, err := pkg_file.LocateExecutable("docker")
	if err == nil {
		log.Logger.Debugw("docker found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("docker not found in PATH", "error", err)
	return false
}

// If not run, fails with:
// "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"
func checkDockerRunning(ctx context.Context) bool {
	cli, err := docker_client.NewClientWithOpts(docker_client.FromEnv)
	if err != nil {
		return false
	}
	_, err = cli.Ping(ctx)
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

// isErrDockerClientVersionNewerThanDaemon returns true if the docker client version is newer than the daemon version.
// e.g.,
// "Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"
func isErrDockerClientVersionNewerThanDaemon(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "client version") && strings.Contains(err.Error(), "is too new")
}
