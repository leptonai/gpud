package xid

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
)

func TestStateUpdateBasedOnEvents(t *testing.T) {
	createXidEvent := func(xid uint64, eventType common.EventType, suggestedAction common.RepairActionType) components.Event {
		xidErr := XidError{
			Xid:        xid,
			DataSource: "test",
			SuggestedActionsByGPUd: &common.SuggestedActions{
				RepairActions: []common.RepairActionType{suggestedAction},
			},
		}
		xidData, _ := json.Marshal(xidErr)
		return components.Event{
			Name:      EventNameErroXid,
			Type:      eventType,
			ExtraInfo: map[string]string{EventKeyErroXidData: string(xidData)},
		}
	}

	t.Run("no event found", func(t *testing.T) {
		state := EvolveHealthyState([]components.Event{})
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
		assert.Equal(t, "XIDComponent is healthy", state.Reason)
	})

	t.Run("critical xid", func(t *testing.T) {
		events := []components.Event{
			createXidEvent(123, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.False(t, state.Healthy)
		assert.Equal(t, components.StateDegraded, state.Health)
		assert.Contains(t, state.Error, "xid 123")
	})

	t.Run("fatal xid", func(t *testing.T) {
		events := []components.Event{
			createXidEvent(456, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.False(t, state.Healthy)
		assert.Equal(t, components.StateUnhealthy, state.Health)
		assert.Contains(t, state.Error, "xid 456")
	})

	t.Run("reboot recover", func(t *testing.T) {
		events := []components.Event{
			{Name: "reboot"},
			createXidEvent(789, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
	})

	t.Run("reboot multiple time cannot recover", func(t *testing.T) {
		events := []components.Event{
			createXidEvent(789, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createXidEvent(789, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
			{Name: "reboot"},
			createXidEvent(789, common.EventTypeCritical, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.False(t, state.Healthy)
		assert.Equal(t, common.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	})

	t.Run("SetHealthy", func(t *testing.T) {
		events := []components.Event{
			{Name: "SetHealthy"},
			createXidEvent(789, common.EventTypeFatal, common.RepairActionTypeRebootSystem),
		}
		state := EvolveHealthyState(events)
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
		assert.Nil(t, state.SuggestedActions)
	})

	t.Run("invalid xid", func(t *testing.T) {
		events := []components.Event{
			{
				Name:      EventNameErroXid,
				Type:      common.EventTypeCritical,
				ExtraInfo: map[string]string{EventKeyErroXidData: "invalid json"},
			},
		}
		state := EvolveHealthyState(events)
		assert.True(t, state.Healthy)
		assert.Equal(t, components.StateHealthy, state.Health)
	})
}
