package xid

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
)

func createXidEvent(eventTime time.Time, xid uint64, eventType common.EventType, suggestedAction common.RepairActionType) components.Event {
	xidErr := xidErrorFromDmesg{
		Xid:        xid,
		DataSource: "test",
		DeviceUUID: "PCI:0000:9b:00",
		SuggestedActionsByGPUd: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{suggestedAction},
		},
	}
	xidData, _ := json.Marshal(xidErr)
	ret := components.Event{
		Name:      EventNameErrorXid,
		Type:      eventType,
		ExtraInfo: map[string]string{EventKeyErrorXidData: string(xidData)},
	}
	if !eventTime.IsZero() {
		ret.Time = metav1.Time{Time: eventTime}
	}
	return ret
}

func TestStateUpdateBasedOnEvents(t *testing.T) {
	t.Run("no event found", func(t *testing.T) {
		state := EvolveHealthyState([]components.Event{})
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
		assert.Equal(t, "XIDComponent is healthy", state.Reason)
	})

	t.Run("critical xid", func(t *testing.T) {
		events := []components.Event{
			createXidEvent(time.Time{}, 123, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.False(t, state.Healthy)
		assert.Equal(t, components.StateDegraded, state.Health)
		assert.Equal(t, "XID 123 detected on PCI:0000:9b:00", state.Reason)
	})

	t.Run("fatal xid", func(t *testing.T) {
		events := []components.Event{
			createXidEvent(time.Time{}, 456, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.False(t, state.Healthy)
		assert.Equal(t, components.StateUnhealthy, state.Health)
		assert.Equal(t, "XID 456 detected on PCI:0000:9b:00", state.Reason)
	})

	t.Run("reboot recover", func(t *testing.T) {
		events := []components.Event{
			{Name: "reboot"},
			createXidEvent(time.Time{}, 789, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
	})

	t.Run("reboot multiple time cannot recover", func(t *testing.T) {
		events := []components.Event{
			createXidEvent(time.Time{}, 94, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createXidEvent(time.Time{}, 94, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createXidEvent(time.Time{}, 94, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
			createXidEvent(time.Time{}, 31, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.False(t, state.Healthy)
		assert.Equal(t, common.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	})

	t.Run("SetHealthy", func(t *testing.T) {
		events := []components.Event{
			{Name: "SetHealthy"},
			createXidEvent(time.Time{}, 789, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
		assert.Nil(t, state.SuggestedActions)
	})

	t.Run("invalid xid", func(t *testing.T) {
		events := []components.Event{
			{
				Name:      EventNameErrorXid,
				Type:      common.EventTypeCritical,
				ExtraInfo: map[string]string{EventKeyErrorXidData: "invalid json"},
			},
		}
		state := EvolveHealthyState(events)
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
	})
}

func TestXidErrorFromDmesgJSON(t *testing.T) {
	testTime := metav1.Time{Time: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}

	t.Run("successful marshaling", func(t *testing.T) {
		xidErr := xidErrorFromDmesg{
			Time:       testTime,
			DataSource: "test-source",
			DeviceUUID: "test-uuid",
			Xid:        123,
			SuggestedActionsByGPUd: &common.SuggestedActions{
				RepairActions: []common.RepairActionType{common.RepairActionTypeRebootSystem},
			},
			CriticalErrorMarkedByGPUd: true,
		}

		jsonBytes, err := xidErr.JSON()
		assert.NoError(t, err)
		assert.NotNil(t, jsonBytes)

		// Verify JSON structure by unmarshaling
		var unmarshaled xidErrorFromDmesg
		err = json.Unmarshal(jsonBytes, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, xidErr.Time.UTC(), unmarshaled.Time.UTC())
		assert.Equal(t, xidErr.DataSource, unmarshaled.DataSource)
		assert.Equal(t, xidErr.DeviceUUID, unmarshaled.DeviceUUID)
		assert.Equal(t, xidErr.Xid, unmarshaled.Xid)
		assert.Equal(t, xidErr.CriticalErrorMarkedByGPUd, unmarshaled.CriticalErrorMarkedByGPUd)
		assert.Equal(t, xidErr.SuggestedActionsByGPUd.RepairActions, unmarshaled.SuggestedActionsByGPUd.RepairActions)
	})

	t.Run("minimal fields", func(t *testing.T) {
		xidErr := xidErrorFromDmesg{
			Time:       testTime,
			DataSource: "test-source",
			DeviceUUID: "test-uuid",
			Xid:        123,
		}

		jsonBytes, err := xidErr.JSON()
		assert.NoError(t, err)
		assert.NotNil(t, jsonBytes)

		var unmarshaled xidErrorFromDmesg
		err = json.Unmarshal(jsonBytes, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, xidErr.Time.UTC(), unmarshaled.Time.UTC())
		assert.Equal(t, xidErr.DataSource, unmarshaled.DataSource)
		assert.Equal(t, xidErr.DeviceUUID, unmarshaled.DeviceUUID)
		assert.Equal(t, xidErr.Xid, unmarshaled.Xid)
		assert.Nil(t, unmarshaled.SuggestedActionsByGPUd)
		assert.False(t, unmarshaled.CriticalErrorMarkedByGPUd)
	})
}
