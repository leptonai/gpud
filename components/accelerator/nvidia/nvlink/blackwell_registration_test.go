package nvlink

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

type productFabricInstance struct{ namedFabricTestInstance }

func (p *productFabricInstance) FabricStateSupported() bool {
	return product.SupportFabricStateByGPUProduct(p.name)
}
func (p *productFabricInstance) FabricManagerSupported() bool {
	return product.SupportedFMByGPUProduct(p.name)
}

func TestBlackwellCapabilitiesUseRegistrationGuard(t *testing.T) {
	for _, name := range []string{"NVIDIA GB300", "NVIDIA B300 SXM6 AC"} {
		for _, count := range []int{4, 8} {
			t.Run(fmt.Sprintf("%s/%d", name, count), func(t *testing.T) {
				dev := &fabricTestDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS, Status: nvml.ERROR_UNKNOWN}}
				devs := map[string]device.Device{}
				for i := 0; i < count; i++ {
					devs[fmt.Sprint("GPU-", i)] = dev
				}
				c := MockNVLinkComponent(context.Background(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
					return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{FeatureEnabled: true}}}, nil
				}).(*component)
				defer func() { require.NoError(t, c.Close()) }()
				c.nvmlInstance = &productFabricInstance{namedFabricTestInstance{fabricTestInstance: fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}, name: name}}
				now := time.Now()
				c.getTimeNowFunc = func() time.Time { return now }
				peer := p2pStatusNotSupported
				c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return peer, nil }
				c.checkNVSwitchExistsFunc = func() bool { return false }
				cr := c.Check().(*checkResult)
				require.True(t, cr.SystemExpectedNVLink)
				require.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
				require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, cr.suggestedActions.RepairActions)
				now = now.Add(77 * time.Second)
				peer = p2pStatusOK
				dev.state = device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}
				require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())
				peer = p2pStatusNotSupported
				require.Equal(t, apiv1.HealthStateTypeUnhealthy, c.Check().HealthStateType())
			})
		}
	}
}
