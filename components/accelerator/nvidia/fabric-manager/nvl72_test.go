package fabricmanager

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

type evidenceDevice struct {
	device.Device
	state device.FabricState
	err   error
}

func (d *evidenceDevice) GetFabricState() (device.FabricState, error) { return d.state, d.err }

func TestGB300FabricRetainsEveryGPUWhenOneQueryFails(t *testing.T) {
	healthy := device.FabricState{APIVersion: 3, State: nvml.GPU_FABRIC_STATE_COMPLETED, Status: nvml.SUCCESS, HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_HEALTHY, HealthMask: uint32(nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_FALSE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_RECOVERY_FALSE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_RECOVERY |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_ROUTE_UNHEALTHY_FALSE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ROUTE_UNHEALTHY |
		uint32(nvml.GPU_FABRIC_HEALTH_MASK_ACCESS_TIMEOUT_RECOVERY_FALSE)<<nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_ACCESS_TIMEOUT_RECOVERY}
	inst := &fakeNVMLInstanceDevices{mockNVMLInstance: mockNVMLInstance{exists: true, productName: "NVIDIA GB300", supportsFabricState: true}, devs: map[string]device.Device{
		"GPU-a": &evidenceDevice{state: healthy}, "GPU-b": &evidenceDevice{err: errors.New("query failed")}, "GPU-c": &evidenceDevice{state: healthy}, "GPU-d": &evidenceDevice{state: healthy},
	}}
	report := collectFabricState(inst)
	require.Len(t, report.Entries, 4)
	require.False(t, *report.Evaluated)
	require.Error(t, report.Err)
	require.Equal(t, "GPU-b", report.Entries[1].GPUUUID)
	require.NotEmpty(t, report.Entries[1].Error)
	c := &component{ctx: context.Background(), nvmlInstance: inst, checkNVSwitchExistsFunc: func() bool { return false }, collectFabricStateFunc: func() fabricStateReport { return report }}
	s := c.Check().HealthStates()[0]
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, s.Health)
	require.Contains(t, s.Reason, "GPU-b")
	require.JSONEq(t, `["GPU-b"]`, s.ExtraInfo["gpu_uuids"])
	var entries []device.FabricStateEntry
	require.NoError(t, json.Unmarshal([]byte(s.ExtraInfo["fabric_states"]), &entries))
	require.Len(t, entries, 4)
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeCheckSharedFabric}, s.SuggestedActions.RepairActions)
	inst.devs["GPU-b"] = &evidenceDevice{state: healthy}
	report = collectFabricState(inst)
	require.True(t, report.Healthy)
	require.True(t, *report.Evaluated)
}
