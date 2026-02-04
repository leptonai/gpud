package docker

import (
	"context"
	"errors"
	"testing"

	dockerapitypes "github.com/docker/docker/api/types"
	dockerapitypescontainer "github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgfile "github.com/leptonai/gpud/pkg/file"
)

// --- CheckDockerInstalled mockey tests ---

func TestCheckDockerInstalled_Found(t *testing.T) {
	mockey.PatchConvey("docker found in PATH", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/docker", nil
		}).Build()

		result := CheckDockerInstalled()
		assert.True(t, result)
	})
}

func TestCheckDockerInstalled_NotFound(t *testing.T) {
	mockey.PatchConvey("docker not found in PATH", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(bin string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		result := CheckDockerInstalled()
		assert.False(t, result)
	})
}

// --- CheckDockerRunning mockey tests ---

func TestCheckDockerRunning_ClientCreationError(t *testing.T) {
	mockey.PatchConvey("docker client creation fails", t, func() {
		mockey.Mock(dockerclient.NewClientWithOpts).To(func(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
			return nil, errors.New("cannot create client")
		}).Build()

		result := CheckDockerRunning(context.Background())
		assert.False(t, result)
	})
}

// --- ListContainers mockey tests ---

func TestListContainers_ClientCreationError(t *testing.T) {
	mockey.PatchConvey("ListContainers client creation fails", t, func() {
		mockey.Mock(dockerclient.NewClientWithOpts).To(func(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
			return nil, errors.New("cannot create client")
		}).Build()

		result, err := ListContainers(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot create client")
		assert.Nil(t, result)
	})
}

func TestListContainers_ContainerListError(t *testing.T) {
	mockey.PatchConvey("ListContainers list fails", t, func() {
		mockClient := &dockerclient.Client{}

		mockey.Mock(dockerclient.NewClientWithOpts).To(func(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).ContainerList).To(func(ctx context.Context, options dockerapitypescontainer.ListOptions) ([]dockerapitypes.Container, error) {
			return nil, errors.New("cannot connect to docker daemon")
		}).Build()

		mockey.Mock((*dockerclient.Client).Close).Return(nil).Build()

		result, err := ListContainers(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot connect to docker daemon")
		assert.Nil(t, result)
	})
}

func TestListContainers_Success(t *testing.T) {
	mockey.PatchConvey("ListContainers success", t, func() {
		mockClient := &dockerclient.Client{}

		mockey.Mock(dockerclient.NewClientWithOpts).To(func(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).ContainerList).To(func(ctx context.Context, options dockerapitypescontainer.ListOptions) ([]dockerapitypes.Container, error) {
			return []dockerapitypes.Container{
				{
					ID:      "abc123",
					Names:   []string{"/test-container"},
					Image:   "nginx:latest",
					Created: 1234567890,
					State:   "running",
					Labels: map[string]string{
						"io.kubernetes.pod.name":      "my-pod",
						"io.kubernetes.pod.namespace": "default",
					},
				},
				{
					ID:      "def456",
					Names:   []string{"/another-container"},
					Image:   "redis:latest",
					Created: 1234567891,
					State:   "exited",
					Labels:  map[string]string{},
				},
			}, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).Close).Return(nil).Build()

		result, err := ListContainers(context.Background())
		require.NoError(t, err)
		require.Len(t, result, 2)

		assert.Equal(t, "abc123", result[0].ID)
		assert.Equal(t, "/test-container", result[0].Name)
		assert.Equal(t, "nginx:latest", result[0].Image)
		assert.Equal(t, "running", result[0].State)
		assert.Equal(t, "my-pod", result[0].PodName)
		assert.Equal(t, "default", result[0].PodNamespace)

		assert.Equal(t, "def456", result[1].ID)
		assert.Equal(t, "exited", result[1].State)
		assert.Equal(t, "", result[1].PodName)
	})
}

func TestListContainers_EmptyList(t *testing.T) {
	mockey.PatchConvey("ListContainers returns empty list", t, func() {
		mockClient := &dockerclient.Client{}

		mockey.Mock(dockerclient.NewClientWithOpts).To(func(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).ContainerList).To(func(ctx context.Context, options dockerapitypescontainer.ListOptions) ([]dockerapitypes.Container, error) {
			return []dockerapitypes.Container{}, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).Close).Return(nil).Build()

		result, err := ListContainers(context.Background())
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestListContainers_CloseError(t *testing.T) {
	mockey.PatchConvey("ListContainers with client close error", t, func() {
		mockClient := &dockerclient.Client{}

		mockey.Mock(dockerclient.NewClientWithOpts).To(func(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).ContainerList).To(func(ctx context.Context, options dockerapitypescontainer.ListOptions) ([]dockerapitypes.Container, error) {
			return []dockerapitypes.Container{}, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).Close).To(func() error {
			return errors.New("close failed")
		}).Build()

		result, err := ListContainers(context.Background())
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

// --- CheckDockerRunning mockey tests (additional) ---

func TestCheckDockerRunning_Success(t *testing.T) {
	mockey.PatchConvey("docker running success", t, func() {
		mockClient := &dockerclient.Client{}

		mockey.Mock(dockerclient.NewClientWithOpts).To(func(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).Ping).To(func(ctx context.Context) (dockerapitypes.Ping, error) {
			return dockerapitypes.Ping{}, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).Close).Return(nil).Build()

		result := CheckDockerRunning(context.Background())
		assert.True(t, result)
	})
}

func TestCheckDockerRunning_PingError(t *testing.T) {
	mockey.PatchConvey("docker ping fails", t, func() {
		mockClient := &dockerclient.Client{}

		mockey.Mock(dockerclient.NewClientWithOpts).To(func(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).Ping).To(func(ctx context.Context) (dockerapitypes.Ping, error) {
			return dockerapitypes.Ping{}, errors.New("connection refused")
		}).Build()

		mockey.Mock((*dockerclient.Client).Close).Return(nil).Build()

		result := CheckDockerRunning(context.Background())
		assert.False(t, result)
	})
}

func TestCheckDockerRunning_CloseError(t *testing.T) {
	mockey.PatchConvey("docker running close error", t, func() {
		mockClient := &dockerclient.Client{}

		mockey.Mock(dockerclient.NewClientWithOpts).To(func(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).Ping).To(func(ctx context.Context) (dockerapitypes.Ping, error) {
			return dockerapitypes.Ping{}, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).Close).To(func() error {
			return errors.New("close failed")
		}).Build()

		result := CheckDockerRunning(context.Background())
		assert.True(t, result)
	})
}

// --- convertToDockerContainer edge cases ---

func TestConvertToDockerContainer_OnlyPodName(t *testing.T) {
	mockey.PatchConvey("convertToDockerContainer with only pod name label", t, func() {
		resp := dockerapitypes.Container{
			ID:    "partial-labels",
			Names: []string{"/partial"},
			Image: "test:latest",
			State: "running",
			Labels: map[string]string{
				"io.kubernetes.pod.name": "my-pod",
			},
		}

		result := convertToDockerContainer(resp)
		assert.Equal(t, "my-pod", result.PodName)
		assert.Empty(t, result.PodNamespace)
	})
}

func TestConvertToDockerContainer_OnlyPodNamespace(t *testing.T) {
	mockey.PatchConvey("convertToDockerContainer with only pod namespace label", t, func() {
		resp := dockerapitypes.Container{
			ID:    "ns-only",
			Names: []string{"/ns-container"},
			Image: "test:latest",
			State: "running",
			Labels: map[string]string{
				"io.kubernetes.pod.namespace": "kube-system",
			},
		}

		result := convertToDockerContainer(resp)
		assert.Empty(t, result.PodName)
		assert.Equal(t, "kube-system", result.PodNamespace)
	})
}

func TestConvertToDockerContainer_NoLabels(t *testing.T) {
	mockey.PatchConvey("convertToDockerContainer with nil labels", t, func() {
		resp := dockerapitypes.Container{
			ID:     "no-labels",
			Names:  []string{"/simple"},
			Image:  "nginx",
			State:  "running",
			Labels: nil,
		}

		result := convertToDockerContainer(resp)
		assert.Equal(t, "no-labels", result.ID)
		assert.Equal(t, "/simple", result.Name)
		assert.Empty(t, result.PodName)
		assert.Empty(t, result.PodNamespace)
	})
}

func TestConvertToDockerContainer_MultipleNames(t *testing.T) {
	mockey.PatchConvey("convertToDockerContainer joins multiple names", t, func() {
		resp := dockerapitypes.Container{
			ID:    "multi-name",
			Names: []string{"/name1", "/name2", "/name3"},
			Image: "test:latest",
			State: "running",
		}

		result := convertToDockerContainer(resp)
		assert.Equal(t, "/name1,/name2,/name3", result.Name)
	})
}

func TestConvertToDockerContainer_EmptyNames(t *testing.T) {
	mockey.PatchConvey("convertToDockerContainer with no names", t, func() {
		resp := dockerapitypes.Container{
			ID:    "no-name",
			Names: []string{},
			Image: "test:latest",
			State: "exited",
		}

		result := convertToDockerContainer(resp)
		assert.Empty(t, result.Name)
	})
}

// --- DockerContainer struct tests ---

func TestDockerContainer_ZeroValue(t *testing.T) {
	mockey.PatchConvey("DockerContainer zero value", t, func() {
		c := DockerContainer{}
		assert.Empty(t, c.ID)
		assert.Empty(t, c.Name)
		assert.Empty(t, c.Image)
		assert.Equal(t, int64(0), c.CreatedAt)
		assert.Empty(t, c.State)
		assert.Empty(t, c.PodName)
		assert.Empty(t, c.PodNamespace)
	})
}

// --- IsErrDockerClientVersionNewerThanDaemon additional tests ---

func TestIsErrDockerClientVersionNewerThanDaemon_OnlyClientVersion(t *testing.T) {
	mockey.PatchConvey("error with only 'client version' returns false", t, func() {
		err := errors.New("client version 1.44")
		assert.False(t, IsErrDockerClientVersionNewerThanDaemon(err))
	})
}

func TestIsErrDockerClientVersionNewerThanDaemon_OnlyTooNew(t *testing.T) {
	mockey.PatchConvey("error with only 'is too new' returns false", t, func() {
		err := errors.New("something is too new")
		assert.False(t, IsErrDockerClientVersionNewerThanDaemon(err))
	})
}

func TestIsErrDockerClientVersionNewerThanDaemon_FullMatch(t *testing.T) {
	mockey.PatchConvey("error with both parts returns true", t, func() {
		err := errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43")
		assert.True(t, IsErrDockerClientVersionNewerThanDaemon(err))
	})
}

// --- ListContainers with context ---

func TestListContainers_ContextCanceled(t *testing.T) {
	mockey.PatchConvey("ListContainers with canceled context", t, func() {
		mockClient := &dockerclient.Client{}

		mockey.Mock(dockerclient.NewClientWithOpts).To(func(ops ...dockerclient.Opt) (*dockerclient.Client, error) {
			return mockClient, nil
		}).Build()

		mockey.Mock((*dockerclient.Client).ContainerList).To(func(ctx context.Context, options dockerapitypescontainer.ListOptions) ([]dockerapitypes.Container, error) {
			return nil, ctx.Err()
		}).Build()

		mockey.Mock((*dockerclient.Client).Close).Return(nil).Build()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		_, err := ListContainers(ctx)
		require.Error(t, err)
	})
}
