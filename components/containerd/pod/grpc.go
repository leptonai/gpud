package pod

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"

	pkg_file "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

// NOTE
// DO NOT USE https://github.com/kubernetes/kubernetes/blob/v1.32.0-alpha.0/staging/src/k8s.io/cri-client/pkg/remote_runtime.go yet
// it fails with
// "code = Unavailable desc = name resolver error: produced zero addresses"

const (
	// maxMsgSize use 16MB as the default message size limit.
	// grpc library default is 4MB
	maxMsgSize = 1024 * 1024 * 16

	// connection parameters
	maxBackoffDelay      = 3 * time.Second
	baseBackoffDelay     = 100 * time.Millisecond
	minConnectionTimeout = 10 * time.Second
)

// ref. https://github.com/kubernetes/kubernetes/blob/v1.29.2/pkg/kubelet/cri/remote/remote_runtime.go
func defaultDialOptions() []grpc.DialOption {
	cps := grpc.ConnectParams{Backoff: backoff.DefaultConfig}
	cps.MinConnectTimeout = minConnectionTimeout
	cps.Backoff.BaseDelay = baseBackoffDelay
	cps.Backoff.MaxDelay = maxBackoffDelay
	return []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)),
		grpc.WithConnectParams(cps),
		grpc.WithContextDialer(dialUnix),
		grpc.WithBlock(), //nolint:staticcheck
	}
}

func dialUnix(ctx context.Context, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", addr)
}

func parseUnixEndpoint(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Scheme != "unix" {
		return "", fmt.Errorf("invalid scheme: %s (only supports 'unix' protocol)", u.Scheme)
	}
	return u.Path, nil
}

// createConn creates a new CRI service client.
// Cannot use "k8s.io/kubernetes/pkg/kubelet/cri/remote.NewRemoteRuntimeService" directly
// as it causes a bunch of go module errors, importing the whole kubernetes repo.
// ref. https://github.com/kubernetes-sigs/cri-tools/blob/master/cmd/main.go
// ref. https://github.com/kubernetes/kubernetes/blob/v1.29.2/pkg/kubelet/cri/remote/remote_runtime.go
// ref. https://github.com/kubernetes/kubernetes/blob/v1.32.0-alpha.0/staging/src/k8s.io/cri-client/pkg/remote_runtime.go
func createConn(ctx context.Context, endpoint string) (runtimeapi.RuntimeServiceClient, runtimeapi.ImageServiceClient, *grpc.ClientConn, error) {
	// "k8s.io/cri-client/pkg/util.GetAddressAndDialer" doesn't work...
	// "code = Unavailable desc = name resolver error: produced zero addresses"
	addr, err := parseUnixEndpoint(endpoint)
	if err != nil {
		return nil, nil, nil, err
	}

	// "WithBlock" ctx cancel is no-op
	conn, err := grpc.DialContext(ctx, addr, defaultDialOptions()...) //nolint:staticcheck
	if err != nil {
		return nil, nil, nil, err
	}
	log.Logger.Debugw("successfully dialed -- checking version", "endpoint", endpoint)

	// ref. https://github.com/kubernetes/kubernetes/blob/v1.32.0-alpha.0/staging/src/k8s.io/cri-client/pkg/remote_runtime.go
	runtimeClient := runtimeapi.NewRuntimeServiceClient(conn)
	version, err := runtimeClient.Version(ctx, &runtimeapi.VersionRequest{})
	if err != nil {
		conn.Close()
		return nil, nil, nil, err
	}
	log.Logger.Debugw("successfully checked version", "endpoint", endpoint, "version", version.String())

	status, err := runtimeClient.Status(ctx, &runtimeapi.StatusRequest{})
	if err != nil {
		conn.Close()
		return nil, nil, nil, err
	}
	log.Logger.Debugw("successfully checked status", "endpoint", endpoint, "status", status.String())

	imageClient := runtimeapi.NewImageServiceClient(conn)
	return runtimeClient, imageClient, conn, nil
}

func CheckContainerdRunning(ctx context.Context) bool {
	if runtime.GOOS != "linux" {
		log.Logger.Debugw("ignoring default containerd pod checking since it's not linux", "os", runtime.GOOS)
		return false
	}

	p, err := pkg_file.LocateExecutable("containerd")
	if err == nil {
		log.Logger.Debugw("containerd found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("containerd not found in PATH -- fallback to containerd run checks", "error", err)

	containerdSocketExists := false
	containerdRunning := false

	if _, err := os.Stat(defaultSocketFile); err == nil {
		log.Logger.Debugw("containerd default socket file exists, containerd installed", "file", defaultSocketFile)
		containerdSocketExists = true
	} else {
		log.Logger.Debugw("containerd default socket file does not exist, skip containerd check", "file", defaultSocketFile, "error", err)
	}

	cctx, ccancel := context.WithTimeout(ctx, 5*time.Second)
	defer ccancel()

	if _, _, conn, err := createConn(cctx, defaultContainerRuntimeEndpoint); err == nil {
		log.Logger.Debugw("containerd default cri endpoint open, containerd running", "endpoint", defaultContainerRuntimeEndpoint)
		containerdRunning = true
		_ = conn.Close()
	} else {
		log.Logger.Debugw("containerd default cri endpoint not open, skip containerd checking", "endpoint", defaultContainerRuntimeEndpoint, "error", err)
	}

	if containerdSocketExists && containerdRunning {
		log.Logger.Debugw("auto-detected containerd -- configuring containerd pod component")
		return true
	}
	return false
}
