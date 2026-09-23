package containerd

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	configcommon "github.com/leptonai/gpud/pkg/config/common"
)

type healthyRuntime struct {
	runtimeapi.UnimplementedRuntimeServiceServer
}

func (healthyRuntime) Version(context.Context, *runtimeapi.VersionRequest) (*runtimeapi.VersionResponse, error) {
	return &runtimeapi.VersionResponse{Version: "0.1.0", RuntimeName: "containerd", RuntimeVersion: "2.0.0", RuntimeApiVersion: "v1"}, nil
}
func (healthyRuntime) Status(context.Context, *runtimeapi.StatusRequest) (*runtimeapi.StatusResponse, error) {
	return &runtimeapi.StatusResponse{}, nil
}
func (healthyRuntime) ListPodSandbox(context.Context, *runtimeapi.ListPodSandboxRequest) (*runtimeapi.ListPodSandboxResponse, error) {
	return &runtimeapi.ListPodSandboxResponse{Items: []*runtimeapi.PodSandbox{{Id: "test-sandbox", Metadata: &runtimeapi.PodSandboxMetadata{Name: "test", Namespace: "default"}}}}, nil
}
func (healthyRuntime) ListContainers(context.Context, *runtimeapi.ListContainersRequest) (*runtimeapi.ListContainersResponse, error) {
	return &runtimeapi.ListContainersResponse{}, nil
}

func TestConfiguredRuntimeSocketRecovery(t *testing.T) {
	// Keep below the Unix socket path limit, including on macOS.
	dir, err := os.MkdirTemp("", "cri-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	socket := filepath.Join(dir, "cri.sock")
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("configured runtime"), 0600))
	comp, err := New(&components.GPUdInstance{RootCtx: context.Background(), Containerd: configcommon.ContainerdConfig{
		Endpoint: "unix://" + socket, ConfigPath: configPath, ServiceName: "rke2-agent",
	}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, comp.Close()) })
	c := comp.(*component)
	c.checkDependencyInstalledFunc = func() bool { t.Fatal("explicit runtime must not inspect the image PATH"); return false }
	c.checkCRIORunningFunc = func(context.Context) bool { t.Fatal("explicit runtime must not fall back to CRI-O"); return true }
	c.checkServiceActiveFunc = func(context.Context) (bool, error) { return true, nil }
	c.getContainerdUptimeFunc = func() (*time.Duration, error) { return nil, nil }
	c.listKubeletPodsFunc = nil
	b, err := c.getContainerdConfigFunc()
	require.NoError(t, err)
	require.Equal(t, "configured runtime", string(b))

	for i := 1; i <= socketMissingConsecutiveThreshold; i++ {
		cr := c.Check().(*checkResult)
		if i < socketMissingConsecutiveThreshold {
			require.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		} else {
			require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		}
	}
	start := func() *grpc.Server {
		listener, err := net.Listen("unix", socket)
		require.NoError(t, err)
		server := grpc.NewServer()
		runtimeapi.RegisterRuntimeServiceServer(server, &healthyRuntime{})
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(server.Stop)
		return server
	}
	server := start()
	cr := c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	require.NoError(t, cr.err)
	require.Len(t, cr.Pods, 1) // Real CRI enumeration used the configured endpoint.
	require.Zero(t, c.socketMissingCount)
	server.Stop()
	require.False(t, c.checkSocketExistsFunc())
	// A stale socket is not a live runtime. Keep its inode after closing the listener.
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	require.NoError(t, err)
	stale.SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())
	cr = c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.Equal(t, "containerd installed but not running", cr.reason)
	require.NoError(t, os.Remove(socket))
	for range socketMissingConsecutiveThreshold {
		cr = c.Check().(*checkResult)
	}
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	start()
	cr = c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	require.Zero(t, c.socketMissingCount)
}

func TestConfiguredRuntimeServiceOverride(t *testing.T) {
	comp, err := New(&components.GPUdInstance{
		RootCtx:                         context.Background(),
		Containerd:                      configcommon.ContainerdConfig{ServiceName: "rke2-agent", SystemctlCommands: "exit 1;"},
		ContainerdServiceActiveCommands: "exit 0",
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, comp.Close()) }()
	c := comp.(*component)
	active, err := c.checkServiceActiveFunc(context.Background())
	require.NoError(t, err)
	require.True(t, active)
	uptime, err := c.getContainerdUptimeFunc()
	require.NoError(t, err)
	require.Nil(t, uptime, "arbitrary active override must not grant grace from an unrelated unit")
}
