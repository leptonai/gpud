package docker

import (
	"context"
	"encoding/json"
	"strings"

	dockerapitypes "github.com/docker/docker/api/types"
	dockerapitypescontainer "github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"

	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

// ListContainers lists all containers from the docker daemon.
// If docker daemon is not running, fails with:
// "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"
func ListContainers(ctx context.Context) ([]DockerContainer, error) {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := cli.Close(); err != nil {
			log.Logger.Errorw("failed to close docker client", "error", err)
		}
	}()

	cs, err := cli.ContainerList(ctx, dockerapitypescontainer.ListOptions{
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

func convertToDockerContainer(resp dockerapitypes.Container) DockerContainer {
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

func CheckDockerInstalled() bool {
	p, err := pkgfile.LocateExecutable("docker")
	if err == nil {
		log.Logger.Debugw("docker found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("docker not found in PATH", "error", err)
	return false
}

// CheckDockerRunning checks if the docker daemon is running.
// If not run, fails with:
// "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"
func CheckDockerRunning(ctx context.Context) bool {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		return false
	}
	defer func() {
		if err := cli.Close(); err != nil {
			log.Logger.Errorw("failed to close docker client", "error", err)
		}
	}()

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

// IsErrDockerClientVersionNewerThanDaemon returns true if the docker client version is newer than the daemon version.
// e.g.,
// "Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"
func IsErrDockerClientVersionNewerThanDaemon(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "client version") && strings.Contains(err.Error(), "is too new")
}
