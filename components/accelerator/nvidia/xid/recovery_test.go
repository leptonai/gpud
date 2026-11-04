package xid

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvsentinel"
	"github.com/leptonai/gpud/pkg/nvsentinel/nvsentineltest"
)

func recoveryLine(device, to string) string {
	return fmt.Sprintf("NVRM: Xid (PCI:%s): 154, GPU recovery action changed from 0x4 (Drain and Reset) to %s", device, to)
}

func recoveryEvent(t *testing.T, device, to string) eventstore.Event {
	t.Helper()
	parsed := Match(recoveryLine(device, to))
	require.NotNil(t, parsed)
	payload := xidErrorEventDetail{Xid: 154, DeviceUUID: parsed.DeviceUUID, RecoveryAction: parsed.RecoveryAction, Description: parsed.Detail.Description, SuggestedActionsByGPUd: parsed.Detail.SuggestedActionsByGPUd}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return eventstore.Event{Name: EventNameErrorXid, Type: string(parsed.Detail.EventType), ExtraInfo: map[string]string{EventKeyErrorXidData: string(raw)}}
}

func TestRecoveryActionPolicy(t *testing.T) {
	cases := []struct {
		to     string
		typ    apiv1.EventType
		health apiv1.HealthStateType
		action apiv1.RepairActionType
		valid  bool
	}{
		{"0x0 (None)", apiv1.EventTypeWarning, apiv1.HealthStateTypeHealthy, "", true},
		{"0x1 (GPU Reset Required)", apiv1.EventTypeFatal, apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem, true},
		{"0x2 (Node Reboot Required)", apiv1.EventTypeFatal, apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem, true},
		{"0x3 (Drain P2P)", apiv1.EventTypeCritical, apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU, true},
		{"0x4 (Drain and Reset)", apiv1.EventTypeWarning, apiv1.HealthStateTypeHealthy, "", true},
		{"0x5 (Recover IMEX Domain)", apiv1.EventTypeCritical, apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU, true},
		{"0x6 (Bus Reset)", apiv1.EventTypeFatal, apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem, true},
		{"0x7 (System Reboot)", apiv1.EventTypeFatal, apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem, true},
		{"0x1 (PF FLR)", apiv1.EventTypeFatal, apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem, true},
		{"0x2 (OS Reboot)", apiv1.EventTypeFatal, apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem, true},
		{"0x9 (Node Reboot Required)", apiv1.EventTypeCritical, apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU, true},
		{"not-a-number (Node Reboot Required)", apiv1.EventTypeCritical, apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU, false},
		{"0x100000000 (Node Reboot Required)", apiv1.EventTypeCritical, apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU, false},
		{"0x3 (Node Reboot Required)", apiv1.EventTypeCritical, apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU, true},
	}
	for _, tc := range cases {
		t.Run(tc.to, func(t *testing.T) {
			parsed := Match(recoveryLine("0000:01:00", tc.to))
			require.NotNil(t, parsed.RecoveryAction)
			assert.Equal(t, tc.valid, parsed.RecoveryAction.Valid)
			assert.Equal(t, tc.typ, parsed.Detail.EventType)
			if tc.valid {
				assert.Equal(t, uint32(4), parsed.RecoveryAction.From)
				assert.Equal(t, "Drain and Reset", parsed.RecoveryAction.FromName)
			}
			state := evolveHealthyState(eventstore.Events{recoveryEvent(t, "0000:01:00", tc.to)}, nil, 2)
			assert.Equal(t, tc.health, state.Health)
			if tc.action == "" {
				assert.Nil(t, state.SuggestedActions)
			} else {
				require.NotNil(t, state.SuggestedActions)
				assert.Equal(t, tc.action, state.SuggestedActions.RepairActions[0])
			}
			resolved := resolveXIDEvent(recoveryEvent(t, "0000:01:00", tc.to), nil)
			assert.Equal(t, string(tc.typ), resolved.Type)
			if tc.valid {
				assert.Contains(t, resolved.Message, "0x4 (Drain and Reset)")
				assert.Contains(t, resolved.Message, tc.to)
			}
		})
	}
	legacy := eventstore.Event{Name: EventNameErrorXid, ExtraInfo: map[string]string{EventKeyErrorXidData: "154"}}
	assert.Equal(t, apiv1.HealthStateTypeHealthy, evolveHealthyState(eventstore.Events{legacy}, nil, 2).Health)
}

func TestRecoveryOwnershipAndOriginalFault(t *testing.T) {
	reboot := recoveryEvent(t, "0000:01:00", "0x2 (Node Reboot Required)")
	none := recoveryEvent(t, "0000:01:00", "0x0 (None)")
	p2p := recoveryEvent(t, "0000:01:00", "0x3 (Drain P2P)")
	other := recoveryEvent(t, "0000:02:00", "0x2 (Node Reboot Required)")
	reboot.Time = time.Unix(1, 0)
	other.Time = reboot.Time
	none.Time = time.Unix(2, 0)
	p2p.Time = none.Time
	hardware := createXidEvent(time.Time{}, 79, apiv1.EventTypeFatal, apiv1.RepairActionTypeHardwareInspection)
	state := evolveHealthyState(eventstore.Events{reboot, hardware}, nil, 2)
	assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	assert.Contains(t, state.Reason, "XID 79")
	assert.Contains(t, state.Reason, "XID 154")
	state = evolveHealthyState(eventstore.Events{none, reboot, hardware}, nil, 2)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	assert.NotContains(t, state.Reason, "XID 154")
	state = evolveHealthyState(eventstore.Events{none, reboot, other}, nil, 2)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Contains(t, state.Reason, "0000:02:00")
	assert.NotContains(t, state.Reason, "0000:01:00")
	state = evolveHealthyState(eventstore.Events{p2p, reboot}, nil, 2)
	assert.Equal(t, apiv1.HealthStateTypeDegraded, state.Health)
	assert.Equal(t, apiv1.RepairActionTypeCheckUserAppAndGPU, state.SuggestedActions.RepairActions[0])
	assert.Equal(t, apiv1.HealthStateTypeHealthy, evolveHealthyState(eventstore.Events{none, reboot}, nil, 2).Health)
}

func TestRecoveryRebootCycles(t *testing.T) {
	a := recoveryEvent(t, "0000:01:00", "0x2 (Node Reboot Required)")
	b := recoveryEvent(t, "0000:02:00", "0x2 (Node Reboot Required)")
	r := eventstore.Event{Name: "reboot"}
	assert.Equal(t, apiv1.HealthStateTypeHealthy, evolveHealthyState(eventstore.Events{r, a}, nil, 2).Health)
	state := evolveHealthyState(eventstore.Events{a, r, a}, nil, 2)
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
	state = evolveHealthyState(eventstore.Events{a, r, a, r, a}, nil, 2)
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
	state = evolveHealthyState(eventstore.Events{b, r, a, r, a}, nil, 2)
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
	state = evolveHealthyState(eventstore.Events{a, r, r, a}, nil, 2)
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
	state = evolveHealthyStateWithThresholds(eventstore.Events{a, r, a}, nil, 2, Thresholds{Overrides: map[int]ThresholdOverride{154: {RebootThreshold: 1}}})
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0], "XID 154 does not diagnose hardware failure, even with a threshold override")
}

// Real producer -> SQLite eventstore -> Events API -> health consumer smoke.
// This does not execute any GPU repair or write to a host kernel log.
func TestRecoveryProducerEventStoreHealth(t *testing.T) {
	c, _ := newTestComponentWithNVSentinel(t, nil)
	ch := make(chan kmsg.Message, 1)
	go c.start(ch, time.Hour)
	now := time.Now().UTC()
	ch <- kmsg.Message{Timestamp: metav1.NewTime(now), Message: GetMessageToInject(154).Message}
	require.Eventually(t, func() bool {
		states := c.LastHealthStates()
		return len(states) == 1 && states[0].Health == apiv1.HealthStateTypeUnhealthy
	}, 10*time.Second, 10*time.Millisecond)
	events, err := c.Events(context.Background(), now.Add(-time.Second))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, apiv1.EventTypeFatal, events[0].Type)
	assert.Contains(t, events[0].Message, "0x2 (Node Reboot Required)")
	stored, err := c.eventBucket.Get(context.Background(), now.Add(-time.Second))
	require.NoError(t, err)
	var payload xidErrorEventDetail
	require.NoError(t, json.Unmarshal([]byte(stored[0].ExtraInfo[EventKeyErrorXidData]), &payload))
	require.NotNil(t, payload.RecoveryAction)
	assert.Equal(t, uint32(2), payload.RecoveryAction.To)
	assert.Equal(t, "Node Reboot Required", payload.RecoveryAction.ToName)
	assert.Equal(t, "kmsg", payload.DataSource)
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, c.LastHealthStates()[0].SuggestedActions.RepairActions[0])
}

func TestRecoveryNVSentinelNoneReport(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)
	go c.start(nil, time.Hour)
	ev := newNVSentinelXidEvent("154", "GPU-recovery")
	ev.GeneratedTimestamp = time.Now().UTC().Truncate(time.Second).Add(-time.Second)
	ev.Message = recoveryLine("0000:01:00", "0x3 (Drain P2P)")
	ev.Action = nvsentinel.RecommendedActionContactSupport
	ev.IsFatal = false
	src.Send(ev)
	require.Eventually(t, func() bool { return c.LastHealthStates()[0].Health == apiv1.HealthStateTypeDegraded }, 10*time.Second, 10*time.Millisecond)
	assert.Contains(t, c.LastHealthStates()[0].SuggestedActions.Description, "quiesce P2P")
	assert.False(t, c.hasStoredXidTwin(154, "GPU-recovery", ev.GeneratedTimestamp))
	ev.ID = "recovery-none"
	ev.GeneratedTimestamp = ev.GeneratedTimestamp.Add(time.Second)
	ev.Message = recoveryLine("0000:01:00", "0x0 (None)")
	ev.Action = nvsentinel.RecommendedActionNone
	src.Send(ev)
	require.Eventually(t, func() bool { return c.LastHealthStates()[0].Health == apiv1.HealthStateTypeHealthy }, 10*time.Second, 10*time.Millisecond)
	// The second real NVSentinel event is healthy and has no error code.
	ev.IsHealthy = true
	ev.ErrorCodes = nil
	ev.Message = "GPU recovery action returned to None"
	c.onNVSentinelEvent(ev)
	events, err := c.Events(context.Background(), ev.GeneratedTimestamp.Add(-time.Minute))
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestRecoveryScan(t *testing.T) {
	comp, err := New(&components.GPUdInstance{RootCtx: context.Background(), NVMLInstance: createMockNVMLInstance()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = comp.Close() })
	c := mustComponent(t, comp)
	for _, tc := range []struct {
		to     string
		health apiv1.HealthStateType
	}{
		{"0x2 (Node Reboot Required)", apiv1.HealthStateTypeUnhealthy},
		{"0x3 (Drain P2P)", apiv1.HealthStateTypeDegraded},
		{"0x4 (Drain and Reset)", apiv1.HealthStateTypeHealthy},
		{"0x5 (Recover IMEX Domain)", apiv1.HealthStateTypeDegraded},
		{"0x6 (Bus Reset)", apiv1.HealthStateTypeUnhealthy},
		{"0x7 (System Reboot)", apiv1.HealthStateTypeUnhealthy},
		{"0x0 (None)", apiv1.HealthStateTypeHealthy},
	} {
		c.readAllKmsg = func(context.Context) ([]kmsg.Message, error) {
			return []kmsg.Message{{Timestamp: metav1.NewTime(time.Now()), Message: recoveryLine("0000:01:00", tc.to)}}, nil
		}
		result := c.Check()
		assert.Equal(t, tc.health, result.HealthStateType())
		assert.Equal(t, tc.health, result.HealthStates()[0].Health)
	}
}

func TestRecoveryPreservesNVLinkFallbackAndLinkedNodeAction(t *testing.T) {
	line := "NVRM: Xid (PCI:0000:01:00): 145, RLW_SRC_TRACK Nonfatal XC0 i0 Link 00 (0x00000007 0x00000004 0x00000000 0x00000000)"
	parsed := Match(line)
	require.NotNil(t, parsed)
	require.NotNil(t, parsed.Detail.SuggestedActionsByGPUd)
	assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, parsed.Detail.SuggestedActionsByGPUd.RepairActions[0], "XID 145 fallback remains unchanged by the XID 154 fix")
	fault := createXidEvent(time.Time{}, 145, parsed.Detail.EventType, parsed.Detail.SuggestedActionsByGPUd.RepairActions[0])
	state := evolveHealthyState(eventstore.Events{fault}, nil, 2)
	assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	state = evolveHealthyState(eventstore.Events{recoveryEvent(t, "0000:01:00", "0x2 (Node Reboot Required)"), fault}, nil, 2)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
	assert.Contains(t, state.Reason, "XID 145")
	assert.Contains(t, state.Reason, "XID 154")
}

func TestRecoveryScanNoneDoesNotClearOriginalFault(t *testing.T) {
	comp, err := New(&components.GPUdInstance{RootCtx: context.Background(), NVMLInstance: createMockNVMLInstance()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = comp.Close() })
	c := mustComponent(t, comp)
	now := time.Now()
	c.readAllKmsg = func(context.Context) ([]kmsg.Message, error) {
		return []kmsg.Message{
			{Timestamp: metav1.NewTime(now), Message: "NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus"},
			{Timestamp: metav1.NewTime(now.Add(time.Second)), Message: recoveryLine("0000:01:00", "0x0 (None)")},
		}, nil
	}
	result := c.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "XID 79")
}
