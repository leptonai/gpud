package nvlink

import (
	"errors"
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestRubinUnknownFabricDoesNotJustifyHostReboot(t *testing.T) {
	for _, link := range []product.HostLink{product.HostLinkUnknown, product.HostLinkC2C} {
		for _, queryErr := range []error{nil, errors.New("counter query failed"), nvmlerrors.ErrGPULost, nvmlerrors.ErrGPURequiresReset} {
			t.Run(fmt.Sprintf("link=%d/error=%v", link, queryErr), func(t *testing.T) {
				dev := &fabricTestDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}}
				devs := map[string]device.Device{"GPU-0": dev, "GPU-1": dev}
				c := MockNVLinkComponent(t.Context(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
					return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{FeatureEnabled: false}}}, queryErr
				}).(*component)
				defer func() { require.NoError(t, c.Close()) }()
				c.nvmlInstance = &rubinHostInstance{
					rubinFabricInstance: rubinFabricInstance{fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}},
					link:                link,
				}
				c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return p2pStatusNotSupported, nil }
				cr := c.Check().(*checkResult)
				require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health, cr.reason)
				require.NotNil(t, cr.suggestedActions)
				if errors.Is(queryErr, nvmlerrors.ErrGPULost) || errors.Is(queryErr, nvmlerrors.ErrGPURequiresReset) {
					require.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
				} else {
					require.NotContains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
				}
			})
		}
	}
}

func TestLinkFailurePreservesFabricErrorAndPartialLinks(t *testing.T) {
	fabricErr := nvmlerrors.ErrGPULost
	linkErr := errors.New("counter query failed")
	dev := &fabricTestDevice{err: fabricErr}
	devs := map[string]device.Device{"GPU-0": dev}
	partial := NVLink{UUID: "GPU-0", Supported: true, States: []NVLinkState{{Link: 0, FeatureEnabled: true}}}
	c := MockNVLinkComponent(t.Context(), func() map[string]device.Device { return devs }, func(string, device.Device) (NVLink, error) {
		return partial, linkErr
	}).(*component)
	defer func() { require.NoError(t, c.Close()) }()
	c.nvmlInstance = &rubinHostInstance{
		rubinFabricInstance: rubinFabricInstance{fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}},
		link:                product.HostLinkC2C,
	}
	cr := c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.ErrorIs(t, cr.err, fabricErr)
	require.ErrorIs(t, cr.err, linkErr)
	require.Equal(t, []NVLink{partial}, cr.NVLinks)
	require.Contains(t, cr.HealthStates()[0].Error, fabricErr.Error())
	require.Contains(t, cr.HealthStates()[0].Error, linkErr.Error())
	require.NotNil(t, cr.suggestedActions)
	require.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
}
