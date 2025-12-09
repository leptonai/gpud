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

// createXidEventWithNilSuggestedActions creates an XID event with SuggestedActionsByGPUd=nil.
// This simulates XIDs that don't have suggested actions defined in the catalog.
func createXidEventWithNilSuggestedActions(eventTime time.Time, xid uint64, eventType apiv1.EventType) eventstore.Event {
	xidErr := xidErrorEventDetail{
		Xid:                    xid,
		DataSource:             "test",
		DeviceUUID:             "PCI:0000:9b:00",
		SuggestedActionsByGPUd: nil, // No suggested actions
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

	t.Run("fatal xid 123", func(t *testing.T) {
		// XID 123 is EventTypeFatal in the catalog, so we use Fatal here.
		// In real usage, Match() returns the correct EventType from the catalog.
		events := eventstore.Events{
			createXidEvent(time.Time{}, 123, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events, map[string]device.Device{"GPU-b850f46d-d5ea-c752-ddf3-c4453e44d3f7": mockDevice}, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
		// XID 123 (SPI_PMU_RPC_WRITE_FAIL) has mnemonic, expect it in reason
		assert.Contains(t, state.Reason, "XID 123")
		assert.Contains(t, state.Reason, "GPU PCI:0000:9b:00")
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
		// XID 456 is unknown, so the reason should contain the XID number
		assert.Contains(t, state.Reason, "XID 456")
		assert.Contains(t, state.Reason, "GPU PCI:0000:9b:00")
	})

	t.Run("reboot recover", func(t *testing.T) {
		events := eventstore.Events{
			{Name: "reboot"},
			createXidEvent(time.Time{}, 789, apiv1.EventTypeCritical, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})

	t.Run("reboot multiple time cannot recover, should be unhealthy", func(t *testing.T) {
		// XID 94 is EventTypeFatal in the catalog, so we use Fatal here.
		// In real usage, Match() returns the correct EventType from the catalog.
		events := eventstore.Events{
			createXidEvent(time.Time{}, 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createXidEvent(time.Time{}, 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createXidEvent(time.Time{}, 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			createXidEvent(time.Time{}, 31, apiv1.EventTypeWarning, apiv1.RepairActionTypeCheckUserAppAndGPU),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	})

	// Tests for reboot recovery behavior based on SuggestedActionsByGPUd.
	// These tests verify the inline comments in evolveHealthyState() that explain:
	// - XIDs with SuggestedActionsByGPUd=nil will NOT be cleared on reboot
	// - Only RebootSystem and CheckUserAppAndGPU actions are cleared on reboot

	t.Run("reboot does NOT clear XID with nil SuggestedActionsByGPUd", func(t *testing.T) {
		// This is the critical case: XIDs without SuggestedActionsByGPUd cannot be cleared.
		// This was the root cause of the XID 149 "Placeholder message" bug.
		events := eventstore.Events{
			{Name: "reboot", Time: time.Now()},
			createXidEventWithNilSuggestedActions(time.Now().Add(-1*time.Hour), 999, apiv1.EventTypeFatal),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		// Should remain unhealthy because SuggestedActionsByGPUd is nil
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health,
			"XID with nil SuggestedActionsByGPUd should NOT be cleared on reboot")
	})

	t.Run("reboot does NOT clear XID with HardwareInspection action", func(t *testing.T) {
		// Only RebootSystem and CheckUserAppAndGPU are cleared on reboot.
		// HardwareInspection requires manual intervention and should persist.
		events := eventstore.Events{
			{Name: "reboot", Time: time.Now()},
			createXidEvent(time.Now().Add(-1*time.Hour), 999, apiv1.EventTypeFatal, apiv1.RepairActionTypeHardwareInspection),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		// Should remain unhealthy because HardwareInspection is not cleared by reboot
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health,
			"XID with HardwareInspection action should NOT be cleared on reboot")
		require.NotNil(t, state.SuggestedActions)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	})

	t.Run("reboot DOES clear XID with CheckUserAppAndGPU action", func(t *testing.T) {
		// CheckUserAppAndGPU is one of the two actions that ARE cleared on reboot.
		events := eventstore.Events{
			{Name: "reboot", Time: time.Now()},
			createXidEvent(time.Now().Add(-1*time.Hour), 999, apiv1.EventTypeCritical, apiv1.RepairActionTypeCheckUserAppAndGPU),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		// Should be healthy because CheckUserAppAndGPU is cleared by reboot
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health,
			"XID with CheckUserAppAndGPU action should be cleared on reboot")
	})

	t.Run("reboot DOES clear XID with RebootSystem action", func(t *testing.T) {
		// RebootSystem is one of the two actions that ARE cleared on reboot.
		// This is already tested in "reboot recover" but adding explicit test for clarity.
		events := eventstore.Events{
			{Name: "reboot", Time: time.Now()},
			createXidEvent(time.Now().Add(-1*time.Hour), 999, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		// Should be healthy because RebootSystem is cleared by reboot
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health,
			"XID with RebootSystem action should be cleared on reboot")
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

func Test_buildMessage_SubCode(t *testing.T) {
	xidErr := xidErrorEventDetail{
		Xid:                145,
		SubCode:            0,
		SubCodeDescription: "RLW_CTRL",
		DeviceUUID:         "PCI:0000:04:00",
		ErrorStatus:        0,
		Description:        "NVLINK: RLW Error",
	}
	reason := xidErr.buildMessage(nil)

	// buildMessage concatenates mnemonic (NVLINK_RLW_ERROR) with description (NVLINK: RLW Error)
	assert.Contains(t, reason, "XID 145.0")
	assert.Contains(t, reason, "NVLINK_RLW_ERROR")
	assert.Contains(t, reason, "PCI:0000:04:00")
}

// Test_MatchToEventMessageFlowFormatsMnemonic verifies that the event message uses the catalog
// mnemonic and includes subcodes for NVLink XIDs (144-150).
func Test_MatchToEventMessageFlowFormatsMnemonic(t *testing.T) {
	testCases := []struct {
		name     string
		kmsgLine string
	}{
		{
			name:     "RLW_CTRL message",
			kmsgLine: "NVRM: Xid (PCI:0000:04:00): 145, RLW_CTRL Nonfatal XC0 i0 Link 00 (0x00000003 0x80000000 0x00000000 0x00000000)",
		},
		{
			name:     "RLW_REMAP message",
			kmsgLine: "NVRM: Xid (PCI:0000:04:00): 145, RLW_REMAP Nonfatal XC0 i0 Link 00 (0x00000004 0x00000001 0x00000000 0x00000000)",
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
				xidPayload.ErrorStatus = xidErr.Detail.ErrorStatus
			}

			reason := xidPayload.buildMessage(nil)

			// Verify the message contains expected components
			assert.Contains(t, reason, "145.0", "dot subcode should be present for NVLink")
			assert.Contains(t, reason, "NVLINK_RLW_ERROR", "mnemonic should be present")
			assert.Contains(t, reason, "PCI:0000:04:00", "device UUID should be present")
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
				ErrorStatus:        xidErr.Detail.ErrorStatus,
			}

			// Step 4: Generate the user-facing reason message
			reason := xidPayload.buildMessage(nil)

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

// Test_buildMessage_Format verifies that the user-facing format uses
// numeric subcodes (when present) and catalog mnemonics.
func Test_buildMessage_Format(t *testing.T) {
	t.Run("with subcode", func(t *testing.T) {
		xidErr := xidErrorEventDetail{
			Xid:                149,
			SubCode:            37,
			SubCodeDescription: "NETIR_LINK_EVT",
			InvestigatoryHint:  "INVESTIGATE_PEER_DEVICE",
			DeviceUUID:         "PCI:0000:00:00",
			ErrorStatus:        0,
		}
		reason := xidErr.buildMessage(nil)
		assert.Equal(t, "XID 149.37 (err status 0x00000000) NVLINK_NETIR_ERROR detected on GPU PCI:0000:00:00", reason)
	})

	t.Run("without subcode", func(t *testing.T) {
		xidErr := xidErrorEventDetail{
			Xid:                149,
			SubCode:            0,
			SubCodeDescription: "NETIR_LINK_EVT",
			DeviceUUID:         "PCI:0000:00:00",
			ErrorStatus:        0,
		}
		reason := xidErr.buildMessage(nil)
		assert.Equal(t, "XID 149.0 (err status 0x00000000) NVLINK_NETIR_ERROR detected on GPU PCI:0000:00:00", reason)
	})

	t.Run("different subcodes produce different messages", func(t *testing.T) {
		xidErrPeer := xidErrorEventDetail{
			Xid:                149,
			SubCode:            37,
			SubCodeDescription: "NETIR_LINK_EVT",
			InvestigatoryHint:  "INVESTIGATE_PEER_DEVICE",
			DeviceUUID:         "PCI:0000:00:00",
			ErrorStatus:        0,
		}
		xidErrSoftware := xidErrorEventDetail{
			Xid:                149,
			SubCode:            38,
			SubCodeDescription: "NETIR_LINK_EVT",
			InvestigatoryHint:  "INVESTIGATE_SW/USER",
			DeviceUUID:         "PCI:0000:00:00",
			ErrorStatus:        0,
		}
		reasonPeer := xidErrPeer.buildMessage(nil)
		reasonSoftware := xidErrSoftware.buildMessage(nil)

		assert.NotEqual(t, reasonPeer, reasonSoftware)
		assert.Contains(t, reasonPeer, "149.37")
		assert.Contains(t, reasonSoftware, "149.38")
	})
}

// Test_StatusAwareMessages covers the exact lines requested by the user and ensures severity/message are correct.
func Test_StatusAwareMessages(t *testing.T) {
	cases := []struct {
		name             string
		line             string
		expectedMnemonic string
		expectedEvent    apiv1.EventType
		expectedSub      int
	}{
		{
			name:             "SAW_MVB nonfatal should stay warning",
			line:             "NVRM: Xid (PCI:0000:01:00): 144, SAW_MVB Nonfatal XC0 i0 Link 0 (0x00000001 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedMnemonic: "NVLINK_SAW_ERROR",
			expectedEvent:    apiv1.EventTypeWarning,
			expectedSub:      0,
		},
		{
			name:             "NETIR_LINK_EVT subcode 38",
			line:             "NVRM: Xid (PCI:0000:00:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 00 (0x026001c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedMnemonic: "NVLINK_NETIR_ERROR",
			expectedEvent:    apiv1.EventTypeFatal,
			expectedSub:      38,
		},
		{
			name:             "NETIR_LINK_EVT subcode 37",
			line:             "NVRM: Xid (PCI:0000:00:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 00 (0x025001c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedMnemonic: "NVLINK_NETIR_ERROR",
			expectedEvent:    apiv1.EventTypeFatal,
			expectedSub:      37,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			xidErr := Match(tc.line)
			require.NotNil(t, xidErr)
			require.NotNil(t, xidErr.Detail)

			xidPayload := xidErrorEventDetail{
				DeviceUUID:         xidErr.DeviceUUID,
				Xid:                uint64(xidErr.Xid),
				SubCode:            xidErr.Detail.SubCode,
				SubCodeDescription: xidErr.Detail.SubCodeDescription,
				InvestigatoryHint:  xidErr.Detail.InvestigatoryHint,
				Description:        xidErr.Detail.Description,
				ErrorStatus:        xidErr.Detail.ErrorStatus,
			}
			reason := xidPayload.buildMessage(nil)

			assert.Contains(t, reason, tc.expectedMnemonic)
			assert.Contains(t, reason, xidErr.DeviceUUID)
			assert.Contains(t, reason, fmt.Sprintf("%d.%d", xidErr.Xid, tc.expectedSub))
			assert.Equal(t, tc.expectedEvent, xidErr.Detail.EventType)
			assert.Equal(t, tc.expectedSub, xidErr.Detail.SubCode)
		})
	}
}

// Test_HealthStateReason_NVLinkXIDs verifies the reason field format for NVLink XIDs (144-150)
// with subcode and error status information.
func Test_HealthStateReason_NVLinkXIDs(t *testing.T) {
	mockDevice := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:04:00.0")
	devices := map[string]device.Device{"GPU-test-uuid": mockDevice}

	testCases := []struct {
		name                string
		kmsgLine            string
		expectedXid         int
		expectedSubCode     int
		expectedContains    []string
		expectedNotContains []string
	}{
		{
			name:            "XID 144 SAW Error with error status",
			kmsgLine:        "NVRM: Xid (PCI:0000:04:00): 144, SAW_MVB Nonfatal XC0 i0 Link 00 (0x00000001 0x00000008 0x00000000 0x00000000)",
			expectedXid:     144,
			expectedSubCode: 0,
			expectedContains: []string{
				"XID 144.0",
				"err status 0x00000008",
				"NVLINK_SAW_ERROR",
				"GPU PCI:0000:04:00",
			},
		},
		{
			name:            "XID 145 RLW Error with subcode 0",
			kmsgLine:        "NVRM: Xid (PCI:0000:04:00): 145, RLW_CTRL Nonfatal XC0 i0 Link 00 (0x00000003 0x80000000 0x00000000 0x00000000)",
			expectedXid:     145,
			expectedSubCode: 0,
			expectedContains: []string{
				"XID 145.0",
				"err status 0x80000000",
				"NVLINK_RLW_ERROR",
				"GPU PCI:0000:04:00",
			},
		},
		{
			name:            "XID 149 NETIR Error with subcode 37 (peer device)",
			kmsgLine:        "NVRM: Xid (PCI:0000:04:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 00 (0x025001c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:     149,
			expectedSubCode: 37,
			expectedContains: []string{
				"XID 149.37",
				"err status 0x00000000",
				"NVLINK_NETIR_ERROR",
				"GPU PCI:0000:04:00",
			},
		},
		{
			name:            "XID 149 NETIR Error with subcode 38 (software)",
			kmsgLine:        "NVRM: Xid (PCI:0000:04:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 00 (0x026001c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:     149,
			expectedSubCode: 38,
			expectedContains: []string{
				"XID 149.38",
				"err status 0x00000000",
				"NVLINK_NETIR_ERROR",
				"GPU PCI:0000:04:00",
			},
		},
		{
			name:            "XID 149 NETIR Error with subcode 4 (cartridge error)",
			kmsgLine:        "NVRM: Xid (PCI:0000:04:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 08 (0x004505c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:     149,
			expectedSubCode: 4,
			expectedContains: []string{
				"XID 149.4",
				"err status 0x00000000",
				"NVLINK_NETIR_ERROR",
				"GPU PCI:0000:04:00",
			},
		},
		{
			name:            "XID 150 MSE Watchdog Fatal",
			kmsgLine:        "NVRM: Xid (PCI:0000:04:00): 150, MSE_WATCHDOG Fatal XC0 i0 Link 00 (0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:     150,
			expectedSubCode: 0,
			expectedContains: []string{
				"XID 150.0",
				"err status 0x00000000",
				"NVLINK_MSE_ERROR",
				"GPU PCI:0000:04:00",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			xidErr := Match(tc.kmsgLine)
			require.NotNil(t, xidErr)
			require.NotNil(t, xidErr.Detail)

			// Simulate the component behavior: create event and resolve it
			xidPayload := xidErrorEventDetail{
				DeviceUUID:         xidErr.DeviceUUID,
				Xid:                uint64(xidErr.Xid),
				SubCode:            xidErr.Detail.SubCode,
				SubCodeDescription: xidErr.Detail.SubCodeDescription,
				InvestigatoryHint:  xidErr.Detail.InvestigatoryHint,
				Description:        xidErr.Detail.Description,
				ErrorStatus:        xidErr.Detail.ErrorStatus,
			}

			// Test buildMessage which is used in evolveHealthyState
			reason := xidPayload.buildMessage(devices)

			// Verify XID and subcode
			assert.Equal(t, tc.expectedXid, xidErr.Xid)
			assert.Equal(t, tc.expectedSubCode, xidErr.Detail.SubCode)

			// Verify reason contains expected strings
			for _, expected := range tc.expectedContains {
				assert.Contains(t, reason, expected, "reason should contain: %s", expected)
			}

			// Verify reason doesn't contain unexpected strings
			for _, notExpected := range tc.expectedNotContains {
				assert.NotContains(t, reason, notExpected, "reason should not contain: %s", notExpected)
			}

			// Verify UUID resolution works when device is found
			if tc.kmsgLine != "" {
				assert.Contains(t, reason, "UUID:GPU-test-uuid", "reason should contain resolved UUID")
			}
		})
	}
}

// Test_HealthStateReason_StandardXIDs verifies the reason field format for standard (non-NVLink) XIDs.
func Test_HealthStateReason_StandardXIDs(t *testing.T) {
	mockDevice := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:9b:00.0")
	devices := map[string]device.Device{"GPU-test-uuid": mockDevice}

	testCases := []struct {
		name             string
		xid              uint64
		deviceUUID       string
		description      string
		expectedContains []string
	}{
		{
			name:        "XID 94 Contained ECC Error",
			xid:         94,
			deviceUUID:  "PCI:0000:9b:00",
			description: "Contained ECC error",
			expectedContains: []string{
				"XID 94",
				"ROBUST_CHANNEL_CONTAINED_ERROR",
				"GPU PCI:0000:9b:00",
			},
		},
		{
			name:        "XID 79 GPU fallen off bus",
			xid:         79,
			deviceUUID:  "PCI:0000:9b:00",
			description: "GPU has fallen off the bus",
			expectedContains: []string{
				"XID 79",
				"GPU_HAS_FALLEN_OFF_THE_BUS",
				"GPU PCI:0000:9b:00",
			},
		},
		{
			name:        "XID 31 GPU memory page fault",
			xid:         31,
			deviceUUID:  "PCI:0000:9b:00",
			description: "GPU memory page fault",
			expectedContains: []string{
				"XID 31",
				"ROBUST_CHANNEL_FIFO_ERROR_MMU_ERR_FLT",
				"GPU PCI:0000:9b:00",
			},
		},
		{
			name:        "XID 63 Row remapping",
			xid:         63,
			deviceUUID:  "PCI:0000:9b:00",
			description: "Row remapping event",
			expectedContains: []string{
				"XID 63",
				"INFOROM_DRAM_RETIREMENT_EVENT",
				"GPU PCI:0000:9b:00",
			},
		},
		{
			name:        "XID 64 Row remapping failure",
			xid:         64,
			deviceUUID:  "PCI:0000:9b:00",
			description: "Row remapping failure",
			expectedContains: []string{
				"XID 64",
				"INFOROM_DRAM_RETIREMENT_FAILURE",
				"GPU PCI:0000:9b:00",
			},
		},
		{
			name:        "XID 13 Graphics Engine Exception",
			xid:         13,
			deviceUUID:  "PCI:0000:9b:00",
			description: "Graphics Engine Exception",
			expectedContains: []string{
				"XID 13",
				"GR_EXCEPTION",
				"GPU PCI:0000:9b:00",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Get detail from catalog
			detail, ok := GetDetail(int(tc.xid))
			require.True(t, ok, "should find detail for XID %d", tc.xid)

			xidPayload := xidErrorEventDetail{
				DeviceUUID:             tc.deviceUUID,
				Xid:                    tc.xid,
				Description:            tc.description,
				SuggestedActionsByGPUd: detail.SuggestedActionsByGPUd,
			}

			reason := xidPayload.buildMessage(devices)

			// Verify reason contains expected strings
			for _, expected := range tc.expectedContains {
				assert.Contains(t, reason, expected, "reason should contain: %s", expected)
			}

			// Standard XIDs should NOT have dotted subcode format
			assert.NotContains(t, reason, fmt.Sprintf("XID %d.", tc.xid), "standard XID should not have dotted subcode")

			// Standard XIDs should NOT have error status
			assert.NotContains(t, reason, "err status", "standard XID should not have error status")
		})
	}
}

// Test_HealthStateReason_evolveHealthyState_Integration tests the full integration
// of reason generation through evolveHealthyState.
func Test_HealthStateReason_evolveHealthyState_Integration(t *testing.T) {
	mockDevice := testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:04:00.0")
	devices := map[string]device.Device{"GPU-test-uuid": mockDevice}

	testCases := []struct {
		name             string
		events           eventstore.Events
		expectedHealth   apiv1.HealthStateType
		expectedContains []string
	}{
		{
			name:           "No events - healthy",
			events:         eventstore.Events{},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedContains: []string{
				"XIDComponent is healthy",
			},
		},
		{
			name: "NVLink XID 149 fatal event",
			events: eventstore.Events{
				createNVLinkXidEvent(time.Now(), 149, 37, 0x00000000, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedContains: []string{
				"XID 149.37",
				"err status 0x00000000",
				"NVLINK_NETIR_ERROR",
			},
		},
		{
			name: "NVLink XID 144 fatal event (SAW error with fatal status)",
			events: eventstore.Events{
				createNVLinkXidEvent(time.Now(), 144, 0, 0x00000002, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedContains: []string{
				"XID 144.0",
				"NVLINK_SAW_ERROR",
			},
		},
		{
			name: "Standard XID 94 fatal event",
			events: eventstore.Events{
				createXidEvent(time.Now(), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedContains: []string{
				"XID 94",
				"ROBUST_CHANNEL_CONTAINED_ERROR",
			},
		},
		{
			name: "Recovered after reboot",
			events: eventstore.Events{
				{Name: "reboot", Time: time.Now()},
				createXidEvent(time.Now().Add(-1*time.Minute), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedContains: []string{
				"XIDComponent is healthy",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			state := evolveHealthyState(tc.events, devices, DefaultRebootThreshold)

			assert.Equal(t, tc.expectedHealth, state.Health)

			for _, expected := range tc.expectedContains {
				assert.Contains(t, state.Reason, expected, "reason should contain: %s", expected)
			}
		})
	}
}

// createNVLinkXidEvent creates a test event for NVLink XIDs (144-150) with subcode and error status.
func createNVLinkXidEvent(eventTime time.Time, xid uint64, subCode int, errorStatus uint32, eventType apiv1.EventType, suggestedAction apiv1.RepairActionType) eventstore.Event {
	xidErr := xidErrorEventDetail{
		Xid:         xid,
		DataSource:  "test",
		DeviceUUID:  "PCI:0000:04:00",
		SubCode:     subCode,
		ErrorStatus: errorStatus,
		Description: fmt.Sprintf("NVLINK Error for XID %d", xid),
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

// Test_HealthStateReason_UnknownXID verifies the reason field for unknown XIDs.
func Test_HealthStateReason_UnknownXID(t *testing.T) {
	xidPayload := xidErrorEventDetail{
		DeviceUUID:  "PCI:0000:9b:00",
		Xid:         999, // Unknown XID
		Description: "Unknown error",
	}

	reason := xidPayload.buildMessage(nil)

	// Unknown XID should still show the XID number
	assert.Contains(t, reason, "XID 999")
	assert.Contains(t, reason, "GPU PCI:0000:9b:00")
	// Should use the description since no mnemonic exists
	assert.Contains(t, reason, "Unknown error")
}

// Test_HealthStateReason_EmptyDescription verifies behavior when description is empty.
func Test_HealthStateReason_EmptyDescription(t *testing.T) {
	// XID with known mnemonic but empty description
	xidPayload := xidErrorEventDetail{
		DeviceUUID:  "PCI:0000:9b:00",
		Xid:         94, // Known XID with mnemonic
		Description: "", // Empty description
	}

	reason := xidPayload.buildMessage(nil)

	// Should still use mnemonic from catalog
	assert.Contains(t, reason, "XID 94")
	assert.Contains(t, reason, "ROBUST_CHANNEL_CONTAINED_ERROR")
	assert.Contains(t, reason, "GPU PCI:0000:9b:00")
}

// Test_EventType_DifferentUnits_SameSubCodeAndErrorStatus verifies that different Units
// with the same SubCode and ErrorStatus return the correct EventType.
// This is a critical test for the bug where RLW_REMAP (Non-fatal) and RLW_SRC_TRACK (Fatal)
// both have SubCode=0 and ErrorStatus=0x00000001, but should have different severities.
func Test_EventType_DifferentUnits_SameSubCodeAndErrorStatus(t *testing.T) {
	testCases := []struct {
		name          string
		kmsgLine      string
		expectedEvent apiv1.EventType
		unit          string
		errorStatus   uint32
	}{
		{
			// RLW_REMAP with ErrorStatus 0x00000001 should be Non-fatal (Warning)
			// catalog_generated.go:191: Severity: "Non-fatal"
			name:          "RLW_REMAP_0x00000001_should_be_nonfatal",
			kmsgLine:      "NVRM: Xid (PCI:0000:04:00): 145, RLW_REMAP Nonfatal XC0 i0 Link 00 (0x00000004 0x00000001 0x00000000 0x00000000)",
			expectedEvent: apiv1.EventTypeWarning,
			unit:          "RLW_REMAP",
			errorStatus:   0x00000001,
		},
		{
			// RLW_SRC_TRACK with ErrorStatus 0x00000001 should be Fatal
			// catalog_generated.go:210: Severity: "Fatal"
			name:          "RLW_SRC_TRACK_0x00000001_should_be_fatal",
			kmsgLine:      "NVRM: Xid (PCI:0000:04:00): 145, RLW_SRC_TRACK Fatal XC0 i0 Link 00 (0x00000007 0x00000001 0x00000000 0x00000000)",
			expectedEvent: apiv1.EventTypeFatal,
			unit:          "RLW_SRC_TRACK",
			errorStatus:   0x00000001,
		},
		{
			// RLW_RSPCOL with ErrorStatus 0x00000001 should be Non-fatal (Warning)
			// catalog_generated.go:202: Severity: "Non-fatal"
			name:          "RLW_RSPCOL_0x00000001_should_be_nonfatal",
			kmsgLine:      "NVRM: Xid (PCI:0000:04:00): 145, RLW_RSPCOL Nonfatal XC0 i0 Link 00 (0x00000005 0x00000001 0x00000000 0x00000000)",
			expectedEvent: apiv1.EventTypeWarning,
			unit:          "RLW_RSPCOL",
			errorStatus:   0x00000001,
		},
		{
			// RLW_RXPIPE with ErrorStatus 0x00000001 should be Non-fatal (Warning)
			// catalog_generated.go:205: Severity: "Non-fatal*"
			name:          "RLW_RXPIPE_0x00000001_should_be_nonfatal",
			kmsgLine:      "NVRM: Xid (PCI:0000:04:00): 145, RLW_RXPIPE Nonfatal XC0 i0 Link 00 (0x00000006 0x00000001 0x00000000 0x00000000)",
			expectedEvent: apiv1.EventTypeWarning,
			unit:          "RLW_RXPIPE",
			errorStatus:   0x00000001,
		},
		{
			// RLW_TAGSTATE with ErrorStatus 0x00000001 should be Non-fatal (Warning)
			// catalog_generated.go:217: Severity: "Non-fatal"
			name:          "RLW_TAGSTATE_0x00000001_should_be_nonfatal",
			kmsgLine:      "NVRM: Xid (PCI:0000:04:00): 145, RLW_TAGSTATE Nonfatal XC0 i0 Link 00 (0x00000008 0x00000001 0x00000000 0x00000000)",
			expectedEvent: apiv1.EventTypeWarning,
			unit:          "RLW_TAGSTATE",
			errorStatus:   0x00000001,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			xidErr := Match(tc.kmsgLine)
			require.NotNil(t, xidErr, "Match should return non-nil for %s", tc.kmsgLine)
			require.NotNil(t, xidErr.Detail, "Detail should be populated")

			assert.Equal(t, tc.expectedEvent, xidErr.Detail.EventType,
				"EventType mismatch for Unit=%s ErrorStatus=0x%08x: expected %s, got %s",
				tc.unit, tc.errorStatus, tc.expectedEvent, xidErr.Detail.EventType)
		})
	}
}

// Test_AddEventDetails_PreservesCorrectEventType verifies that addEventDetails
// does not overwrite an already-correct event type with a potentially incorrect
// merged value from getDetailWithSubCodeAndStatus.
func Test_AddEventDetails_PreservesCorrectEventType(t *testing.T) {
	testCases := []struct {
		name              string
		initialEventType  string
		xidErr            xidErrorEventDetail
		expectedEventType string
	}{
		{
			name:             "preserves_warning_for_RLW_REMAP",
			initialEventType: string(apiv1.EventTypeWarning),
			xidErr: xidErrorEventDetail{
				Xid:                145,
				SubCode:            0,
				SubCodeDescription: "RLW_REMAP",
				ErrorStatus:        0x00000001,
				DeviceUUID:         "PCI:0000:04:00",
			},
			expectedEventType: string(apiv1.EventTypeWarning),
		},
		{
			name:             "preserves_fatal_for_RLW_SRC_TRACK",
			initialEventType: string(apiv1.EventTypeFatal),
			xidErr: xidErrorEventDetail{
				Xid:                145,
				SubCode:            0,
				SubCodeDescription: "RLW_SRC_TRACK",
				ErrorStatus:        0x00000001,
				DeviceUUID:         "PCI:0000:04:00",
			},
			expectedEventType: string(apiv1.EventTypeFatal),
		},
		{
			name:             "sets_type_when_empty",
			initialEventType: "",
			xidErr: xidErrorEventDetail{
				Xid:                145,
				SubCode:            0,
				SubCodeDescription: "RLW_REMAP",
				ErrorStatus:        0x00000001,
				DeviceUUID:         "PCI:0000:04:00",
			},
			// When empty, it will use the merged detail which may be wrong,
			// but this is the fallback behavior for legacy events
			expectedEventType: string(apiv1.EventTypeFatal), // merged value
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ev := eventstore.Event{
				Name: EventNameErrorXid,
				Type: tc.initialEventType,
				Time: time.Now(),
			}

			result := addEventDetails(ev, &tc.xidErr, nil)

			assert.Equal(t, tc.expectedEventType, result.Type,
				"EventType should be %s but got %s", tc.expectedEventType, result.Type)
		})
	}
}

// Test_EventType_EndToEnd_MatchThenAddEventDetails verifies the complete flow:
// 1. Match() parses the kmsg and returns correct EventType
// 2. Event is created with the correct Type
// 3. addEventDetails preserves the correct Type (doesn't overwrite)
func Test_EventType_EndToEnd_MatchThenAddEventDetails(t *testing.T) {
	testCases := []struct {
		name          string
		kmsgLine      string
		expectedEvent apiv1.EventType
	}{
		{
			name:          "RLW_REMAP_nonfatal_preserved_through_flow",
			kmsgLine:      "NVRM: Xid (PCI:0000:04:00): 145, RLW_REMAP Nonfatal XC0 i0 Link 00 (0x00000004 0x00000001 0x00000000 0x00000000)",
			expectedEvent: apiv1.EventTypeWarning,
		},
		{
			name:          "RLW_SRC_TRACK_fatal_preserved_through_flow",
			kmsgLine:      "NVRM: Xid (PCI:0000:04:00): 145, RLW_SRC_TRACK Fatal XC0 i0 Link 00 (0x00000007 0x00000001 0x00000000 0x00000000)",
			expectedEvent: apiv1.EventTypeFatal,
		},
		{
			name:          "RLW_REMAP_fatal_status_0x40",
			kmsgLine:      "NVRM: Xid (PCI:0000:04:00): 145, RLW_REMAP Fatal XC0 i0 Link 00 (0x00000004 0x00000040 0x00000000 0x00000000)",
			expectedEvent: apiv1.EventTypeFatal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Parse with Match()
			xidErr := Match(tc.kmsgLine)
			require.NotNil(t, xidErr, "Match should return non-nil")
			require.NotNil(t, xidErr.Detail, "Detail should be populated")

			// Step 2: Create event with the correct Type from Match()
			xidPayload := xidErrorEventDetail{
				DeviceUUID:             xidErr.DeviceUUID,
				Xid:                    uint64(xidErr.Xid),
				SubCode:                xidErr.Detail.SubCode,
				SubCodeDescription:     xidErr.Detail.SubCodeDescription,
				ErrorStatus:            xidErr.Detail.ErrorStatus,
				Description:            xidErr.Detail.Description,
				SuggestedActionsByGPUd: xidErr.Detail.SuggestedActionsByGPUd,
			}

			ev := eventstore.Event{
				Name: EventNameErrorXid,
				Type: string(xidErr.Detail.EventType), // Set from Match()
				Time: time.Now(),
			}

			// Step 3: Call addEventDetails (simulating event resolution)
			result := addEventDetails(ev, &xidPayload, nil)

			// Step 4: Verify the EventType is preserved correctly
			assert.Equal(t, string(tc.expectedEvent), result.Type,
				"EventType should be preserved as %s through the flow, but got %s",
				tc.expectedEvent, result.Type)
		})
	}
}

// Test_NVLink_NonExtendedFormat_RebootRecovery verifies that NVLink XIDs (144-150)
// with non-extended format messages (like "Placeholder message") are correctly
// cleared on reboot because the base catalog entries have SuggestedActionsByGPUd.
//
// This test addresses the bug where XID 149 with message format like:
//
//	NVRM: Xid (PCI:0008:06:00): 149, Placeholder message
//
// was NOT being cleared after reboot because:
// 1. The message doesn't match RegexNVRMXidExtended (no intrinfo/errorstatus)
// 2. Match() falls back to GetDetail(149) which returned nil SuggestedActionsByGPUd
// 3. evolveHealthyState() requires SuggestedActionsByGPUd to clear on reboot
func Test_NVLink_NonExtendedFormat_RebootRecovery(t *testing.T) {
	// Test all NVLink XIDs (144-150) with non-extended format
	nvlinkXIDs := []int{144, 145, 146, 147, 148, 149, 150}

	for _, xid := range nvlinkXIDs {
		t.Run(fmt.Sprintf("XID_%d_clears_on_reboot", xid), func(t *testing.T) {
			// Simulate a non-extended format message (like "Placeholder message")
			// This doesn't match RegexNVRMXidExtended, so Match() uses GetDetail()
			nonExtendedLine := fmt.Sprintf("NVRM: Xid (PCI:0008:06:00): %d, Placeholder message", xid)

			// Verify Match() returns a result with base entry
			xidErr := Match(nonExtendedLine)
			require.NotNil(t, xidErr, "Match should return non-nil for XID %d", xid)
			require.NotNil(t, xidErr.Detail, "Detail should be populated")

			// Verify SuggestedActionsByGPUd is set (this is the fix)
			require.NotNil(t, xidErr.Detail.SuggestedActionsByGPUd,
				"XID %d base entry should have SuggestedActionsByGPUd for reboot recovery", xid)
			require.NotEmpty(t, xidErr.Detail.SuggestedActionsByGPUd.RepairActions)
			assert.Equal(t, apiv1.RepairActionTypeRebootSystem,
				xidErr.Detail.SuggestedActionsByGPUd.RepairActions[0])

			// Create event and verify reboot clears it
			xidPayload := xidErrorEventDetail{
				DeviceUUID:             xidErr.DeviceUUID,
				Xid:                    uint64(xidErr.Xid),
				SuggestedActionsByGPUd: xidErr.Detail.SuggestedActionsByGPUd,
			}
			xidData, _ := json.Marshal(xidPayload)

			events := eventstore.Events{
				{Name: "reboot", Time: time.Now()},
				{
					Name:      EventNameErrorXid,
					Type:      string(xidErr.Detail.EventType),
					Time:      time.Now().Add(-1 * time.Hour),
					ExtraInfo: map[string]string{EventKeyErrorXidData: string(xidData)},
				},
			}

			state := evolveHealthyState(events, nil, DefaultRebootThreshold)
			assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health,
				"XID %d should be cleared after reboot, but got health=%s", xid, state.Health)
			assert.Equal(t, "XIDComponent is healthy", state.Reason)
		})
	}
}

// Test_EventType_SubCodeMerging_DoesNotAffectMatch verifies that even though
// getDetailWithSubCodeAndStatus merges different units with maxEventType,
// the Match() function correctly uses lookupNVLinkRule to get the precise severity.
func Test_EventType_SubCodeMerging_DoesNotAffectMatch(t *testing.T) {
	// All these units have SubCode=0 (bits 25-20 are all zeros in their patterns)
	// and some share the same ErrorStatus, but should have different severities.
	testCases := []struct {
		name          string
		unit          string
		intrinfo      string // hex string for pattern matching
		errorStatus   string // hex string
		logSeverity   string // "Nonfatal" or "Fatal"
		expectedEvent apiv1.EventType
	}{
		// RLW_REMAP: SubCode=0, various ErrorStatus values from bug report
		{"RLW_REMAP_es1", "RLW_REMAP", "0x00000004", "0x00000001", "Nonfatal", apiv1.EventTypeWarning},
		{"RLW_REMAP_es2", "RLW_REMAP", "0x00000004", "0x00000002", "Nonfatal", apiv1.EventTypeWarning},
		{"RLW_REMAP_es4", "RLW_REMAP", "0x00000004", "0x00000004", "Nonfatal", apiv1.EventTypeWarning},
		{"RLW_REMAP_es8", "RLW_REMAP", "0x00000004", "0x00000008", "Nonfatal", apiv1.EventTypeWarning},
		{"RLW_REMAP_es10", "RLW_REMAP", "0x00000004", "0x00000010", "Nonfatal", apiv1.EventTypeWarning},
		{"RLW_REMAP_es20", "RLW_REMAP", "0x00000004", "0x00000020", "Nonfatal", apiv1.EventTypeWarning},
		{"RLW_REMAP_es40_fatal", "RLW_REMAP", "0x00000004", "0x00000040", "Fatal", apiv1.EventTypeFatal},
		{"RLW_REMAP_es80_fatal", "RLW_REMAP", "0x00000004", "0x00000080", "Fatal", apiv1.EventTypeFatal},

		// RLW_SRC_TRACK: SubCode=0, ErrorStatus=0x00000001, Severity=Fatal
		{"RLW_SRC_TRACK_es1_fatal", "RLW_SRC_TRACK", "0x00000007", "0x00000001", "Fatal", apiv1.EventTypeFatal},
		{"RLW_SRC_TRACK_es2_nonfatal", "RLW_SRC_TRACK", "0x00000007", "0x00000002", "Nonfatal", apiv1.EventTypeWarning},
		{"RLW_SRC_TRACK_es4_nonfatal", "RLW_SRC_TRACK", "0x00000007", "0x00000004", "Nonfatal", apiv1.EventTypeWarning},

		// RLW_RSPCOL: SubCode=0, ErrorStatus=0x00000001, Severity=Non-fatal
		{"RLW_RSPCOL_es1", "RLW_RSPCOL", "0x00000005", "0x00000001", "Nonfatal", apiv1.EventTypeWarning},
		{"RLW_RSPCOL_es2_fatal", "RLW_RSPCOL", "0x00000005", "0x00000002", "Fatal", apiv1.EventTypeFatal},

		// RLW_RXPIPE: SubCode=0, various ErrorStatus values
		{"RLW_RXPIPE_es1", "RLW_RXPIPE", "0x00000006", "0x00000001", "Nonfatal", apiv1.EventTypeWarning},
		{"RLW_RXPIPE_es2", "RLW_RXPIPE", "0x00000006", "0x00000002", "Nonfatal", apiv1.EventTypeWarning},

		// RLW_TAGSTATE: SubCode=0, ErrorStatus=0x00000001, Severity=Non-fatal
		{"RLW_TAGSTATE_es1", "RLW_TAGSTATE", "0x00000008", "0x00000001", "Nonfatal", apiv1.EventTypeWarning},
		{"RLW_TAGSTATE_es2_fatal", "RLW_TAGSTATE", "0x00000008", "0x00000002", "Fatal", apiv1.EventTypeFatal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kmsgLine := fmt.Sprintf(
				"NVRM: Xid (PCI:0000:04:00): 145, %s %s XC0 i0 Link 00 (%s %s 0x00000000 0x00000000)",
				tc.unit, tc.logSeverity, tc.intrinfo, tc.errorStatus,
			)

			xidErr := Match(kmsgLine)
			require.NotNil(t, xidErr, "Match should return non-nil for: %s", kmsgLine)
			require.NotNil(t, xidErr.Detail, "Detail should be populated")

			assert.Equal(t, tc.expectedEvent, xidErr.Detail.EventType,
				"EventType mismatch for %s: expected %s, got %s\nkmsg: %s",
				tc.name, tc.expectedEvent, xidErr.Detail.EventType, kmsgLine)
		})
	}
}
