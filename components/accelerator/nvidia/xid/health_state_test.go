package xid

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func createXidEvent(eventTime time.Time, xid uint64, eventType apiv1.EventType, suggestedAction apiv1.RepairActionType) eventstore.Event {
	xidErr := xidErrorEventDetail{
		Xid:        xid,
		DataSource: "test",
		DeviceUUID: "PCI:0000:9b:00",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{suggestedAction},
		},
	}
	xidData, _ := json.Marshal(xidErr)
	ret := eventstore.Event{
		Name:      EventNameErrorXid,
		Type:      string(eventType),
		ExtraInfo: map[string]string{EventKeyErrorXidData: string(xidData)},
	}
	if !eventTime.IsZero() {
		ret.Time = eventTime
	}
	return ret
}

func TestStateUpdateBasedOnEvents(t *testing.T) {
	t.Run("no event found", func(t *testing.T) {
		state := evolveHealthyState(eventstore.Events{}, nil, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
		assert.Equal(t, "XIDComponent is healthy", state.Reason)
	})

	mockDevice := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:9b:00.0")

	t.Run("critical xid", func(t *testing.T) {
		events := eventstore.Events{
			createXidEvent(time.Time{}, 123, apiv1.EventTypeCritical, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events, map[string]device.Device{"GPU-b850f46d-d5ea-c752-ddf3-c4453e44d3f7": mockDevice}, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
		assert.Equal(t, "XID 123 (SPI PMU RPC Write Failure) detected on GPU PCI:0000:9b:00 UUID:GPU-b850f46d-d5ea-c752-ddf3-c4453e44d3f7", state.Reason)
	})

	t.Run("fatal xid", func(t *testing.T) {
		events := eventstore.Events{
			createXidEvent(time.Time{}, 456, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		t.Logf("original type=%s", events[0].Type)
		resolved := resolveXIDEvent(events[0], map[string]device.Device{"GPU-b850f46d-d5ea-c752-ddf3-c4453e44d3f7": mockDevice})
		t.Logf("resolved type=%s msg=%s", resolved.Type, resolved.Message)
		state := evolveHealthyState(events, map[string]device.Device{"GPU-b850f46d-d5ea-c752-ddf3-c4453e44d3f7": mockDevice}, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
		assert.Equal(t, "XID 456 detected on GPU PCI:0000:9b:00 UUID:GPU-b850f46d-d5ea-c752-ddf3-c4453e44d3f7", state.Reason)
	})

	t.Run("reboot recover", func(t *testing.T) {
		events := eventstore.Events{
			{Name: "reboot"},
			createXidEvent(time.Time{}, 789, apiv1.EventTypeCritical, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})

	t.Run("reboot multiple time cannot recover, should be in degraded state", func(t *testing.T) {
		events := eventstore.Events{
			createXidEvent(time.Time{}, 94, apiv1.EventTypeCritical, apiv1.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createXidEvent(time.Time{}, 94, apiv1.EventTypeCritical, apiv1.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createXidEvent(time.Time{}, 94, apiv1.EventTypeCritical, apiv1.RepairActionTypeRebootSystem),
			createXidEvent(time.Time{}, 31, apiv1.EventTypeWarning, apiv1.RepairActionTypeCheckUserAppAndGPU),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeDegraded, state.Health)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	})

	t.Run("EmptyEvents_ReturnsHealthy", func(t *testing.T) {
		events := eventstore.Events{}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
		assert.Nil(t, state.SuggestedActions)
		assert.Equal(t, "XIDComponent is healthy", state.Reason)
	})

	t.Run("rebootCountingResetAfterPurgeMatchesLegacy", func(t *testing.T) {
		now := time.Now()
		localEvents := eventstore.Events{
			createXidEvent(now, 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			{Time: now.Add(-1 * time.Minute), Name: "SetHealthy"},
			createXidEvent(now.Add(-2*time.Minute), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			createXidEvent(now.Add(-4*time.Minute), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}

		trimmed := trimEventsAfterSetHealthy(localEvents)
		require.Len(t, trimmed, 1)
		assert.Equal(t, now.Unix(), trimmed[0].Time.Unix())

		rebootEvents := eventstore.Events{
			{Time: now.Add(-3 * time.Minute), Name: "reboot"},
			{Time: now.Add(-5 * time.Minute), Name: "reboot"},
		}

		merged := mergeEvents(rebootEvents, trimmed)
		require.Len(t, merged, 3)
		assert.Equal(t, []string{"error_xid", "reboot", "reboot"}, []string{merged[0].Name, merged[1].Name, merged[2].Name})

		state := evolveHealthyState(merged, nil, DefaultRebootThreshold)
		require.NotNil(t, state.SuggestedActions)
		require.NotEmpty(t, state.SuggestedActions.RepairActions)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
		assert.Equal(t, apiv1.HealthStateTypeDegraded, state.Health)
	})

	t.Run("invalid xid", func(t *testing.T) {
		events := eventstore.Events{
			{
				Name:      EventNameErrorXid,
				Type:      string(apiv1.EventTypeCritical),
				ExtraInfo: map[string]string{EventKeyErrorXidData: "invalid json"},
			},
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})
}

func Test_xidErrorEventDetailJSON(t *testing.T) {
	testTime := metav1.Time{Time: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}

	t.Run("successful marshaling", func(t *testing.T) {
		xidErr := xidErrorEventDetail{
			Time:       testTime,
			DataSource: "test-source",
			DeviceUUID: "test-uuid",
			Xid:        123,
			SuggestedActionsByGPUd: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem},
			},
		}

		jsonBytes, err := json.Marshal(xidErr)
		assert.NoError(t, err)
		assert.NotNil(t, jsonBytes)

		// Verify JSON structure by unmarshaling
		var unmarshaled xidErrorEventDetail
		err = json.Unmarshal(jsonBytes, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, xidErr.Time.UTC(), unmarshaled.Time.UTC())
		assert.Equal(t, xidErr.DataSource, unmarshaled.DataSource)
		assert.Equal(t, xidErr.DeviceUUID, unmarshaled.DeviceUUID)
		assert.Equal(t, xidErr.Xid, unmarshaled.Xid)
		assert.Equal(t, xidErr.SuggestedActionsByGPUd.RepairActions, unmarshaled.SuggestedActionsByGPUd.RepairActions)
	})

	t.Run("minimal fields", func(t *testing.T) {
		xidErr := xidErrorEventDetail{
			Time:       testTime,
			DataSource: "test-source",
			DeviceUUID: "test-uuid",
			Xid:        123,
		}

		jsonBytes, err := json.Marshal(xidErr)
		assert.NoError(t, err)
		assert.NotNil(t, jsonBytes)

		var unmarshaled xidErrorEventDetail
		err = json.Unmarshal(jsonBytes, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, xidErr.Time.UTC(), unmarshaled.Time.UTC())
		assert.Equal(t, xidErr.DataSource, unmarshaled.DataSource)
		assert.Equal(t, xidErr.DeviceUUID, unmarshaled.DeviceUUID)
		assert.Equal(t, xidErr.Xid, unmarshaled.Xid)
		assert.Nil(t, unmarshaled.SuggestedActionsByGPUd)
	})
}

func Test_newXIDErrorReasonWithDetail_SubCode(t *testing.T) {
	reason := newXIDErrorReasonWithDetail(145, 0, "RLW_CTRL", "PCI:0000:04:00", nil)

	assert.Contains(t, reason, "145/RLW_CTRL")
	assert.Contains(t, reason, "NVLINK: RLW Error")
	assert.Contains(t, reason, "PCI:0000:04:00")
}

// Test_MatchToEventMessageFlowDistinguishesSubCodes verifies that different NVLink subcode names
// (like RLW_CTRL vs RLW_REMAP) produce distinguishable event messages when processed end-to-end.
//
// This test ensures error readability by validating that:
// 1. The parsed subcode name from kmsg is preserved through the event storage flow
// 2. The final message uses the actual parsed subcode (e.g., "RLW_REMAP") rather than
//    a generic catalog fallback (e.g., "RLW_CTRL" for all subcode-0 entries)
// 3. Users can differentiate between different NVLink failure sub-components
//
// Without this behavior, errors like "XID 145, RLW_CTRL" and "XID 145, RLW_REMAP" would
// both display as "XID 145 (NVLINK: RLW Error)" making troubleshooting impossible.
func Test_MatchToEventMessageFlowDistinguishesSubCodes(t *testing.T) {
	testCases := []struct {
		name             string
		kmsgLine         string
		expectedSubCode  string
		expectedNotMatch string // ensure it does NOT contain a different subcode
	}{
		{
			name:             "RLW_CTRL should appear in message",
			kmsgLine:         "NVRM: Xid (PCI:0000:04:00): 145, RLW_CTRL Nonfatal XC0 i0 Link 00 (0x00000003 0x80000000 0x00000000 0x00000000)",
			expectedSubCode:  "RLW_CTRL",
			expectedNotMatch: "RLW_REMAP",
		},
		{
			name:             "RLW_REMAP should appear in message",
			kmsgLine:         "NVRM: Xid (PCI:0000:04:00): 145, RLW_REMAP Nonfatal XC0 i0 Link 00 (0x00000004 0x00000001 0x00000000 0x00000000)",
			expectedSubCode:  "RLW_REMAP",
			expectedNotMatch: "RLW_CTRL",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Parse the kmsg line
			xidErr := Match(tc.kmsgLine)
			assert.NotNil(t, xidErr, "Match should return non-nil")
			assert.NotNil(t, xidErr.Detail, "Detail should be populated")
			assert.Equal(t, tc.expectedSubCode, xidErr.Detail.SubCodeDescription, "SubCodeDescription from Match")

			// Step 2: Simulate event storage by creating the payload
			xidPayload := xidErrorEventDetail{
				DeviceUUID: xidErr.DeviceUUID,
				Xid:        uint64(xidErr.Xid),
			}
			if xidErr.Detail != nil {
				xidPayload.SubCode = xidErr.Detail.SubCode
				xidPayload.SubCodeDescription = xidErr.Detail.SubCodeDescription
				xidPayload.Description = xidErr.Detail.Description
			}

			// Step 3: Verify the payload has the correct subcode description
			assert.Equal(t, tc.expectedSubCode, xidPayload.SubCodeDescription, "Payload SubCodeDescription")

			// Step 4: Simulate reading back and generating the message
			reason := newXIDErrorReasonWithDetail(int(xidPayload.Xid), xidPayload.SubCode, xidPayload.SubCodeDescription, xidPayload.DeviceUUID, nil)

			// Step 5: Verify the message distinguishes the subcode
			assert.Contains(t, reason, tc.expectedSubCode, "Message should contain subcode name")
			assert.NotContains(t, reason, tc.expectedNotMatch, "Message should not contain wrong subcode name")
			assert.Contains(t, reason, "145/"+tc.expectedSubCode, "Message should format as XID/subcode")
		})
	}
}
