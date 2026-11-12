package fabricmanager

import (
	"context"
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestCheckBlackwellFabric(t *testing.T) {
	for _, name := range []string{"NVIDIA GB200", "NVIDIA GB300", "NVIDIA-B300-SXM6-AC", "NVIDIA B200", "NVIDIA B100"} {
		for _, healthy := range []bool{true, false} {
			t.Run(name+map[bool]string{true: "/healthy", false: "/unhealthy"}[healthy], func(t *testing.T) {
				rack := name == "NVIDIA GB200" || name == "NVIDIA GB300"
				collected, checkedFM := false, false
				comp := &component{
					ctx: context.Background(),
					nvmlInstance: &mockNVMLInstance{
						exists: true, productName: name,
						supportsFM:          product.SupportedFMByGPUProduct(name),
						supportsFabricState: product.SupportFabricStateByGPUProduct(name),
					},
					// A rack fabric is reachable even when only one local GPU is visible.
					getCountLspci: func(context.Context) (int, error) {
						if rack {
							return 1, nil
						}
						return 8, nil
					},
					checkNVSwitchExistsFunc: func() bool { return !rack },
					collectFabricStateFunc: func() fabricStateReport {
						collected = true
						if healthy {
							return fabricStateReport{Healthy: true}
						}
						return fabricStateReport{Reason: "bandwidth degraded"}
					},
					checkFMExistsFunc: func() bool { checkedFM = true; return true },
					checkFMActiveFunc: func() bool { return true },
				}
				cr := comp.Check().(*checkResult)
				require.True(t, collected, "fabric telemetry must be checked without a product-name override")
				assert.Equal(t, !rack, checkedFM, "only HGX/DGX systems require an on-node FM daemon")
				if healthy {
					assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
				} else {
					assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
					assert.Contains(t, cr.reason, "bandwidth degraded")
				}
			})
		}
	}
}

func TestCheckGraceBlackwellTopology(t *testing.T) {
	unsupported := &mockFabricStateDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_SUPPORTED, Status: nvml.ERROR_UNKNOWN}}
	unsupportedAPI := &mockFabricStateDevice{err: fmt.Errorf("unsupported: %w", nvml.ERROR_NOT_SUPPORTED)}
	completed := &mockFabricStateDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}}
	fault := &mockFabricStateDevice{state: device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_SUPPORTED, HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY}}
	lost := &mockFabricStateDevice{err: nvmlerrors.ErrGPULost}
	for _, tc := range []struct {
		name, product            string
		devs                     map[string]device.Device
		localSwitch, testingMode bool
		want                     apiv1.HealthStateType
		supported, collected     bool
	}{
		{name: "bare GB200 unsupported state", product: "NVIDIA GB200", devs: map[string]device.Device{"0": unsupported}, want: apiv1.HealthStateTypeHealthy, collected: true},
		{name: "bare GB200 unsupported API", product: "NVIDIA GB200", devs: map[string]device.Device{"0": unsupportedAPI, "1": unsupportedAPI}, want: apiv1.HealthStateTypeHealthy, collected: true},
		{name: "explicit NVL4", product: "NVIDIA GB200 NVL4", devs: map[string]device.Device{"0": unsupportedAPI}, want: apiv1.HealthStateTypeHealthy},
		{name: "explicit NVL72 unsupported", product: "NVIDIA GB200 NVL72", devs: map[string]device.Device{"0": unsupportedAPI}, want: apiv1.HealthStateTypeUnhealthy, supported: true, collected: true},
		{name: "detected switch unsupported", product: "NVIDIA GB200", devs: map[string]device.Device{"0": unsupported}, localSwitch: true, want: apiv1.HealthStateTypeUnhealthy, supported: true, collected: true},
		{name: "fault injection", product: "NVIDIA GB200", devs: map[string]device.Device{"0": unsupported}, testingMode: true, want: apiv1.HealthStateTypeUnhealthy, supported: true, collected: true},
		{name: "bare GB200 rack healthy", product: "NVIDIA GB200", devs: map[string]device.Device{"0": completed}, want: apiv1.HealthStateTypeHealthy, supported: true, collected: true},
		{name: "mixed support", product: "NVIDIA GB200", devs: map[string]device.Device{"0": unsupportedAPI, "1": completed}, want: apiv1.HealthStateTypeUnhealthy, supported: true, collected: true},
		{name: "explicit health fault", product: "NVIDIA GB200", devs: map[string]device.Device{"0": fault}, want: apiv1.HealthStateTypeUnhealthy, supported: true, collected: true},
		{name: "lost GPU", product: "NVIDIA GB200", devs: map[string]device.Device{"0": unsupportedAPI, "1": lost}, want: apiv1.HealthStateTypeUnhealthy, supported: true, collected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inst := &fakeNVMLInstanceDevices{mockNVMLInstance: mockNVMLInstance{
				exists: true, productName: tc.product,
				supportsFM:          product.SupportedFMByGPUProduct(tc.product),
				supportsFabricState: product.SupportFabricStateByGPUProduct(tc.product),
			}, devs: tc.devs}
			collected := false
			c := &component{
				ctx: context.Background(), nvmlInstance: inst, testingMode: tc.testingMode,
				getCountLspci:           func(context.Context) (int, error) { return 1, nil },
				checkNVSwitchExistsFunc: func() bool { return tc.localSwitch },
				collectFabricStateFunc:  func() fabricStateReport { collected = true; return collectFabricState(inst) },
				checkFMExistsFunc:       func() bool { t.Error("GB systems must not require a local FM daemon"); return false },
			}
			cr := c.Check().(*checkResult)
			assert.Equal(t, tc.want, cr.health)
			assert.Equal(t, tc.supported, cr.FabricStateSupported)
			assert.Equal(t, tc.collected, collected)
		})
	}
}
