package fabric

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

func TestSuggestedActionsPreserveDeviceFaults(t *testing.T) {
	for _, err := range []error{nvmlerrors.ErrGPULost, nvmlerrors.ErrGPURequiresReset, errors.Join(errors.New("other GPU query failed"), nvmlerrors.ErrGPULost)} {
		require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, SuggestedActions(err, apiv1.HealthStateTypeDegraded).RepairActions)
	}
	for _, err := range []error{nil, errors.New("unavailable telemetry")} {
		require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, SuggestedActions(err, apiv1.HealthStateTypeDegraded).RepairActions)
	}
}

func TestCollectRegistrationStates(t *testing.T) {
	for _, tc := range []struct {
		name            string
		state           device.FabricState
		err             error
		pending, failed bool
	}{
		{"not started", device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_STARTED, Status: nvml.ERROR_UNKNOWN}, nil, true, false},
		{"in progress", device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS, Status: nvml.ERROR_UNKNOWN}, nil, true, false},
		{"completed", device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED}, nil, false, false},
		{"failed", device.FabricState{State: nvml.GPU_FABRIC_STATE_COMPLETED, Status: nvml.ERROR_UNKNOWN}, nil, false, true},
		{"unsupported", device.FabricState{}, nil, false, true},
		{"unknown", device.FabricState{State: 255}, nil, false, true},
		{"pending unhealthy", device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS, HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY}, nil, true, true},
		{"pending degraded bandwidth", device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS, HealthMask: nvml.GPU_FABRIC_HEALTH_MASK_DEGRADED_BW_TRUE << nvml.GPU_FABRIC_HEALTH_MASK_SHIFT_DEGRADED_BW}, nil, true, true},
		{"query failure", device.FabricState{}, errors.New("GPU lost"), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := CollectWith(map[string]device.Device{"GPU-0": nil}, func(device.Device) (device.FabricState, error) { return tc.state, tc.err })
			require.Equal(t, tc.pending, r.Pending)
			require.Equal(t, tc.failed, r.Failed)
			require.Equal(t, !tc.pending && !tc.failed, r.Healthy)
		})
	}
}

func TestCollectMixedFaultOverridesPending(t *testing.T) {
	// Iteration order cannot hide a fault behind another GPU's registration.
	for range 20 {
		i := 0
		r := CollectWith(map[string]device.Device{"GPU-0": nil, "GPU-1": nil}, func(device.Device) (device.FabricState, error) {
			i++
			if i == 1 {
				return device.FabricState{State: nvml.GPU_FABRIC_STATE_IN_PROGRESS}, nil
			}
			return device.FabricState{}, errors.New("query failed")
		})
		require.True(t, r.Pending)
		require.True(t, r.Failed)
		require.False(t, r.Healthy)
		require.Contains(t, r.Reason, "GPU GPU-0: state=In Progress")
		require.Contains(t, r.Reason, "query failed")
	}
	require.True(t, Collect(nil).Failed)
}

func TestUnsupportedFabricApplicability(t *testing.T) {
	unsupported := device.FabricState{State: nvml.GPU_FABRIC_STATE_NOT_SUPPORTED, Status: nvml.ERROR_UNKNOWN}
	for _, tc := range []struct {
		name   string
		states []device.FabricState
		errs   []error
		skip   bool
	}{
		{"unsupported states", []device.FabricState{unsupported, unsupported}, []error{nil, nil}, true},
		{"unsupported APIs", []device.FabricState{{}, {}}, []error{nvml.ERROR_NOT_SUPPORTED, nvml.ERROR_NOT_SUPPORTED}, true},
		{"mixed support", []device.FabricState{unsupported, {State: nvml.GPU_FABRIC_STATE_COMPLETED}}, []error{nil, nil}, false},
		{"query failure", []device.FabricState{unsupported, {}}, []error{nil, nvmlerrors.ErrGPULost}, false},
		{"health fault", []device.FabricState{unsupported, {HealthSummary: nvml.GPU_FABRIC_HEALTH_SUMMARY_UNHEALTHY}}, []error{nil, nil}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := 0
			report := CollectWith(map[string]device.Device{"0": nil, "1": nil}, func(device.Device) (device.FabricState, error) {
				defer func() { i++ }()
				return tc.states[i], tc.errs[i]
			})
			require.Equal(t, tc.skip, report.UnsupportedWithoutFabric("NVIDIA-GB200", false, false))
			require.Equal(t, tc.skip, report.UnsupportedWithoutFabric("NVIDIA-GB300", false, false))
			require.False(t, report.UnsupportedWithoutFabric("NVIDIA-GB200 NVL72", false, false))
			require.False(t, report.UnsupportedWithoutFabric("NVIDIA H100", true, false))
			require.Equal(t, tc.skip, report.UnsupportedWithoutFabric("NVIDIA-Graphics-Device", false, false))
			require.False(t, report.UnsupportedWithoutFabric("NVIDIA-Graphics-Device", false, true))
		})
	}
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, SuggestedActions(nil, apiv1.HealthStateTypeUnhealthy).RepairActions)
}
