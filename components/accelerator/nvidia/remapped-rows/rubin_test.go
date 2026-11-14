package remappedrows

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestRubinMemoryRepairDecisions(t *testing.T) {
	type nvmlState struct {
		bank         nvml.EccBankRemapperStatus_v1
		bankRet      nvml.Return
		repair       nvml.RepairStatus
		repairRet    nvml.Return
		unrepairable uint32
		unrepRet     nvml.Return
	}
	for _, tc := range []struct {
		name    string
		arch    string
		state   nvmlState
		health  apiv1.HealthStateType
		action  apiv1.RepairActionType
		wantErr bool
	}{
		{name: "bank remap pending", arch: "rubin", state: nvmlState{bank: nvml.EccBankRemapperStatus_v1{BPending: 1}},
			health: apiv1.HealthStateTypeUnhealthy, action: apiv1.RepairActionTypeRebootSystem},
		{name: "channel repair pending", arch: "rubin", state: nvmlState{repair: nvml.RepairStatus{BChannelRepairPending: 1}},
			health: apiv1.HealthStateTypeUnhealthy, action: apiv1.RepairActionTypeRebootSystem},
		{name: "unrepairable outranks pending repair", arch: "rubin", state: nvmlState{repair: nvml.RepairStatus{BTpcRepairPending: 1}, unrepairable: 1},
			health: apiv1.HealthStateTypeUnhealthy, action: apiv1.RepairActionTypeHardwareInspection},
		{name: "exhausted spares alone", arch: "rubin", state: nvmlState{bank: nvml.EccBankRemapperStatus_v1{InactiveRemappings: 3, Histogram: nvml.EccBankRemapperHistogram_v1{NoSpareGroupCount: 2}}},
			health: apiv1.HealthStateTypeHealthy},
		{name: "unsupported", arch: "rubin", state: nvmlState{bankRet: nvml.ERROR_FUNCTION_NOT_FOUND, repairRet: nvml.ERROR_NOT_SUPPORTED, unrepRet: nvml.ERROR_NOT_SUPPORTED},
			health: apiv1.HealthStateTypeHealthy},
		{name: "query error", arch: "rubin", state: nvmlState{repairRet: nvml.ERROR_UNKNOWN},
			health: apiv1.HealthStateTypeUnhealthy, wantErr: true},
		{name: "non-Rubin keeps row-only behavior", arch: "blackwell", state: nvmlState{bank: nvml.EccBankRemapperStatus_v1{BPending: 1}, unrepairable: 1},
			health: apiv1.HealthStateTypeHealthy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.state
			dev := testutil.NewMockDevice(&mock.Device{
				GetBankRemapperStatus_v1Func: func() (nvml.EccBankRemapperStatus_v1, nvml.Return) { return s.bank, s.bankRet },
				GetRepairStatusFunc:          func() (nvml.RepairStatus, nvml.Return) { return s.repair, s.repairRet },
				GetUnrepairableMemoryFlag_v1Func: func() (nvml.UnrepairableMemoryStatus_v1, nvml.Return) {
					return nvml.UnrepairableMemoryStatus_v1{BUnrepairableMemory: s.unrepairable}, s.unrepRet
				},
			}, tc.arch, "NVIDIA", "", "0000:01:00.0")
			inst := &mockNVMLInstance{
				getDevicesFunc:     func() map[string]device.Device { return map[string]device.Device{"GPU-0": dev} },
				getProductNameFunc: func() string { return "NVIDIA-Graphics-Device" },
				getMemoryErrorManagementCapabilitiesFunc: func() product.MemoryErrorManagementCapabilities {
					return product.CapabilitiesForGPU("NVIDIA-Graphics-Device", tc.arch, product.HostLinkUnknown).MemoryManagement
				},
				architecture: tc.arch,
			}
			comp, err := New(&components.GPUdInstance{RootCtx: t.Context(), NVMLInstance: inst})
			require.NoError(t, err)
			defer func() { assert.NoError(t, comp.Close()) }()
			c := mustComponent(t, comp)
			c.getRemappedRowsFunc = func(uuid string, _ device.Device) (RemappedRows, error) {
				return RemappedRows{UUID: uuid, Supported: true}, nil
			}

			cr := c.Check().(*checkResult)
			assert.Equal(t, tc.health, cr.health, cr.reason)
			assert.Equal(t, tc.wantErr, cr.err != nil, "err: %v", cr.err)
			if tc.action == "" {
				assert.Nil(t, cr.suggestedActions)
			} else {
				require.NotNil(t, cr.suggestedActions)
				assert.Equal(t, []apiv1.RepairActionType{tc.action}, cr.suggestedActions.RepairActions)
			}
			if tc.arch == "rubin" && (!tc.wantErr || s.bankRet == nvml.SUCCESS || s.repairRet == nvml.SUCCESS || s.unrepRet == nvml.SUCCESS) {
				require.Len(t, cr.MemoryRepairStatuses, 1)
				assert.Contains(t, cr.HealthStates()[0].ExtraInfo["data"], "memory_repair_statuses")
			} else {
				assert.Empty(t, cr.MemoryRepairStatuses)
			}
		})
	}
}

func TestRubinMemoryRepairTotalFailureOmitsStatus(t *testing.T) {
	dev := testutil.NewMockDevice(&mock.Device{
		GetBankRemapperStatus_v1Func: func() (nvml.EccBankRemapperStatus_v1, nvml.Return) {
			return nvml.EccBankRemapperStatus_v1{}, nvml.ERROR_GPU_IS_LOST
		},
	}, "rubin", "NVIDIA", "", "0000:01:00.0")
	inst := &mockNVMLInstance{
		getDevicesFunc:     func() map[string]device.Device { return map[string]device.Device{"GPU-0": dev} },
		getProductNameFunc: func() string { return "NVIDIA-Graphics-Device" },
		getMemoryErrorManagementCapabilitiesFunc: func() product.MemoryErrorManagementCapabilities {
			return product.CapabilitiesForGPU("NVIDIA-Graphics-Device", "rubin", product.HostLinkUnknown).MemoryManagement
		},
		architecture: "rubin",
	}
	comp, err := New(&components.GPUdInstance{RootCtx: t.Context(), NVMLInstance: inst})
	require.NoError(t, err)
	defer func() { assert.NoError(t, comp.Close()) }()
	c := mustComponent(t, comp)
	c.getRemappedRowsFunc = func(uuid string, _ device.Device) (RemappedRows, error) {
		return RemappedRows{UUID: uuid, Supported: true}, nil
	}

	cr := c.Check().(*checkResult)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	require.Equal(t, "error getting memory repair status", cr.reason)
	require.Empty(t, cr.MemoryRepairStatuses)
}
