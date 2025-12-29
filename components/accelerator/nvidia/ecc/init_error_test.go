package ecc

import (
	"context"
	"errors"
	"testing"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func TestInitErrorUnhealthy(t *testing.T) {
	initErr := errors.New("nvml init failed")
	comp, err := New(&components.GPUdInstance{
		RootCtx:      context.Background(),
		NVMLInstance: nvidianvml.NewErrored(initErr),
	})
	if err != nil {
		t.Fatalf("failed to create component: %v", err)
	}

	if !comp.IsSupported() {
		t.Fatalf("expected IsSupported true when NVML init error is present")
	}

	if got := comp.Check().HealthStateType(); got != apiv1.HealthStateTypeUnhealthy {
		t.Fatalf("expected unhealthy, got %v", got)
	}
}
