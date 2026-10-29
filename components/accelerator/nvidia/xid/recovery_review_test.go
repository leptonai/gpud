package xid

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	"github.com/leptonai/gpud/pkg/nvsentinel"
	"github.com/leptonai/gpud/pkg/nvsentinel/nvsentineltest"
)

func TestRecoveryLegacyEventsRemainInformational(t *testing.T) {
	legacyDetail := recoveryDetail(nil)
	assert.Equal(t, apiv1.EventTypeWarning, legacyDetail.EventType)
	assert.Nil(t, legacyDetail.SuggestedActionsByGPUd)
	for _, raw := range []string{
		"154",
		`{"data_source":"kmsg","device_uuid":"PCI:0000:01:00","xid":154,"description":"GPU Recovery Action Changed"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			c, _ := newTestComponentWithNVSentinel(t, nil)
			now := time.Now().UTC()
			ev := eventstore.Event{Time: now, Name: EventNameErrorXid, Type: string(apiv1.EventTypeWarning), ExtraInfo: map[string]string{EventKeyErrorXidData: raw}}
			require.NoError(t, c.eventBucket.Insert(context.Background(), ev))
			apiEvents, err := c.Events(context.Background(), now.Add(-time.Second))
			require.NoError(t, err)
			require.Len(t, apiEvents, 1)
			assert.Equal(t, apiv1.EventTypeWarning, apiEvents[0].Type)
			assert.Contains(t, apiEvents[0].Message, "GPU Recovery Action Changed")
			assert.NotContains(t, apiEvents[0].Message, "malformed")
			for _, events := range []eventstore.Events{{ev}, {{Name: "reboot"}, ev}} {
				state := evolveHealthyState(events, nil, 2)
				assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
				assert.Nil(t, state.SuggestedActions)
			}
		})
	}
}

func TestRecoveryNativeAndNVSentinelShareDeviceOwnership(t *testing.T) {
	devices := map[string]device.Device{
		"GPU-recovery": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:01:00.0"),
	}
	native := recoveryEvent(t, "0000:01:00", "0x2 (OS Reboot)")
	none := recoveryEvent(t, "0000:01:00", "0x0 (None)")
	native.Time = time.Unix(1, 0)
	none.Time = time.Unix(2, 0)
	var payload xidErrorEventDetail
	require.NoError(t, json.Unmarshal([]byte(none.ExtraInfo[EventKeyErrorXidData]), &payload))
	payload.DeviceUUID = "GPU-recovery"
	payload.DataSource = dataSourceNVSentinel
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	none.ExtraInfo[EventKeyErrorXidData] = string(raw)
	state := evolveHealthyState(eventstore.Events{none, native}, devices, 2)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Nil(t, state.SuggestedActions)
}

func TestRecoveryCoverageDoesNotSuppressTransitions(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)
	ev := newNVSentinelXidEvent("154", "PCI:0000:01:00")
	ev.Message = recoveryLine("0000:01:00", "0x2 (OS Reboot)")
	src.RecordCoverage(ev)
	assert.False(t, c.nvsentinelCoversXid(Match(ev.Message)))
	assert.False(t, c.nvsentinelCoversXid(Match(recoveryLine("0000:01:00", "0x0 (None)"))))
	assert.False(t, c.nvsentinelCoversXid(Match(recoveryLine("0000:01:00", "truncated"))))

	ev.Message = recoveryLine("0000:01:00", "truncated")
	src.RecordCoverage(ev)
	assert.False(t, c.nvsentinelCoversXid(Match(ev.Message)))
	assert.False(t, c.nvsentinelCoversXid(Match(recoveryLine("0000:01:00", "differently truncated"))))
	assert.False(t, c.nvsentinelCoversXid(&Error{Xid: 154, DeviceUUID: "PCI:0000:01:00"}))
}

func TestRecoveryDrainAndResetLeavesRowRemappingToItsComponent(t *testing.T) {
	recovery := recoveryEvent(t, "0000:01:00", "0x4 (Drain and Reset)")
	fault := createXidEvent(time.Time{}, 94, apiv1.EventTypeInfo, apiv1.RepairActionTypeIgnoreNoActionRequired)
	var events eventstore.Events
	for range 4 {
		events = append(eventstore.Events{recovery, fault, {Name: "reboot"}}, events...)
		state := evolveHealthyState(events, nil, 2)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
		require.NotNil(t, state.SuggestedActions)
		assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, state.SuggestedActions.RepairActions)
		assert.Contains(t, state.Reason, "Drain and Reset")
	}
}

func TestRecoveryRebootClearsEveryAction(t *testing.T) {
	for _, to := range []string{"0x1 (PF FLR)", "0x2 (OS Reboot)", "0x3 (Drain P2P)", "0x4 (Drain and Reset)", "0x5 (Recover IMEX Domain)", "0x6 (Bus Reset)", "0x7 (System Reboot)", "0x9 (Unknown)", "truncated"} {
		t.Run(to, func(t *testing.T) {
			state := evolveHealthyState(eventstore.Events{{Name: "reboot"}, recoveryEvent(t, "0000:01:00", to)}, nil, 2)
			assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
			assert.Nil(t, state.SuggestedActions)
			assert.NotContains(t, state.Reason, "XID 154")
		})
	}
}

func TestRecoveryUnrecognizedReportCannotReplaceKnownRequest(t *testing.T) {
	for _, to := range []string{"truncated", "0x9 (Unknown)", "0x100000000 (Overflow)"} {
		t.Run(to, func(t *testing.T) {
			reboot := recoveryEvent(t, "0000:01:00", "0x2 (OS Reboot)")
			unknown := recoveryEvent(t, "0000:01:00", to)
			for _, events := range []eventstore.Events{{unknown, reboot}, {reboot, unknown}} {
				state := evolveHealthyState(events, nil, 2)
				assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
				require.NotNil(t, state.SuggestedActions)
				assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
				assert.Contains(t, state.Reason, "OS Reboot")
			}
			// An unreadable report from another GPU must remain visible.
			state := evolveHealthyState(eventstore.Events{recoveryEvent(t, "0000:02:00", to), reboot}, nil, 2)
			assert.Contains(t, state.Reason, "0000:02:00")
		})
	}
}

func TestRecoveryNeverDiagnosesHardwareFromRepeatedGuidance(t *testing.T) {
	var events eventstore.Events
	for _, to := range []string{"0x1 (PF FLR)", "0x4 (Drain and Reset)", "0x2 (OS Reboot)", "0x6 (Bus Reset)", "0x7 (System Reboot)"} {
		events = append(eventstore.Events{recoveryEvent(t, "0000:01:00", to), {Name: "reboot"}}, events...)
	}
	state := evolveHealthyStateWithThresholds(events, nil, 2, Thresholds{Overrides: map[int]ThresholdOverride{154: {RebootThreshold: 1}}})
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	require.NotNil(t, state.SuggestedActions)
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
}

func TestRecoveryMergeKeepsHealthActionsAndReasonTogether(t *testing.T) {
	fault := createXidEvent(time.Time{}, 94, apiv1.EventTypeInfo, apiv1.RepairActionTypeIgnoreNoActionRequired)
	p2p := recoveryEvent(t, "0000:01:00", "0x3 (Drain P2P)")
	state := evolveHealthyState(eventstore.Events{p2p, fault}, nil, 2)
	assert.Equal(t, apiv1.HealthStateTypeDegraded, state.Health)
	require.NotNil(t, state.SuggestedActions)
	assert.Equal(t, apiv1.RepairActionTypeCheckUserAppAndGPU, state.SuggestedActions.RepairActions[0])
	assert.Contains(t, state.Reason, "Drain P2P")
	assert.Contains(t, state.SuggestedActions.Description, "Drain P2P")
}

func TestRecoveryHealthyNVSentinelEventsAreIgnored(t *testing.T) {
	for _, code := range []string{"", "154"} {
		t.Run(code, func(t *testing.T) {
			src := nvsentineltest.NewFakeSource()
			c, _ := newTestComponentWithNVSentinel(t, src)
			ev := newNVSentinelXidEvent(code, "GPU-recovery")
			if code == "" {
				ev.ErrorCodes = nil
			}
			ev.IsHealthy = true
			ev.IsFatal = false
			ev.Action = nvsentinel.RecommendedActionNone
			ev.Message = "Cancelled by SysLogsXIDError error code 163" //nolint:misspell // Match NVSentinel's wire message.
			c.onNVSentinelEvent(ev)
			select {
			case pending := <-c.extraEventCh:
				t.Fatalf("healthy event queued as an XID incident: %+v", pending.event)
			default:
			}
			src.RecordCoverage(ev)
			assert.False(t, c.nvsentinelCoversXid(Match(recoveryLine("0000:01:00", "0x2 (OS Reboot)"))))
		})
	}
}

func TestRecoveryDoesNotChangeOriginatingFaultPrecedence(t *testing.T) {
	hardware := createXidEvent(time.Time{}, 64, apiv1.EventTypeFatal, apiv1.RepairActionTypeHardwareInspection)
	reboot := createXidEvent(time.Time{}, 79, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem)
	state := evolveHealthyState(eventstore.Events{reboot, hardware}, nil, 2)
	require.NotNil(t, state.SuggestedActions)
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
	state = evolveHealthyState(eventstore.Events{{Name: "reboot"}, reboot, hardware}, nil, 2)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
}

func TestRecoveryReadPreservesStoredClassification(t *testing.T) {
	ev := recoveryEvent(t, "0000:01:00", "0x2 (OS Reboot)")
	var payload xidErrorEventDetail
	require.NoError(t, json.Unmarshal([]byte(ev.ExtraInfo[EventKeyErrorXidData]), &payload))
	payload.Description = "classification from the producer"
	payload.SuggestedActionsByGPUd = newSuggestedActions(apiv1.RepairActionTypeCheckUserAppAndGPU)
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	ev.ExtraInfo[EventKeyErrorXidData] = string(raw)
	ev.Type = string(apiv1.EventTypeCritical)
	resolved := resolveXIDEvent(ev, nil)
	assert.Equal(t, string(apiv1.EventTypeCritical), resolved.Type)
	assert.Contains(t, resolved.Message, payload.Description)
	state := evolveHealthyState(eventstore.Events{ev}, nil, 2)
	assert.Equal(t, apiv1.HealthStateTypeDegraded, state.Health)
	require.NotNil(t, state.SuggestedActions)
	assert.Equal(t, apiv1.RepairActionTypeCheckUserAppAndGPU, state.SuggestedActions.RepairActions[0])
}

func TestRecoveryProducersPersistPolicy(t *testing.T) {
	for _, producer := range []string{"kmsg", "nvsentinel"} {
		for _, tc := range []struct {
			to     string
			typ    apiv1.EventType
			health apiv1.HealthStateType
			action apiv1.RepairActionType
		}{
			{"0x0 (None)", apiv1.EventTypeWarning, apiv1.HealthStateTypeHealthy, ""},
			{"0x1 (PF FLR)", apiv1.EventTypeFatal, apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem},
			{"0x2 (OS Reboot)", apiv1.EventTypeFatal, apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem},
			{"0x3 (Drain P2P)", apiv1.EventTypeCritical, apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU},
			{"0x4 (Drain and Reset)", apiv1.EventTypeWarning, apiv1.HealthStateTypeHealthy, ""},
			{"0x5 (Recover IMEX Domain)", apiv1.EventTypeCritical, apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU},
			{"0x6 (Bus Reset)", apiv1.EventTypeFatal, apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem},
			{"0x7 (System Reboot)", apiv1.EventTypeFatal, apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem},
			{"0x9 (Unknown)", apiv1.EventTypeCritical, apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU},
			{"truncated", apiv1.EventTypeCritical, apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU},
		} {
			t.Run(producer+"/"+tc.to, func(t *testing.T) {
				c, _ := newTestComponentWithNVSentinel(t, nil)
				ch := make(chan apiv1.KernelMessage, 1)
				if producer == "nvsentinel" {
					ch = nil
				}
				go c.startKernelTest(ch, time.Hour)
				now := time.Now().UTC()
				line := recoveryLine("0000:01:00", tc.to)
				if producer == "kmsg" {
					ch <- apiv1.KernelMessage{Timestamp: metav1.NewTime(now), Message: line}
				} else {
					ev := newNVSentinelXidEvent("154", "GPU-recovery")
					ev.Message = line
					c.onNVSentinelEvent(ev)
				}
				var stored eventstore.Events
				require.Eventually(t, func() bool {
					var err error
					stored, err = c.eventBucket.Get(context.Background(), now.Add(-time.Second))
					return err == nil && len(stored) == 1
				}, 10*time.Second, 10*time.Millisecond)
				require.Eventually(t, func() bool {
					states := c.LastHealthStates()
					return len(states) == 1 && states[0].Health == tc.health
				}, 10*time.Second, 10*time.Millisecond)
				assert.Equal(t, string(tc.typ), stored[0].Type)
				var payload xidErrorEventDetail
				require.NoError(t, json.Unmarshal([]byte(stored[0].ExtraInfo[EventKeyErrorXidData]), &payload))
				require.NotNil(t, payload.RecoveryAction)
				assert.Equal(t, line, payload.RecoveryAction.Raw)
				assert.Equal(t, producer, payload.DataSource)
				if tc.action == "" {
					assert.Nil(t, payload.SuggestedActionsByGPUd)
				} else {
					require.NotNil(t, payload.SuggestedActionsByGPUd)
					assert.Equal(t, []apiv1.RepairActionType{tc.action}, payload.SuggestedActionsByGPUd.RepairActions)
				}
				apiEvents, err := c.Events(context.Background(), now.Add(-time.Second))
				require.NoError(t, err)
				require.Len(t, apiEvents, 1)
				assert.Equal(t, tc.typ, apiEvents[0].Type)
				assert.Contains(t, apiEvents[0].Message, payload.Description)
			})
		}
	}
}
