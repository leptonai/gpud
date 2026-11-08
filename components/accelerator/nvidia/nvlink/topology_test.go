package nvlink

import (
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

type topologyDevice struct {
	device.Device
	scopes []uint32
}

func (d *topologyDevice) GetNvLinkRemotePciInfo(int) (nvml.PciInfo, nvml.Return) {
	var p nvml.PciInfo
	for i, b := range []byte("0000:06:00.0") {
		p.BusId[i] = int8(b)
	}
	return p, nvml.SUCCESS
}
func (d *topologyDevice) GetNvLinkRemoteDeviceType(int) (nvml.IntNvLinkDeviceType, nvml.Return) {
	return nvml.NVLINK_DEVICE_TYPE_SWITCH, nvml.SUCCESS
}
func (d *topologyDevice) GetFieldValues(values []nvml.FieldValue) nvml.Return {
	for i := range values {
		d.scopes = append(d.scopes, values[i].ScopeId)
		values[i].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_LONG_LONG)
		values[i].Timestamp = 123
		binary.NativeEndian.PutUint64(values[i].Value[:], 1<<54+uint64(i))
	}
	values[1].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
	return nvml.SUCCESS
}

func TestRemoteAndNVLink5Counters(t *testing.T) {
	d := &topologyDevice{}
	r := getRemoteEndpoint(d, 7)
	require.Equal(t, "switch", r.DeviceType)
	require.Equal(t, "0000:06:00.0", r.PCIBusID)
	counters := getLinkFieldCounters(d, 7)
	for _, scope := range d.scopes {
		require.Equal(t, uint32(7), scope)
	}
	require.Equal(t, uint64(1<<54), *counters["recovery_successful_events"].Value)
	require.Nil(t, counters["recovery_failed_events"].Value)
	require.NotEmpty(t, counters["recovery_failed_events"].Error)
	s := NVLinkState{Errors: map[string]string{"replay_errors": "not supported"}, FieldCounters: counters, Remote: r}
	b, err := json.Marshal(s)
	require.NoError(t, err)
	var wire map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &wire))
	require.NotContains(t, wire, "replay_errors")
	require.Contains(t, wire, "errors")
}

func TestExternalFabricPartialFailureDoesNotSuggestLocalRepair(t *testing.T) {
	cr := &checkResult{Evaluation: apiv1.Evaluated(), ExternalFabric: true, SystemExpectedNVLink: true, ActiveNVLinkUUIDs: []string{"GPU-a"}, InactiveNVLinkUUIDs: []string{"GPU-b"}}
	evaluateHealthStateWithThresholds(cr)
	s := cr.HealthStates()[0]
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, s.Health)
	require.Equal(t, apiv1.FaultScopeNVLinkDomain, s.FaultScope)
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeCheckSharedFabric}, s.SuggestedActions.RepairActions)
	require.Contains(t, s.Reason, "GPU-b")
	require.JSONEq(t, `["GPU-b"]`, s.ExtraInfo["gpu_uuids"])
}

type externalFabricInstance struct{ mockNVMLInstance }

func (*externalFabricInstance) ProductName() string { return "NVIDIA GB300" }

func TestCachedAndWatchdogStatesRetainDomainScope(t *testing.T) {
	now := time.Now()
	c := &component{nvmlInstance: &externalFabricInstance{}, getTimeNowFunc: func() time.Time { return now }}
	c.currState = *unhealthyRebootState(now, "post-RX-detect failure")
	state := c.LastHealthStates()[0]
	require.Equal(t, apiv1.FaultScopeNVLinkDomain, state.FaultScope)
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeCheckSharedFabric}, state.SuggestedActions.RepairActions)
	require.NotNil(t, state.Evaluated)
	require.False(t, *state.Evaluated)
	c.currState = apiv1.HealthState{}
	c.monitoringStartedAt = now.Add(-2 * defaultCheckStaleAfter)
	state = c.LastHealthStates()[0]
	require.Equal(t, apiv1.FaultScopeNVLinkDomain, state.FaultScope)
	require.Equal(t, apiv1.NotEvaluatedCollection, state.NotEvaluatedReason)
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeCheckSharedFabric}, state.SuggestedActions.RepairActions)
}
