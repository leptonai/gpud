package nvlink

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestRubinUpperNVLinks(t *testing.T) {
	dev := testutil.NewMockDevice(&mock.Device{
		GetNvLinkStateFunc: func(link int) (nvml.EnableState, nvml.Return) {
			if link == 35 {
				return nvml.FEATURE_DISABLED, nvml.SUCCESS
			}
			return nvml.FEATURE_ENABLED, nvml.SUCCESS
		},
		GetNvLinkErrorCounterFunc: func(link int, _ nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
			if link == 35 {
				return 9, nvml.SUCCESS
			}
			return 0, nvml.SUCCESS
		},
	}, "rubin", "NVIDIA", "", "0000:01:00.0")
	links, err := GetNVLink("GPU-rubin", dev)
	require.NoError(t, err)
	require.Len(t, links.States, 36)
	require.Equal(t, 35, links.States[35].Link)
	require.False(t, links.States.AllFeatureEnabled())
	require.Equal(t, uint64(9), links.States.TotalCRCErrors())
	require.Equal(t, uint64(9), links.States.TotalReplayErrors())
	require.Equal(t, uint64(9), links.States.TotalRecoveryErrors())
}

func TestNVLinkEnumerationBoundary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		boundary  int
		ret       nvml.Return
		wantError bool
	}{
		{name: "legacy invalid index", boundary: 18, ret: nvml.ERROR_INVALID_ARGUMENT},
		{name: "legacy unsupported index", boundary: 12, ret: nvml.ERROR_NOT_SUPPORTED},
		{name: "upper link query failure", boundary: 35, ret: nvml.ERROR_UNKNOWN, wantError: true},
		{name: "invalid first link", ret: nvml.ERROR_INVALID_ARGUMENT, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dev := testutil.NewMockDevice(&mock.Device{
				GetNvLinkStateFunc: func(link int) (nvml.EnableState, nvml.Return) {
					require.LessOrEqual(t, link, tc.boundary)
					if link == tc.boundary {
						return nvml.FEATURE_DISABLED, tc.ret
					}
					return nvml.FEATURE_ENABLED, nvml.SUCCESS
				},
				GetNvLinkErrorCounterFunc: func(int, nvml.NvLinkErrorCounter) (uint64, nvml.Return) {
					return 0, nvml.ERROR_NOT_SUPPORTED
				},
			}, "", "", "", "0000:01:00.0")
			links, err := GetNVLink("GPU-test", dev)
			if tc.wantError {
				require.ErrorContains(t, err, fmt.Sprintf("link %d", tc.boundary))
			} else {
				require.NoError(t, err)
			}
			require.True(t, links.Supported)
			require.Len(t, links.States, tc.boundary)
		})
	}
}

// rubinFabricInstance models a DGX Vera Rubin NVL72 compute tray: Rubin GPUs
// attached to Vera CPUs over NVLink-C2C, with the NVLink fabric outside the tray.
type rubinFabricInstance struct{ fabricTestInstance }

func (*rubinFabricInstance) ProductName() string          { return "NVIDIA-Graphics-Device" }
func (*rubinFabricInstance) Architecture() string         { return "rubin" }
func (*rubinFabricInstance) FabricManagerSupported() bool { return false }
func (*rubinFabricInstance) Capabilities() product.Capabilities {
	return product.CapabilitiesForGPU("NVIDIA-Graphics-Device", "rubin", product.HostLinkC2C)
}

func TestRubinGenericRegistrationAndRecovery(t *testing.T) {
	for _, count := range []int{1, 4} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			dev := &fabricTestDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS, Status: nvml.ERROR_UNKNOWN}}
			devs := map[string]device.Device{}
			for i := range count {
				devs[fmt.Sprint("GPU-", i)] = dev
			}
			c := MockNVLinkComponent(context.Background(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
				return NVLink{UUID: uuid, Supported: true, States: []NVLinkState{{FeatureEnabled: true}}}, nil
			}).(*component)
			defer func() { require.NoError(t, c.Close()) }()
			c.nvmlInstance = &rubinFabricInstance{fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}}
			now := time.Now()
			c.getTimeNowFunc = func() time.Time { return now }
			c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return p2pStatusOK, nil }
			c.checkNVSwitchExistsFunc = func() bool { return false }
			cr := c.Check().(*checkResult)
			require.Equal(t, apiv1.HealthStateTypeDegraded, cr.health)
			require.Len(t, cr.FabricStates, count)
			now = now.Add(77 * time.Second)
			dev.state = device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}
			require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())
			dev.state.HealthSummary = nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY
			cr = c.Check().(*checkResult)
			require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
			require.NotContains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
			// Unsupported telemetry must not erase the known C2C rack topology,
			// even if all locally observed links and peer probes remain healthy.
			dev.state = device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_SUPPORTED}
			cr = c.Check().(*checkResult)
			require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
			require.NotContains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
			dev.state = device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}
			require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())
		})
	}
}

// NVIDIA's DGX VR NVL72 reboot verification requires every link active once
// the fabric is Completed/Success; one dead link must not leave the tray Healthy.
func TestRubinRegisteredFabricInactiveLink(t *testing.T) {
	completed := device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED, Status: nvml.SUCCESS}
	pending := device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS, Status: nvml.ERROR_UNKNOWN}
	for _, tc := range []struct {
		name        string
		gb200       bool
		emptyStates bool
		state       device.FabricState
		threshold   int
		want        apiv1.HealthStateType
		wantReason  string
	}{
		{name: "rubin registered", state: completed, want: apiv1.HealthStateTypeUnhealthy, wantReason: "GPU GPU-1 links 3"},
		{name: "rubin registered empty link states", emptyStates: true, state: completed, want: apiv1.HealthStateTypeUnhealthy, wantReason: "GPU GPU-1 no active links"},
		{name: "rubin pending", state: pending, want: apiv1.HealthStateTypeDegraded},
		{name: "operator threshold", state: completed, threshold: 1, want: apiv1.HealthStateTypeHealthy},
		{name: "gb200 unchanged", gb200: true, state: completed, want: apiv1.HealthStateTypeHealthy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dev := &fabricTestDevice{state: tc.state}
			devs := map[string]device.Device{}
			for i := range 4 {
				devs[fmt.Sprint("GPU-", i)] = dev
			}
			c := MockNVLinkComponent(context.Background(), func() map[string]device.Device { return devs }, func(uuid string, _ device.Device) (NVLink, error) {
				if tc.emptyStates && uuid == "GPU-1" {
					return NVLink{UUID: uuid, Supported: true}, nil
				}
				states := []NVLinkState{{Link: 0, FeatureEnabled: true}, {Link: 3, FeatureEnabled: uuid != "GPU-1"}}
				return NVLink{UUID: uuid, Supported: true, States: states}, nil
			}).(*component)
			defer func() { require.NoError(t, c.Close()) }()
			base := fabricTestInstance{mockNVMLInstance{devicesFunc: func() map[string]device.Device { return devs }}}
			if tc.gb200 {
				c.nvmlInstance = &namedFabricTestInstance{fabricTestInstance: base, name: "NVIDIA GB200"}
			} else {
				c.nvmlInstance = &rubinFabricInstance{base}
			}
			c.getThresholdsFunc = func() ExpectedLinkStates {
				return ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: tc.threshold}
			}
			c.getPeerNVLinkP2PStatusFn = func(device.Device, device.Device) (string, error) { return p2pStatusOK, nil }
			c.checkNVSwitchExistsFunc = func() bool { return false }

			cr := c.Check().(*checkResult)
			require.Equal(t, tc.want, cr.health, cr.reason)
			if tc.want == apiv1.HealthStateTypeUnhealthy {
				require.Contains(t, cr.reason, tc.wantReason)
				require.NotContains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
			} else {
				require.NotContains(t, cr.reason, "Rubin rack fabric")
			}
		})
	}
}
