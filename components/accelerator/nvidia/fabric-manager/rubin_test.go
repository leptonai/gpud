package fabricmanager

import (
	"context"
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func rubinInstance(name string, link product.HostLink, count int, state device.FabricState) *fakeNVMLInstanceDevices {
	caps := product.CapabilitiesForGPU(name, "rubin", link)
	devs := map[string]device.Device{}
	for i := range count {
		devs[fmt.Sprint("GPU-", i)] = &mockFabricStateDevice{state: state}
	}
	return &fakeNVMLInstanceDevices{mockNVMLInstance: mockNVMLInstance{
		exists: true, productName: name, architecture: "rubin", hostLink: link,
		supportsFM: caps.FabricManager, supportsFabricState: caps.FabricState,
	}, devs: devs}
}

// DGX Vera Rubin NVL72 compute trays (C2C to Vera) rely on NVOS switch trays
// for FM, so the local daemon must never be consulted.
func TestRubinRackFabric(t *testing.T) {
	for _, count := range []int{1, 4} {
		for _, tc := range []struct {
			name, product string
			link          product.HostLink
			state         device.FabricState
			imexRunning   bool
			want          apiv1.HealthStateType
			wantIMEX      string
		}{
			{name: "C2C healthy with IMEX", product: "NVIDIA-Graphics-Device", link: product.HostLinkC2C, state: device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}, imexRunning: true, want: apiv1.HealthStateTypeHealthy, wantIMEX: imexStateRunning},
			{name: "C2C healthy without IMEX stays healthy", product: "NVIDIA-Graphics-Device", link: product.HostLinkC2C, state: device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}, want: apiv1.HealthStateTypeHealthy, wantIMEX: imexStateNotRunning},
			{name: "C2C pending", product: "NVIDIA-Graphics-Device", link: product.HostLinkC2C, state: device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS, Status: nvml.ERROR_UNKNOWN}, want: apiv1.HealthStateTypeDegraded},
			{name: "C2C fault", product: "NVIDIA-Graphics-Device", link: product.HostLinkC2C, state: device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED, HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY}, want: apiv1.HealthStateTypeUnhealthy, wantIMEX: imexStateNotRunning},
			{name: "explicit NVL72 must have telemetry", product: "NVIDIA-Vera-Rubin-NVL72", state: device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_SUPPORTED}, want: apiv1.HealthStateTypeUnhealthy},
			{name: "C2C must have telemetry even with a generic name", product: "NVIDIA-Graphics-Device", link: product.HostLinkC2C, state: device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_SUPPORTED}, want: apiv1.HealthStateTypeUnhealthy},
		} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, count), func(t *testing.T) {
				inst := rubinInstance(tc.product, tc.link, count, tc.state)
				collected := false
				c := &component{
					ctx: context.Background(), nvmlInstance: inst,
					getCountLspci:           func(context.Context) (int, error) { return count, nil },
					checkNVSwitchExistsFunc: func() bool { return false },
					collectFabricStateFunc:  func() fabricStateReport { collected = true; return collectFabricState(inst) },
					checkFMExistsFunc:       func() bool { t.Fatal("NVOS manages the rack fabric"); return false },
					checkIMEXActiveFunc:     func() bool { return tc.imexRunning },
				}
				cr := c.Check().(*checkResult)
				require.True(t, collected)
				require.Equal(t, tc.want, cr.health)
				require.Equal(t, tc.wantIMEX, cr.IMEXState)
				if cr.suggestedActions != nil {
					require.NotContains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
				}
			})
		}
	}
}

// PCIe-attached Rubin (DGX Rubin NVL8) owns its NVSwitches locally: once NVML
// shows a fabric, a missing nv-fabricmanager must fail the check. Without
// fabric telemetry, or without host-link evidence, the daemon is not demanded.
func TestRubinLocalFabricManager(t *testing.T) {
	registered := device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}
	unsupported := device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_SUPPORTED}
	for _, tc := range []struct {
		name     string
		link     product.HostLink
		state    device.FabricState
		fmExists bool
		fmActive bool
		want     apiv1.HealthStateType
		wantFM   bool
	}{
		{name: "PCIe fabric without FM", link: product.HostLinkPCIe, state: registered, want: apiv1.HealthStateTypeUnhealthy},
		{name: "PCIe fabric with active FM", link: product.HostLinkPCIe, state: registered, fmExists: true, fmActive: true, want: apiv1.HealthStateTypeHealthy, wantFM: true},
		{name: "PCIe fabric with stopped FM", link: product.HostLinkPCIe, state: registered, fmExists: true, want: apiv1.HealthStateTypeUnhealthy},
		{name: "PCIe without fabric telemetry", link: product.HostLinkPCIe, state: unsupported, want: apiv1.HealthStateTypeHealthy},
		{name: "unknown link does not demand FM", link: product.HostLinkUnknown, state: registered, want: apiv1.HealthStateTypeHealthy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inst := rubinInstance("NVIDIA-Graphics-Device", tc.link, 8, tc.state)
			c := &component{
				ctx: context.Background(), nvmlInstance: inst,
				getCountLspci:           func(context.Context) (int, error) { return 8, nil },
				checkNVSwitchExistsFunc: func() bool { return false },
				collectFabricStateFunc:  func() fabricStateReport { return collectFabricState(inst) },
				checkFMExistsFunc:       func() bool { return tc.fmExists },
				checkFMActiveFunc:       func() bool { return tc.fmActive },
				checkIMEXActiveFunc:     func() bool { t.Fatal("IMEX applies only to external rack fabrics"); return false },
			}
			cr := c.Check().(*checkResult)
			require.Equal(t, tc.want, cr.health, cr.reason)
			require.Equal(t, tc.wantFM, cr.FabricManagerActive)
			require.Empty(t, cr.IMEXState)
		})
	}
}
