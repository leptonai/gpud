package nvlink

import (
	"context"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components/accelerator/nvidia/fabric"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

type fabricTestDevice struct {
	device.Device
	state device.FabricState
}

func (d *fabricTestDevice) GetFabricState() (device.FabricState, error) { return d.state, nil }

type fabricTestInstance struct{ mockNVMLInstance }

func (*fabricTestInstance) FabricStateSupported() bool { return true }

func TestCheckFabricRegistrationBeforePeerFailure(t *testing.T) {
	start := time.Date(2026, 11, 5, 7, 33, 14, 0, time.UTC)
	now := start
	dev := &fabricTestDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS, Status: nvml.ERROR_UNKNOWN}}
	devs := map[string]device.Device{}
	for _, uuid := range []string{"GPU-0", "GPU-1", "GPU-2", "GPU-3", "GPU-4", "GPU-5", "GPU-6", "GPU-7"} {
		devs[uuid] = dev
	}
	c := MockNVLinkComponent(context.Background(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
		return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{FeatureEnabled: true}}}, nil
	}).(*component)
	defer func() { require.NoError(t, c.Close()) }()
	c.nvmlInstance = &fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}
	c.getTimeNowFunc = func() time.Time { return now }
	peer := p2pStatusNotSupported
	c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return peer, nil }
	c.getThresholdsFunc = func() ExpectedLinkStates { return ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 8} }

	cr := c.Check().(*checkResult)
	require.Equal(t, 28, cr.PeerNVLinkProbePairCount)
	require.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
	require.Contains(t, cr.reason, "registration in progress")
	require.Equal(t, apiv1.HealthStateTypeDegraded, c.LastHealthStates()[0].Health)
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, cr.HealthStates()[0].SuggestedActions.RepairActions)

	// Replay the reported 77-second recovery: completed registration and P2P OK.
	now = start.Add(77 * time.Second)
	dev.state = device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}
	peer = p2pStatusOK
	require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())

	// Completed registration does not suppress a real all-NS topology failure.
	peer = p2pStatusNotSupported
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, c.Check().HealthStateType())

	// A new incomplete episode has a deadline even if the state alternates.
	now = now.Add(time.Hour)
	dev.state.State = nvml.GPU_FABRIC_STATE_NOT_STARTED
	require.Equal(t, apiv1.HealthStateTypeDegraded, c.Check().HealthStateType())
	now = now.Add(fabric.RegistrationTimeout)
	dev.state.State = nvml.GPU_FABRIC_STATE_IN_PROGRESS
	cr = c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.Contains(t, cr.reason, "did not complete")

	// An explicit fabric fault is immediate, including on the first observation.
	now = now.Add(time.Second)
	dev.state = device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}
	peer = p2pStatusOK
	require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())
	now = now.Add(time.Second)
	dev.state.State = nvml.GPU_FABRIC_STATE_IN_PROGRESS
	dev.state.HealthSummary = nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY
	cr = c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.Contains(t, cr.reason, "health check failed")
	require.Contains(t, cr.reason, "summary=Unhealthy")

	// GPU loss still takes precedence over a registration grace period.
	dev.state.HealthSummary = nvml.GPU_FABRIC_HEALTH_SUMMARY_NOT_SUPPORTED
	c.getNVLinkFunc = func(string, device.Device) (NVLink, error) { return NVLink{}, nvmlerrors.ErrGPULost }
	cr = c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, cr.suggestedActions.RepairActions)
}
