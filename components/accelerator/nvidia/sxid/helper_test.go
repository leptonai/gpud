package sxid

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func createSXidEvent(eventTime time.Time, sxid uint64, eventType apiv1.EventType, suggestedAction apiv1.RepairActionType) apiv1.Event {
	sxidErr := sxidErrorEventDetail{
		SXid:       sxid,
		DataSource: "test",
		DeviceUUID: "PCI:0000:9b:00",
		SuggestedActionsByGPUd: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{suggestedAction},
		},
	}
	sxidData, _ := json.Marshal(sxidErr)
	ret := apiv1.Event{
		Name:                EventNameErrorSXid,
		Type:                eventType,
		DeprecatedExtraInfo: map[string]string{EventKeyErrorSXidData: string(sxidData)},
	}
	if !eventTime.IsZero() {
		ret.Time = metav1.Time{Time: eventTime}
	}
	return ret
}

func TestStateUpdateBasedOnEvents(t *testing.T) {
	t.Run("no event found", func(t *testing.T) {
		state := evolveHealthyState(apiv1.Events{})
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
		assert.Equal(t, "SXIDComponent is healthy", state.Reason)
	})

	t.Run("critical sxid", func(t *testing.T) {
		events := apiv1.Events{
			createSXidEvent(time.Time{}, 123, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
		assert.Equal(t, "SXID 123 detected on PCI:0000:9b:00", state.Reason)
	})

	t.Run("fatal xid", func(t *testing.T) {
		events := apiv1.Events{
			createSXidEvent(time.Time{}, 456, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
		assert.Equal(t, "SXID 456 detected on PCI:0000:9b:00", state.Reason)
	})

	t.Run("reboot recover", func(t *testing.T) {
		events := apiv1.Events{
			{Name: "reboot"},
			createSXidEvent(time.Time{}, 789, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})

	t.Run("reboot multiple time cannot recover", func(t *testing.T) {
		events := apiv1.Events{
			createSXidEvent(time.Time{}, 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createSXidEvent(time.Time{}, 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createSXidEvent(time.Time{}, 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			createSXidEvent(time.Time{}, 31, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	})

	t.Run("SetHealthy", func(t *testing.T) {
		events := apiv1.Events{
			{Name: "SetHealthy"},
			createSXidEvent(time.Time{}, 789, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
		assert.Nil(t, state.SuggestedActions)
	})

	t.Run("invalid sxid", func(t *testing.T) {
		events := apiv1.Events{
			{
				Name:                EventNameErrorSXid,
				Type:                apiv1.EventTypeFatal,
				DeprecatedExtraInfo: map[string]string{EventKeyErrorSXidData: "invalid json"},
			},
		}
		state := evolveHealthyState(events)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})
}

func Test_sxidErrorEventDetailJSON(t *testing.T) {
	testTime := metav1.Time{Time: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}

	t.Run("successful marshaling", func(t *testing.T) {
		sxidErr := sxidErrorEventDetail{
			Time:       testTime,
			DataSource: "test-source",
			DeviceUUID: "test-uuid",
			SXid:       123,
			SuggestedActionsByGPUd: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem},
			},
			CriticalErrorMarkedByGPUd: true,
		}

		jsonBytes, err := sxidErr.JSON()
		assert.NoError(t, err)
		assert.NotNil(t, jsonBytes)

		// Verify JSON structure by unmarshaling
		var unmarshaled sxidErrorEventDetail
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
		sxidErr := sxidErrorEventDetail{
			Time:       testTime,
			DataSource: "test-source",
			DeviceUUID: "test-uuid",
			SXid:       123,
		}

		jsonBytes, err := sxidErr.JSON()
		assert.NoError(t, err)
		assert.NotNil(t, jsonBytes)

		var unmarshaled sxidErrorEventDetail
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
