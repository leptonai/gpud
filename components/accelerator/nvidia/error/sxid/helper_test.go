package sxid

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
)

func createSXidEvent(eventTime time.Time, sxid uint64, eventType common.EventType, suggestedAction common.RepairActionType) components.Event {
	sxidErr := sxidErrorFromDmesg{
		SXid:       sxid,
		DataSource: "test",
		SuggestedActionsByGPUd: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{suggestedAction},
		},
	}
	sxidData, _ := json.Marshal(sxidErr)
	ret := components.Event{
		Name:      EventNameErroSXid,
		Type:      eventType,
		ExtraInfo: map[string]string{EventKeyErroSXidData: string(sxidData)},
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
		assert.Equal(t, "SXIDComponent is healthy", state.Reason)
	})

	t.Run("critical sxid", func(t *testing.T) {
		events := []components.Event{
			createSXidEvent(time.Time{}, 123, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.False(t, state.Healthy)
		assert.Equal(t, components.StateUnhealthy, state.Health)
		assert.Contains(t, state.Error, "sxid 123")
	})

	t.Run("fatal xid", func(t *testing.T) {
		events := []components.Event{
			createSXidEvent(time.Time{}, 456, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.False(t, state.Healthy)
		assert.Equal(t, components.StateUnhealthy, state.Health)
		assert.Contains(t, state.Error, "sxid 456")
	})

	t.Run("reboot recover", func(t *testing.T) {
		events := []components.Event{
			{Name: "reboot"},
			createSXidEvent(time.Time{}, 789, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
	})

	t.Run("reboot multiple time cannot recover", func(t *testing.T) {
		events := []components.Event{
			createSXidEvent(time.Time{}, 94, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createSXidEvent(time.Time{}, 94, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createSXidEvent(time.Time{}, 94, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
			createSXidEvent(time.Time{}, 31, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.False(t, state.Healthy)
		assert.Equal(t, common.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	})

	t.Run("SetHealthy", func(t *testing.T) {
		events := []components.Event{
			{Name: "SetHealthy"},
			createSXidEvent(time.Time{}, 789, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
		assert.Nil(t, state.SuggestedActions)
	})

	t.Run("invalid sxid", func(t *testing.T) {
		events := []components.Event{
			{
				Name:      EventNameErroSXid,
				Type:      common.EventTypeFatal,
				ExtraInfo: map[string]string{EventKeyErroSXidData: "invalid json"},
			},
		}
		state := EvolveHealthyState(events)
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
	})
}

func TestSXidErrorFromDmesgJSON(t *testing.T) {
	testTime := metav1.Time{Time: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}

	t.Run("successful marshaling", func(t *testing.T) {
		sxidErr := sxidErrorFromDmesg{
			Time:       testTime,
			DataSource: "test-source",
			DeviceUUID: "test-uuid",
			SXid:       123,
			SuggestedActionsByGPUd: &common.SuggestedActions{
				RepairActions: []common.RepairActionType{common.RepairActionTypeRebootSystem},
			},
			CriticalErrorMarkedByGPUd: true,
		}

		jsonBytes, err := sxidErr.JSON()
		assert.NoError(t, err)
		assert.NotNil(t, jsonBytes)

		// Verify JSON structure by unmarshaling
		var unmarshaled sxidErrorFromDmesg
		err = json.Unmarshal(jsonBytes, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, sxidErr.Time.UTC(), unmarshaled.Time.UTC())
		assert.Equal(t, sxidErr.DataSource, unmarshaled.DataSource)
		assert.Equal(t, sxidErr.DeviceUUID, unmarshaled.DeviceUUID)
		assert.Equal(t, sxidErr.SXid, unmarshaled.SXid)
		assert.Equal(t, sxidErr.CriticalErrorMarkedByGPUd, unmarshaled.CriticalErrorMarkedByGPUd)
		assert.Equal(t, sxidErr.SuggestedActionsByGPUd.RepairActions, unmarshaled.SuggestedActionsByGPUd.RepairActions)
	})

	t.Run("minimal fields", func(t *testing.T) {
		sxidErr := sxidErrorFromDmesg{
			Time:       testTime,
			DataSource: "test-source",
			DeviceUUID: "test-uuid",
			SXid:       123,
		}

		jsonBytes, err := sxidErr.JSON()
		assert.NoError(t, err)
		assert.NotNil(t, jsonBytes)

		var unmarshaled sxidErrorFromDmesg
		err = json.Unmarshal(jsonBytes, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, sxidErr.Time.UTC(), unmarshaled.Time.UTC())
		assert.Equal(t, sxidErr.DataSource, unmarshaled.DataSource)
		assert.Equal(t, sxidErr.DeviceUUID, unmarshaled.DeviceUUID)
		assert.Equal(t, sxidErr.SXid, unmarshaled.SXid)
		assert.Nil(t, unmarshaled.SuggestedActionsByGPUd)
		assert.False(t, unmarshaled.CriticalErrorMarkedByGPUd)
	})
}
