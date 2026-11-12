package nvlink

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components/accelerator/nvidia/fabric"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

func TestFabricRecoveryPolicyWithFailedLinks(t *testing.T) {
	for _, name := range []string{"NVIDIA H200", "NVIDIA B300 SXM6 AC", "NVIDIA GB200 NVL72", "NVIDIA GB300"} {
		t.Run(name, func(t *testing.T) {
			dev := &fabricTestDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS}}
			devs := map[string]device.Device{"GPU-0": dev, "GPU-1": dev}
			c := MockNVLinkComponent(context.Background(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
				return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{FeatureEnabled: false}}}, nil
			}).(*component)
			defer func() { require.NoError(t, c.Close()) }()
			c.nvmlInstance = &productFabricInstance{namedFabricTestInstance{fabricTestInstance: fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}, name: name}}
			now := time.Now()
			c.getTimeNowFunc = func() time.Time { return now }
			c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return p2pStatusNotSupported, nil }
			check := func(health apiv1.HealthStateType, action apiv1.RepairActionType) {
				t.Helper()
				cr := c.Check().(*checkResult)
				require.Equal(t, health, cr.health)
				require.Equal(t, []apiv1.RepairActionType{action}, cr.HealthStates()[0].SuggestedActions.RepairActions)
				require.Contains(t, cr.reason, "inactive nvlinks=GPU-0,GPU-1")
				require.Contains(t, cr.reason, "peer nvlink p2p statuses=NS")
			}
			check(apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeIgnoreNoActionRequired)
			now = now.Add(fabric.RegistrationTimeout)
			check(apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeIgnoreNoActionRequired)
			now = now.Add(24 * time.Hour)
			check(apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeIgnoreNoActionRequired)
			dev.state = device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED, HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY}
			check(apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeIgnoreNoActionRequired)
			for _, err := range []error{nvmlerrors.ErrGPULost, nvmlerrors.ErrGPURequiresReset} {
				dev.err = err
				check(apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem)
			}
			dev.err = nil
			dev.state.HealthSummary = nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED
			wantAction := apiv1.RepairActionTypeRebootSystem
			if name == "NVIDIA GB200 NVL72" || name == "NVIDIA GB300" {
				wantAction = apiv1.RepairActionTypeIgnoreNoActionRequired
			}
			// Completed registration alone does not prove external link faults are local.
			check(apiv1.HealthStateTypeUnhealthy, wantAction)
			// Missing port telemetry must not turn an unknown fabric failure into a
			// default hardware recall, or hide an independently reported GPU fault.
			c.getNVLinkFunc = func(string, device.Device) (NVLink, error) {
				return NVLink{}, errors.New("port telemetry unavailable")
			}
			dev.state.State = nvml.GPU_FABRIC_STATE_IN_PROGRESS
			cr := c.Check().(*checkResult)
			require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
			require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, cr.suggestedActions.RepairActions)
			dev.err = nvmlerrors.ErrGPULost
			cr = c.Check().(*checkResult)
			require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, cr.suggestedActions.RepairActions)
		})
	}
}
