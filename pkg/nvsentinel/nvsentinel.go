package nvsentinel

import (
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvsentinel/proto/datamodels"
)

// RecommendedAction mirrors the NVSentinel RecommendedAction enum as a
// string, so values stay readable in GPUd logs and JSON output.
type RecommendedAction string

const (
	RecommendedActionNone           RecommendedAction = "NONE"
	RecommendedActionComponentReset RecommendedAction = "COMPONENT_RESET"
	RecommendedActionContactSupport RecommendedAction = "CONTACT_SUPPORT"
	RecommendedActionRunFieldDiag   RecommendedAction = "RUN_FIELDDIAG"
	RecommendedActionRestartVM      RecommendedAction = "RESTART_VM"
	RecommendedActionRestartBM      RecommendedAction = "RESTART_BM"
	RecommendedActionReplaceVM      RecommendedAction = "REPLACE_VM"
	RecommendedActionRunDCGMEUD     RecommendedAction = "RUN_DCGMEUD"
	RecommendedActionCustom         RecommendedAction = "CUSTOM"
	RecommendedActionUnknown        RecommendedAction = "UNKNOWN"
)

// Entity identifies one impacted entity, for example a GPU UUID or an
// InfiniBand device name.
type Entity struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// HealthEvent is one NVSentinel health event in a GPUd-native form.
// It carries every field GPUd components need; the wire type stays in
// the generated proto package.
type HealthEvent struct {
	Agent          string            `json:"agent"`
	ComponentClass string            `json:"component_class"`
	CheckName      string            `json:"check_name"`
	IsFatal        bool              `json:"is_fatal"`
	IsHealthy      bool              `json:"is_healthy"`
	Message        string            `json:"message"`
	Action         RecommendedAction `json:"recommended_action"`
	// CustomAction is set when Action is RecommendedActionCustom.
	CustomAction       string            `json:"custom_recommended_action,omitempty"`
	ErrorCodes         []string          `json:"error_codes,omitempty"`
	Entities           []Entity          `json:"entities_impacted,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	NodeName           string            `json:"node_name"`
	ID                 string            `json:"id,omitempty"`
	GeneratedTimestamp time.Time         `json:"generated_timestamp"`
}

// EntityValue returns the value of the first impacted entity of the given
// type (for example "GPU" or "NIC").
func (e HealthEvent) EntityValue(entityType string) (string, bool) {
	for _, ent := range e.Entities {
		if ent.Type == entityType {
			return ent.Value, true
		}
	}
	return "", false
}

// SuggestedRepairActions maps the NVSentinel recommended action to GPUd
// repair actions. NVSentinel actions that have no GPUd counterpart (for
// example RUN_FIELDDIAG) return nil; the raw action stays on the event.
func (e HealthEvent) SuggestedRepairActions() []apiv1.RepairActionType {
	switch e.Action {
	case RecommendedActionRestartVM, RecommendedActionRestartBM:
		return []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}
	case RecommendedActionContactSupport, RecommendedActionReplaceVM:
		return []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}
	default:
		return nil
	}
}

// healthEventFromProto converts the wire type to the GPUd-native type.
// A missing timestamp falls back to the receive time, so every recorded
// event has a usable timestamp.
func healthEventFromProto(pb *datamodels.HealthEvent, receivedAt time.Time) HealthEvent {
	ev := HealthEvent{
		Agent:          pb.GetAgent(),
		ComponentClass: pb.GetComponentClass(),
		CheckName:      pb.GetCheckName(),
		IsFatal:        pb.GetIsFatal(),
		IsHealthy:      pb.GetIsHealthy(),
		Message:        pb.GetMessage(),
		Action:         RecommendedAction(pb.GetRecommendedAction().String()),
		CustomAction:   pb.GetCustomRecommendedAction(),
		ErrorCodes:     pb.GetErrorCode(),
		Metadata:       pb.GetMetadata(),
		NodeName:       pb.GetNodeName(),
		ID:             pb.GetId(),

		GeneratedTimestamp: receivedAt,
	}
	if pb.GetGeneratedTimestamp() != nil {
		ev.GeneratedTimestamp = pb.GetGeneratedTimestamp().AsTime()
	}

	for _, ent := range pb.GetEntitiesImpacted() {
		ev.Entities = append(ev.Entities, Entity{
			Type:  ent.GetEntityType(),
			Value: ent.GetEntityValue(),
		})
	}

	return ev
}
