package nvsentinel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvsentinel/proto/datamodels"
)

func TestHealthEventEntityValue(t *testing.T) {
	ev := HealthEvent{
		Entities: []Entity{
			{Type: "GPU", Value: "GPU-abc"},
			{Type: "NVLINK", Value: "3"},
		},
	}

	v, ok := ev.EntityValue("GPU")
	assert.True(t, ok)
	assert.Equal(t, "GPU-abc", v)

	_, ok = ev.EntityValue("NIC")
	assert.False(t, ok)
}

func TestHealthEventSuggestedRepairActions(t *testing.T) {
	tests := []struct {
		action   RecommendedAction
		expected []apiv1.RepairActionType
	}{
		{RecommendedActionRestartBM, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{RecommendedActionRestartVM, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{RecommendedActionContactSupport, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}},
		{RecommendedActionReplaceVM, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}},
		{RecommendedActionNone, nil},
		{RecommendedActionComponentReset, nil},
		{RecommendedActionRunFieldDiag, nil},
		{RecommendedActionRunDCGMEUD, nil},
		{RecommendedActionCustom, nil},
		{RecommendedActionUnknown, nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			ev := HealthEvent{Action: tt.action}
			assert.Equal(t, tt.expected, ev.SuggestedRepairActions())
		})
	}
}

func TestHealthEventFromProto(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pb := &datamodels.HealthEvent{
		Version:            1,
		Agent:              "syslog-health-monitor",
		ComponentClass:     "GPU",
		CheckName:          "SysLogsXIDError",
		IsFatal:            true,
		IsHealthy:          false,
		Message:            "Xid 79 detected",
		RecommendedAction:  datamodels.RecommendedAction_RESTART_BM,
		ErrorCode:          []string{"79"},
		EntitiesImpacted:   []*datamodels.Entity{{EntityType: "GPU", EntityValue: "GPU-abc"}},
		Metadata:           map[string]string{"k": "v"},
		GeneratedTimestamp: timestamppb.New(ts),
		NodeName:           "node-1",
		Id:                 "event-id-1",
	}

	ev := healthEventFromProto(pb, time.Now())

	assert.Equal(t, "syslog-health-monitor", ev.Agent)
	assert.Equal(t, "GPU", ev.ComponentClass)
	assert.Equal(t, "SysLogsXIDError", ev.CheckName)
	assert.True(t, ev.IsFatal)
	assert.False(t, ev.IsHealthy)
	assert.Equal(t, "Xid 79 detected", ev.Message)
	assert.Equal(t, RecommendedActionRestartBM, ev.Action)
	assert.Equal(t, []string{"79"}, ev.ErrorCodes)
	assert.Equal(t, []Entity{{Type: "GPU", Value: "GPU-abc"}}, ev.Entities)
	assert.Equal(t, map[string]string{"k": "v"}, ev.Metadata)
	assert.Equal(t, "node-1", ev.NodeName)
	assert.Equal(t, "event-id-1", ev.ID)
	assert.Equal(t, ts, ev.GeneratedTimestamp)
}

func TestHealthEventFromProtoMissingTimestamp(t *testing.T) {
	receivedAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	ev := healthEventFromProto(&datamodels.HealthEvent{}, receivedAt)
	assert.Equal(t, receivedAt, ev.GeneratedTimestamp)
}
