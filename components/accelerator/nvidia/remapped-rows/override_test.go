package remappedrows

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestProductOverrideDoesNotUsePhysicalRubinPolicy(t *testing.T) {
	const overriddenName = "NVIDIA H100 PCIe"
	dev := testutil.NewMockDevice(&mock.Device{}, "rubin", "NVIDIA", "", "0000:01:00.0")
	inst := &overriddenMemoryInstance{mockNVMLInstance: &mockNVMLInstance{
		architecture:       "rubin",
		getDevicesFunc:     func() map[string]device.Device { return map[string]device.Device{"GPU-0": dev} },
		getProductNameFunc: func() string { return overriddenName },
		getMemoryErrorManagementCapabilitiesFunc: func() product.MemoryErrorManagementCapabilities {
			return product.CapabilitiesForGPU(overriddenName, "", product.HostLinkUnknown).MemoryManagement
		},
	}}
	comp, err := New(&components.GPUdInstance{RootCtx: t.Context(), NVMLInstance: inst})
	require.NoError(t, err)
	defer func() { require.NoError(t, comp.Close()) }()
	c := mustComponent(t, comp)
	queried := false
	c.getMemoryRepairStatusFunc = func(uuid string, _ device.Device) (MemoryRepairStatus, error) {
		queried = true
		return MemoryRepairStatus{UUID: uuid, UnrepairableMemorySupported: true, UnrepairableMemory: true}, nil
	}
	c.getRemappedRowsFunc = func(uuid string, _ device.Device) (RemappedRows, error) {
		return RemappedRows{UUID: uuid, Supported: true}, nil
	}
	cr := c.Check().(*checkResult)
	t.Logf("health=%s, reason=%s, repair queried=%t", cr.health, cr.reason, queried)
	require.False(t, queried, "the explicit H100 override must keep its row-only memory policy")
	require.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	require.Nil(t, cr.suggestedActions)
	require.Empty(t, cr.MemoryRepairStatuses)
}

// A product override preserves physical architecture metadata while resolving
// capabilities from the simulated product, as newInstance does.
type overriddenMemoryInstance struct{ *mockNVMLInstance }

func (m *overriddenMemoryInstance) Capabilities() product.Capabilities {
	return product.CapabilitiesForGPU(m.ProductName(), "", product.HostLinkUnknown)
}
