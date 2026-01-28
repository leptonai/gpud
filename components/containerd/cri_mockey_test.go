package containerd

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"

	componentkubelet "github.com/leptonai/gpud/components/kubelet"
	pkgfile "github.com/leptonai/gpud/pkg/file"
)

type fakeRuntimeServer struct {
	runtimeapi.UnimplementedRuntimeServiceServer
	version            string
	statusResp         *runtimeapi.StatusResponse
	listPodSandboxResp *runtimeapi.ListPodSandboxResponse
	listContainersResp *runtimeapi.ListContainersResponse
}

func (f *fakeRuntimeServer) Version(ctx context.Context, _ *runtimeapi.VersionRequest) (*runtimeapi.VersionResponse, error) {
	return &runtimeapi.VersionResponse{RuntimeVersion: f.version}, nil
}

func (f *fakeRuntimeServer) Status(ctx context.Context, _ *runtimeapi.StatusRequest) (*runtimeapi.StatusResponse, error) {
	if f.statusResp != nil {
		return f.statusResp, nil
	}
	return &runtimeapi.StatusResponse{}, nil
}

func (f *fakeRuntimeServer) ListPodSandbox(ctx context.Context, _ *runtimeapi.ListPodSandboxRequest) (*runtimeapi.ListPodSandboxResponse, error) {
	return f.listPodSandboxResp, nil
}

func (f *fakeRuntimeServer) ListContainers(ctx context.Context, _ *runtimeapi.ListContainersRequest) (*runtimeapi.ListContainersResponse, error) {
	return f.listContainersResp, nil
}

func startFakeRuntimeServer(t *testing.T, srv runtimeapi.RuntimeServiceServer) (string, func()) {
	t.Helper()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "containerd.sock")
	lis, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	runtimeapi.RegisterRuntimeServiceServer(grpcServer, srv)
	go func() {
		_ = grpcServer.Serve(lis)
	}()

	cleanup := func() {
		grpcServer.Stop()
		_ = lis.Close()
	}
	return "unix://" + socketPath, cleanup
}

func TestGetVersionAndListAllSandboxes(t *testing.T) {
	srv := &fakeRuntimeServer{
		version: "1.7.25",
		listPodSandboxResp: &runtimeapi.ListPodSandboxResponse{
			Items: []*runtimeapi.PodSandbox{
				{
					Id:        "pod-1",
					CreatedAt: time.Now().UnixNano(),
					State:     runtimeapi.PodSandboxState_SANDBOX_READY,
					Metadata: &runtimeapi.PodSandboxMetadata{
						Name:      "pod",
						Namespace: "default",
					},
				},
			},
		},
		listContainersResp: &runtimeapi.ListContainersResponse{
			Containers: []*runtimeapi.Container{
				{
					Id:           "container-1",
					PodSandboxId: "pod-1",
					CreatedAt:    time.Now().UnixNano(),
					State:        runtimeapi.ContainerState_CONTAINER_RUNNING,
					Metadata: &runtimeapi.ContainerMetadata{
						Name: "container",
					},
					Image: &runtimeapi.ImageSpec{Image: "busybox"},
				},
			},
		},
	}

	endpoint, cleanup := startFakeRuntimeServer(t, srv)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	version, err := GetVersion(ctx, endpoint)
	require.NoError(t, err)
	assert.Equal(t, "1.7.25", version)

	pods, err := ListAllSandboxes(ctx, endpoint)
	require.NoError(t, err)
	require.Len(t, pods, 1)
	assert.Equal(t, "pod", pods[0].Name)
	require.Len(t, pods[0].Containers, 1)
	assert.Equal(t, "container", pods[0].Containers[0].Name)
}

func TestCheckContainerdRunning_WithMockey(t *testing.T) {
	srv := &fakeRuntimeServer{
		version: "1.7.25",
	}
	endpoint, cleanup := startFakeRuntimeServer(t, srv)
	t.Cleanup(cleanup)

	addr, err := parseUnixEndpoint(endpoint)
	require.NoError(t, err)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(dialUnix))
	require.NoError(t, err)

	mockey.PatchConvey("CheckContainerdRunning uses mocked connect", t, func() {
		mockey.Mock(connect).To(func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
			return conn, nil
		}).Build()

		assert.True(t, CheckContainerdRunning(context.Background()))
	})
}

func TestCheckSocketExists_WithMockey(t *testing.T) {
	mockey.PatchConvey("CheckSocketExists respects Stat results", t, func() {
		tempFile, err := os.CreateTemp("", "containerd-sock")
		require.NoError(t, err)
		info, err := tempFile.Stat()
		require.NoError(t, err)
		require.NoError(t, tempFile.Close())

		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			return info, nil
		}).Build()

		assert.True(t, CheckSocketExists())
	})
}

func TestGetVersionFromCli_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetVersionFromCli parses version", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/containerd", nil
		}).Build()
		mockey.Mock((*exec.Cmd).Output).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("containerd containerd.io 1.7.25 bcc810d6b9066471b0b6fa75f557a15a1cbf31bb"), nil
		}).Build()

		version, err := GetVersionFromCli(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "1.7.25", version)
	})
}

func TestComponentMethodsAndDanglingPods(t *testing.T) {
	comp := &component{}
	assert.True(t, comp.IsSupported())

	cr := &checkResult{}
	assert.Equal(t, Name, cr.ComponentName())

	kubeletPods := []componentkubelet.PodStatus{
		{Name: "pod", Namespace: "default"},
	}
	containerdPods := []PodSandbox{
		{Name: "pod", Namespace: "default", State: "SANDBOX_READY"},
		{Name: "dangling", Namespace: "default", State: "SANDBOX_READY"},
	}
	assert.Equal(t, 1, danglingPodCount(containerdPods, kubeletPods))
}
