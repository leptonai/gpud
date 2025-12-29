package infiniband

import (
	"context"
	"errors"
	"testing"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	ibtypes "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
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

	ibComp, ok := comp.(*component)
	if !ok {
		t.Fatalf("expected infiniband component type, got %T", comp)
	}
	ibComp.getThresholdsFunc = func() ibtypes.ExpectedPortStates {
		return ibtypes.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 1}
	}

	if got := ibComp.Check().HealthStateType(); got != apiv1.HealthStateTypeUnhealthy {
		t.Fatalf("expected unhealthy, got %v", got)
	}
}
