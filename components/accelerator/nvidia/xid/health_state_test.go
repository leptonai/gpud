package xid

import (
	"encoding/json"
	"fmt"
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
		assert.Equal(t, "XID 123 SPI_PMU_RPC_WRITE_FAIL detected on GPU PCI:0000:9b:00 UUID:GPU-b850f46d-d5ea-c752-ddf3-c4453e44d3f7", state.Reason)
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
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
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
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
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
	reason := newXIDErrorReasonWithDetail(145, 0, "RLW_CTRL", "", "PCI:0000:04:00", 0, nil)

	assert.Equal(t, "XID 145.0 (err status 0x00000000) NVLINK_RLW_ERROR detected on GPU PCI:0000:04:00", reason)
}

// Test_MatchToEventMessageFlowFormatsMnemonic verifies that the event message uses the catalog
// mnemonic and omits subcodes when none are present in the parsed log.
func Test_MatchToEventMessageFlowFormatsMnemonic(t *testing.T) {
	testCases := []struct {
		name            string
		kmsgLine        string
		expectedMessage string
	}{
		{
			name:            "RLW_CTRL message",
			kmsgLine:        "NVRM: Xid (PCI:0000:04:00): 145, RLW_CTRL Nonfatal XC0 i0 Link 00 (0x00000003 0x80000000 0x00000000 0x00000000)",
			expectedMessage: "XID 145.0 (err status 0x00000000) NVLINK_RLW_ERROR detected on GPU PCI:0000:04:00",
		},
		{
			name:            "RLW_REMAP message",
			kmsgLine:        "NVRM: Xid (PCI:0000:04:00): 145, RLW_REMAP Nonfatal XC0 i0 Link 00 (0x00000004 0x00000001 0x00000000 0x00000000)",
			expectedMessage: "XID 145.0 (err status 0x00000000) NVLINK_RLW_ERROR detected on GPU PCI:0000:04:00",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			xidErr := Match(tc.kmsgLine)
			require.NotNil(t, xidErr, "Match should return non-nil")
			require.NotNil(t, xidErr.Detail, "Detail should be populated")

			xidPayload := xidErrorEventDetail{
				DeviceUUID: xidErr.DeviceUUID,
				Xid:        uint64(xidErr.Xid),
			}
			if xidErr.Detail != nil {
				xidPayload.SubCode = xidErr.Detail.SubCode
				xidPayload.SubCodeDescription = xidErr.Detail.SubCodeDescription
				xidPayload.InvestigatoryHint = xidErr.Detail.InvestigatoryHint
				xidPayload.Description = xidErr.Detail.Description
			}

			reason := newXIDErrorReasonWithDetail(int(xidPayload.Xid), xidPayload.SubCode, xidPayload.SubCodeDescription, xidPayload.InvestigatoryHint, xidPayload.DeviceUUID, xidPayload.ErrorStatus, nil)

			assert.Equal(t, tc.expectedMessage, reason)
			assert.Contains(t, reason, "145.0", "dot subcode should be present for NVLink")
		})
	}
}

// Test_SubCodeDifferentiatesSameUnit verifies that errors with the same Unit but
// different decoded sub-codes produce distinct user-facing messages.
func Test_SubCodeDifferentiatesSameUnit(t *testing.T) {
	testCases := []struct {
		name         string
		kmsgLine     string
		subCodeValue int
	}{
		{
			name:         "NETIR_LINK_EVT with peer device subcode",
			kmsgLine:     "NVRM: Xid (PCI:0000:00:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 00 (0x025001c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			subCodeValue: 37,
		},
		{
			name:         "NETIR_LINK_EVT with software subcode",
			kmsgLine:     "NVRM: Xid (PCI:0000:00:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 00 (0x026001c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			subCodeValue: 38,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Parse the kmsg line
			xidErr := Match(tc.kmsgLine)
			require.NotNil(t, xidErr, "Match should return non-nil")
			require.NotNil(t, xidErr.Detail, "Detail should be populated")

			// Step 2: Verify the InvestigatoryHint is set correctly
			assert.NotEmpty(t, xidErr.Detail.InvestigatoryHint, "InvestigatoryHint should be set")

			// Step 3: Create the event detail payload (simulating component.go)
			xidPayload := xidErrorEventDetail{
				DeviceUUID:         xidErr.DeviceUUID,
				Xid:                uint64(xidErr.Xid),
				SubCode:            xidErr.Detail.SubCode,
				SubCodeDescription: xidErr.Detail.SubCodeDescription,
				InvestigatoryHint:  xidErr.Detail.InvestigatoryHint,
				Description:        xidErr.Detail.Description,
			}

			// Step 4: Generate the user-facing reason message
			reason := newXIDErrorReasonWithDetail(int(xidPayload.Xid), xidPayload.SubCode, xidPayload.SubCodeDescription, xidPayload.InvestigatoryHint, xidPayload.DeviceUUID, xidPayload.ErrorStatus, nil)

			assert.Contains(t, reason, "NVLINK_NETIR_ERROR", "Message should use mnemonic")
			assert.Contains(t, reason, "149.", "Message should include dot sub-code when present")
			assert.Contains(t, reason, "149."+fmt.Sprintf("%d", tc.subCodeValue))
		})
	}
}

// Test_InvestigatoryHintFiltering verifies that IGNORE and CONTACT_SUPPORT
// Investigatory values are filtered out, while other values are passed through.
func Test_InvestigatoryHintFiltering(t *testing.T) {
	testCases := []struct {
		name        string
		kmsgLine    string
		expectHint  bool
		expectedVal string // empty means expect no hint
	}{
		{
			name:        "INVESTIGATE_PEER_DEVICE passes through",
			kmsgLine:    "NVRM: Xid (PCI:0000:00:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 00 (0x025001c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectHint:  true,
			expectedVal: "INVESTIGATE_PEER_DEVICE",
		},
		{
			name:        "INVESTIGATE_SW/USER passes through",
			kmsgLine:    "NVRM: Xid (PCI:0000:00:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 00 (0x026001c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectHint:  true,
			expectedVal: "INVESTIGATE_SW/USER",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			xidErr := Match(tc.kmsgLine)
			require.NotNil(t, xidErr, "Match should return non-nil")
			require.NotNil(t, xidErr.Detail, "Detail should be populated")

			if tc.expectHint {
				assert.Equal(t, tc.expectedVal, xidErr.Detail.InvestigatoryHint, "InvestigatoryHint should match expected value")
			} else {
				assert.Empty(t, xidErr.Detail.InvestigatoryHint, "InvestigatoryHint should be empty for filtered values")
			}
		})
	}
}

// Test_NewXIDErrorReasonWithDetail_Format verifies that the new user-facing format uses
// numeric subcodes (when present) and catalog mnemonics.
func Test_NewXIDErrorReasonWithDetail_Format(t *testing.T) {
	t.Run("with subcode", func(t *testing.T) {
		reason := newXIDErrorReasonWithDetail(149, 37, "NETIR_LINK_EVT", "INVESTIGATE_PEER_DEVICE", "PCI:0000:00:00", 0, nil)
		assert.Equal(t, "XID 149.37 (err status 0x00000000) NVLINK_NETIR_ERROR detected on GPU PCI:0000:00:00", reason)
	})

	t.Run("without subcode", func(t *testing.T) {
		reason := newXIDErrorReasonWithDetail(149, 0, "NETIR_LINK_EVT", "", "PCI:0000:00:00", 0, nil)
		assert.Equal(t, "XID 149.0 (err status 0x00000000) NVLINK_NETIR_ERROR detected on GPU PCI:0000:00:00", reason)
	})

	t.Run("different subcodes produce different messages", func(t *testing.T) {
		reasonPeer := newXIDErrorReasonWithDetail(149, 37, "NETIR_LINK_EVT", "INVESTIGATE_PEER_DEVICE", "PCI:0000:00:00", 0, nil)
		reasonSoftware := newXIDErrorReasonWithDetail(149, 38, "NETIR_LINK_EVT", "INVESTIGATE_SW/USER", "PCI:0000:00:00", 0, nil)

		assert.NotEqual(t, reasonPeer, reasonSoftware)
		assert.Contains(t, reasonPeer, "149.37")
		assert.Contains(t, reasonSoftware, "149.38")
	})
}

// Test_StatusAwareMessages covers the exact lines requested by the user and ensures severity/message are correct.
func Test_StatusAwareMessages(t *testing.T) {
	cases := []struct {
		name          string
		line          string
		expectedMsg   string
		expectedEvent apiv1.EventType
		expectedSub   int
	}{
		{
			name:          "SAW_MVB nonfatal should stay warning",
			line:          "NVRM: Xid (PCI:0000:01:00): 144, SAW_MVB Nonfatal XC0 i0 Link 0 (0x00000001 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedMsg:   "XID 144.0 (err status 0x00000008) NVLINK_SAW_ERROR detected on GPU PCI:0000:01:00",
			expectedEvent: apiv1.EventTypeWarning,
			expectedSub:   0,
		},
		{
			name:          "NETIR_LINK_EVT subcode 38",
			line:          "NVRM: Xid (PCI:0000:00:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 00 (0x026001c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedMsg:   "XID 149.38 (err status 0x00000000) NVLINK_NETIR_ERROR detected on GPU PCI:0000:00:00",
			expectedEvent: apiv1.EventTypeFatal,
			expectedSub:   38,
		},
		{
			name:          "NETIR_LINK_EVT subcode 37",
			line:          "NVRM: Xid (PCI:0000:00:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 00 (0x025001c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedMsg:   "XID 149.37 (err status 0x00000000) NVLINK_NETIR_ERROR detected on GPU PCI:0000:00:00",
			expectedEvent: apiv1.EventTypeFatal,
			expectedSub:   37,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			xidErr := Match(tc.line)
			require.NotNil(t, xidErr)
			require.NotNil(t, xidErr.Detail)

			reason := newXIDErrorReasonWithDetail(xidErr.Xid, xidErr.Detail.SubCode, xidErr.Detail.SubCodeDescription, xidErr.Detail.InvestigatoryHint, xidErr.DeviceUUID, xidErr.Detail.ErrorStatus, nil)

			assert.Equal(t, tc.expectedMsg, reason)
			assert.Equal(t, tc.expectedEvent, xidErr.Detail.EventType)
			assert.Equal(t, tc.expectedSub, xidErr.Detail.SubCode)
		})
	}
}
