package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	dockerapitypescontainer "github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

// ListContainers lists all containers from the docker daemon.
// If docker daemon is not running, fails with:
// "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"
func ListContainers(ctx context.Context) ([]DockerContainer, error) {
	cli, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := cli.Close(); err != nil {
			log.Logger.Errorw("failed to close docker client", "error", err)
		}
	}()

	result, err := cli.ContainerList(ctx, dockerclient.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}
	log.Logger.Debugw("listed containers", "containers", len(result.Items))

	containers := make([]DockerContainer, 0, len(result.Items))
	for _, c := range result.Items {
		containers = append(containers, convertToDockerContainer(c))
	}
	return containers, nil
}

const (
	podNameLabel      = "io.kubernetes.pod.name"
	podNamespaceLabel = "io.kubernetes.pod.namespace"
)

func convertToDockerContainer(resp dockerapitypescontainer.Summary) DockerContainer {
	ret := DockerContainer{
		ID:           resp.ID,
		Name:         strings.Join(resp.Names, ","),
		Image:        resp.Image,
		CreatedAt:    resp.Created,
		State:        string(resp.State),
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

// CheckDockerInstalledHostAware reports whether docker is installed on the
// node. When gpud runs inside a container with the host root filesystem
// mounted (the gpud DaemonSet's gpud.mountHostRoot, default on), a docker CLI
// bundled in the container image itself must not count: the node only "has
// docker" if the HOST has the binary. (LEP-6440: the gpud image ships
// /usr/bin/docker, so the PATH-only check reported "docker installed but
// docker is not running" on containerd-only nodes such as aws-iad-nkxdev-1.)
// Without the host-root mount (bare-metal/systemd installs) it falls back to
// the PATH lookup, preserving the previous behavior.
func CheckDockerInstalledHostAware() bool {
	return checkDockerInstalledOnHost("/host")
}

// checkDockerInstalledOnHost probes for the docker binary under the given
// host root. It takes the root as a parameter so tests can point the probe at
// a temporary directory. When the host root does not exist (not containerized
// or mountHostRoot disabled), it defers to the PATH lookup.
func checkDockerInstalledOnHost(hostRoot string) bool {
	st, err := os.Stat(hostRoot)
	if err != nil || !st.IsDir() {
		return CheckDockerInstalled()
	}
	for _, dir := range []string{"usr/bin", "usr/local/bin", "usr/sbin", "bin", "sbin"} {
		if st, err := os.Stat(filepath.Join(hostRoot, dir, "docker")); err == nil && !st.IsDir() {
			log.Logger.Debugw("docker found on host", "path", filepath.Join(hostRoot, dir, "docker"))
			return true
		}
	}
	log.Logger.Debugw("docker not found on host; skipping container PATH result", "hostRoot", hostRoot)
	return false
}

// CheckDockerRunning checks if the docker daemon is running.
// If not run, fails with:
// "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"
func CheckDockerRunning(ctx context.Context) bool {
	cli, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		return false
	}
	defer func() {
		if err := cli.Close(); err != nil {
			log.Logger.Errorw("failed to close docker client", "error", err)
		}
	}()

	_, err = cli.Ping(ctx, dockerclient.PingOptions{})
	return err == nil
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
