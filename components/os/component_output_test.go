package os

import (
	"context"
	"testing"

	pkg_host "github.com/leptonai/gpud/pkg/host"
)

func TestGet(t *testing.T) {
	defer func() {
		getSystemdDetectVirtFunc = pkg_host.SystemdDetectVirt
	}()
	getSystemdDetectVirtFunc = func(ctx context.Context) (pkg_host.VirtualizationEnvironment, error) {
		return pkg_host.VirtualizationEnvironment{}, context.DeadlineExceeded
	}

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	getFunc := createGet(Config{}, nil)
	_, err := getFunc(ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	expectedError := "failed to get virtualization environment using 'systemd-detect-virt': context deadline exceeded"
	if err.Error() != expectedError {
		t.Fatalf("expected error: %s, got: %s", expectedError, err.Error())
	}
}
