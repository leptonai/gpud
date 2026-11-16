package nvlink

import (
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestNVL8NameDoesNotInventFabricTopology(t *testing.T) {
	devs := map[string]device.Device{}
	for i := range 8 {
		devs[fmt.Sprint("GPU-", i)] = &fabricTestDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_SUPPORTED}}
	}
	c := MockNVLinkComponent(t.Context(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
		return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{Link: 0, FeatureEnabled: false}}}, nil
	}).(*component)
	defer func() { require.NoError(t, c.Close()) }()
	c.nvmlInstance = &namedFabricTestInstance{fabricTestInstance: fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}, name: "NVIDIA DGX Rubin NVL8"}
	c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return p2pStatusNotSupported, nil }
	c.checkNVSwitchExistsFunc = func() bool { return false }
	cr := c.Check().(*checkResult)
	t.Logf("health=%s reason=%s expected=%t inactive=%v", cr.health, cr.reason, cr.SystemExpectedNVLink, cr.InactiveNVLinkUUIDs)
	require.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	require.False(t, cr.SystemExpectedNVLink, "an unqualified product name must not invent link requirements")
}

// Keep physical Rubin metadata while simulating an H100 fabric's capabilities.
type overriddenNVLinkInstance struct{ *namedFabricTestInstance }

func (*overriddenNVLinkInstance) Architecture() string { return "rubin" }
func (m *overriddenNVLinkInstance) Capabilities() product.Capabilities {
	return product.CapabilitiesForGPU(m.ProductName(), "", product.HostLinkUnknown)
}

func TestProductOverridePreservesNVLinkExpectation(t *testing.T) {
	devs := map[string]device.Device{
		"GPU-0": &fabricTestDevice{},
		"GPU-1": &fabricTestDevice{},
	}
	c := MockNVLinkComponent(t.Context(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
		return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{FeatureEnabled: false}}}, nil
	}).(*component)
	defer func() { require.NoError(t, c.Close()) }()
	c.nvmlInstance = &overriddenNVLinkInstance{&namedFabricTestInstance{fabricTestInstance: fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}, name: "NVIDIA H100 80GB HBM3"}}
	c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return p2pStatusNotSupported, nil }
	c.checkNVSwitchExistsFunc = func() bool { return false }
	cr := c.Check().(*checkResult)
	require.True(t, cr.SystemExpectedNVLink)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health, cr.reason)
}
