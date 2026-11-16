package fabricmanager

import (
	"errors"
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestRubinQueryFailureDoesNotEstablishLocalFabric(t *testing.T) {
	queryErr := errors.New("fabric query failed")
	for _, mixedUnsupported := range []bool{false, true} {
		t.Run(fmt.Sprint(mixedUnsupported), func(t *testing.T) {
			inst := rubinInstance("NVIDIA Graphics Device", product.HostLinkUnknown, 1, device.FabricState{})
			inst.devs["GPU-0"] = &mockFabricStateDevice{err: queryErr}
			if mixedUnsupported {
				inst.devs["GPU-1"] = &mockFabricStateDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_SUPPORTED}}
			}
			fmChecked := false
			c := &component{
				ctx: t.Context(), nvmlInstance: inst,
				checkNVSwitchExistsFunc: func() bool { return false },
				collectFabricStateFunc:  func() fabricStateReport { return collectFabricState(inst) },
				checkFMExistsFunc:       func() bool { fmChecked = true; return false },
			}
			cr := c.Check().(*checkResult)
			require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health, cr.reason)
			require.ErrorIs(t, cr.err, queryErr)
			require.False(t, fmChecked, "a failed query does not establish a local fabric")
			require.NotContains(t, cr.reason, "executable not found")
		})
	}
}
