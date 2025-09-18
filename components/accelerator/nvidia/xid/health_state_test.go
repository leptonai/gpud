package xid

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
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
		assert.Equal(t, apiv1.HealthStateTypeDegraded, state.Health)
		assert.Equal(t, "XID 123 (SPI PMU RPC Write Failure) detected on GPU PCI:0000:9b:00 UUID:GPU-b850f46d-d5ea-c752-ddf3-c4453e44d3f7", state.Reason)
	})

	t.Run("fatal xid", func(t *testing.T) {
		events := eventstore.Events{
			createXidEvent(time.Time{}, 456, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
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

	t.Run("SetHealthy", func(t *testing.T) {
		events := eventstore.Events{
			{Name: "SetHealthy"},
			createXidEvent(time.Time{}, 789, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
		assert.Nil(t, state.SuggestedActions)
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
