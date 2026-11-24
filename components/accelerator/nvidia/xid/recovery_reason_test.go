package xid

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

func TestRecoveryReasonEightGPUs(t *testing.T) {
	for _, xid := range []uint64{74, 79} {
		for _, resolveUUID := range []bool{false, true} {
			t.Run(fmt.Sprintf("xid=%d/resolveUUID=%t", xid, resolveUUID), func(t *testing.T) {
				fault := createXidEvent(time.Unix(1, 0), xid, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem)
				original := evolveHealthyState(eventstore.Events{fault}, nil, 2)
				events := eventstore.Events{fault}
				devices := make(map[string]device.Device)
				var wantGPU []string
				for i := range 8 {
					busID := fmt.Sprintf("0000:%02x:00", i+1)
					id := "PCI:" + busID
					if resolveUUID {
						id = fmt.Sprintf("GPU-00000000-0000-0000-0000-%012d", i+1)
						devices[id] = testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", busID+".0")
					}
					wantGPU = append(wantGPU, id)
					event := recoveryEvent(t, busID, "0x2 (Node Reboot Required)")
					event.Time = time.Unix(2, 0)
					events = append(eventstore.Events{event}, events...)
				}

				state := evolveHealthyState(events, devices, 2)
				assert.Less(t, utf8.RuneCountInString(state.Reason), 1024)
				assert.Contains(t, state.Reason, fmt.Sprintf("XID %d", xid))
				assert.Equal(t, 8, strings.Count(state.Reason, "XID 154"))
				for _, id := range wantGPU {
					assert.Contains(t, state.Reason, "on GPU "+id)
				}
				if resolveUUID {
					assert.NotContains(t, state.Reason, "on GPU PCI:0000:01:00")
				}
				assert.NotContains(t, state.Reason, "do not restart the application")
				assert.NotContains(t, state.Reason, "warm reboot")
				assert.Equal(t, original.Health, state.Health)
				assert.Equal(t, original.SuggestedActions, state.SuggestedActions)
			})
		}
	}
}

func TestRecoveryReasonKeepsEachGPUAction(t *testing.T) {
	var events eventstore.Events
	for _, item := range []struct {
		gpu string
		to  string
	}{
		{"0000:01:00", "0x3 (Drain P2P)"},
		{"0000:02:00", "0x9 (Unknown)"},
		{"0000:03:00", "truncated"},
	} {
		events = append(events, recoveryEvent(t, item.gpu, item.to))
	}
	state := evolveHealthyState(events, nil, 2)
	assert.Contains(t, state.Reason, "XID 154 recovery action 0x3 (Drain P2P) on GPU PCI:0000:01:00")
	assert.Contains(t, state.Reason, "XID 154 recovery action 0x9 (Unknown) on GPU PCI:0000:02:00")
	assert.Contains(t, state.Reason, "XID 154 malformed recovery action on GPU PCI:0000:03:00")
	assert.Equal(t, apiv1.HealthStateTypeDegraded, state.Health)
	require.NotNil(t, state.SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeCheckUserAppAndGPU}, state.SuggestedActions.RepairActions)
}

func TestRecoveryReasonPreservesUUIDAndDetailedEvent(t *testing.T) {
	event := recoveryEvent(t, "0000:01:00", "0x2 (Node Reboot Required)")
	var payload xidErrorEventDetail
	require.NoError(t, json.Unmarshal([]byte(event.ExtraInfo[EventKeyErrorXidData]), &payload))
	payload.DeviceUUID = "GPU-00000000-0000-0000-0000-000000000001"
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	event.ExtraInfo[EventKeyErrorXidData] = string(raw)

	state := evolveHealthyState(eventstore.Events{event}, nil, 2)
	assert.Contains(t, state.Reason, "on GPU "+payload.DeviceUUID)
	assert.NotContains(t, state.Reason, "PCI:")
	assert.NotContains(t, state.Reason, payload.Description)
	assert.Equal(t, payload.SuggestedActionsByGPUd, state.SuggestedActions)
	resolved := resolveXIDEvent(event, nil)
	assert.Contains(t, resolved.Message, "0x4 (Drain and Reset) -> 0x2 (Node Reboot Required)")
	assert.Contains(t, resolved.Message, payload.Description)
	assert.Contains(t, resolved.Message, payload.DeviceUUID)
	var resolvedPayload xidErrorEventDetail
	require.NoError(t, json.Unmarshal([]byte(resolved.ExtraInfo[EventKeyErrorXidData]), &resolvedPayload))
	assert.Equal(t, payload.RecoveryAction, resolvedPayload.RecoveryAction)
	assert.Equal(t, payload.SuggestedActionsByGPUd, resolvedPayload.SuggestedActionsByGPUd)
}
