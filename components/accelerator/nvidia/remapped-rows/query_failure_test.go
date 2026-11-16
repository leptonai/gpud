package remappedrows

import (
	"errors"
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestPartialMemoryTelemetryIsNotHealthy(t *testing.T) {
	dev := testutil.NewMockDevice(&mock.Device{}, "rubin", "NVIDIA", "", "0000:01:00.0")
	inst := &mockNVMLInstance{
		architecture: "rubin",
		getDevicesFunc: func() map[string]device.Device {
			return map[string]device.Device{"GPU-0": dev, "GPU-1": dev, "GPU-2": dev, "GPU-3": dev}
		},
		getProductNameFunc: func() string { return "NVIDIA-Graphics-Device" },
		getMemoryErrorManagementCapabilitiesFunc: func() product.MemoryErrorManagementCapabilities {
			return product.CapabilitiesForGPU("NVIDIA-Graphics-Device", "rubin", product.HostLinkUnknown).MemoryManagement
		},
	}
	comp, err := New(&components.GPUdInstance{RootCtx: t.Context(), NVMLInstance: inst})
	require.NoError(t, err)
	defer func() { require.NoError(t, comp.Close()) }()
	c := mustComponent(t, comp)
	c.getMemoryRepairStatusFunc = func(uuid string, _ device.Device) (MemoryRepairStatus, error) {
		return MemoryRepairStatus{UUID: uuid}, nil
	}
	queryErr1, queryErr2 := errors.New("GPU-1 query failed"), errors.New("GPU-2 query failed")
	c.getRemappedRowsFunc = func(uuid string, _ device.Device) (RemappedRows, error) {
		switch uuid {
		case "GPU-1":
			return RemappedRows{}, queryErr1
		case "GPU-2":
			return RemappedRows{}, queryErr2
		default:
			return RemappedRows{UUID: uuid, Supported: true}, nil
		}
	}
	result := c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, result.health)
	require.ErrorIs(t, result.err, queryErr1)
	require.ErrorIs(t, result.err, queryErr2)
	require.Len(t, result.RemappedRows, 2)
	require.Nil(t, result.suggestedActions)
}

func TestMemoryRepairDeviceFailurePreservesEarlierErrors(t *testing.T) {
	for _, ret := range []nvml.Return{nvml.ERROR_GPU_IS_LOST, nvml.ERROR_RESET_REQUIRED} {
		for _, failRepair := range []bool{true, false} {
			t.Run(fmt.Sprintf("%v/repair=%t", ret, failRepair), func(t *testing.T) {
				dev := testutil.NewMockDevice(&mock.Device{
					GetBankRemapperStatus_v1Func: func() (nvml.EccBankRemapperStatus_v1, nvml.Return) {
						return nvml.EccBankRemapperStatus_v1{}, nvml.ERROR_UNKNOWN
					},
					GetRepairStatusFunc: func() (nvml.RepairStatus, nvml.Return) {
						if failRepair {
							return nvml.RepairStatus{}, ret
						}
						return nvml.RepairStatus{BChannelRepairPending: 1}, nvml.SUCCESS
					},
					GetUnrepairableMemoryFlag_v1Func: func() (nvml.UnrepairableMemoryStatus_v1, nvml.Return) {
						require.False(t, failRepair, "stop querying a lost or reset-required GPU")
						return nvml.UnrepairableMemoryStatus_v1{}, ret
					},
				}, "rubin", "NVIDIA", "", "0000:01:00.0")
				st, err := GetMemoryRepairStatus("GPU-0", dev)
				require.ErrorContains(t, err, "bank remapper status")
				if ret == nvml.ERROR_GPU_IS_LOST {
					require.ErrorIs(t, err, nvmlerrors.ErrGPULost)
				} else {
					require.ErrorIs(t, err, nvmlerrors.ErrGPURequiresReset)
				}
				require.Equal(t, !failRepair, st.ChannelRepairPending)
			})
		}
	}
}

func TestDeviceFailureRecoveryPreservesHardwareInspection(t *testing.T) {
	t.Setenv(product.EnvEnableRubinHealthRules, "true")
	for _, deviceErr := range []error{nvmlerrors.ErrGPULost, nvmlerrors.ErrGPURequiresReset} {
		for _, fromRepair := range []bool{true, false} {
			for _, unrepairableGPU := range []string{"", "GPU-0", "GPU-2"} {
				t.Run(fmt.Sprintf("%v/repair=%t/unrepairable=%s", deviceErr, fromRepair, unrepairableGPU), func(t *testing.T) {
					dev := testutil.NewMockDevice(&mock.Device{}, "rubin", "NVIDIA", "", "0000:01:00.0")
					inst := &mockNVMLInstance{
						architecture: "rubin",
						getDevicesFunc: func() map[string]device.Device {
							return map[string]device.Device{"GPU-0": dev, "GPU-1": dev, "GPU-2": dev}
						},
						getProductNameFunc: func() string { return "NVIDIA Graphics Device" },
						getMemoryErrorManagementCapabilitiesFunc: func() product.MemoryErrorManagementCapabilities {
							return product.CapabilitiesForGPU("", "rubin", product.HostLinkUnknown).MemoryManagement
						},
					}
					comp, err := New(&components.GPUdInstance{RootCtx: t.Context(), NVMLInstance: inst})
					require.NoError(t, err)
					defer func() { require.NoError(t, comp.Close()) }()
					c := mustComponent(t, comp)
					c.getMemoryRepairStatusFunc = func(uuid string, _ device.Device) (MemoryRepairStatus, error) {
						if uuid == "GPU-1" && fromRepair {
							return MemoryRepairStatus{}, fmt.Errorf("repair query: %w", deviceErr)
						}
						return MemoryRepairStatus{UUID: uuid, UnrepairableMemorySupported: true, UnrepairableMemory: uuid == unrepairableGPU}, nil
					}
					c.getRemappedRowsFunc = func(uuid string, _ device.Device) (RemappedRows, error) {
						if uuid == "GPU-1" && !fromRepair {
							return RemappedRows{}, fmt.Errorf("row query: %w", deviceErr)
						}
						return RemappedRows{UUID: uuid, Supported: true}, nil
					}
					cr := c.Check().(*checkResult)
					require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
					require.ErrorIs(t, cr.err, deviceErr)
					require.NotNil(t, cr.suggestedActions)
					want := apiv1.RepairActionTypeRebootSystem
					if unrepairableGPU != "" {
						want = apiv1.RepairActionTypeHardwareInspection
					}
					require.Equal(t, []apiv1.RepairActionType{want}, cr.suggestedActions.RepairActions)
				})
			}
		}
	}
}
