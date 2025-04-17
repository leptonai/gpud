package sxid

import (
	"encoding/json"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/sxid"
)

const (
	StateHealthy   = 0
	StateDegraded  = 1
	StateUnhealthy = 2

	rebootThreshold = 2
)

// evolveHealthyState resolves the state of the SXID error component.
// note: assume events are sorted by time in descending order
func evolveHealthyState(events apiv1.Events) (ret apiv1.HealthState) {
	defer func() {
		log.Logger.Debugf("EvolveHealthyState: %v", ret)
	}()
	var lastSuggestedAction *apiv1.SuggestedActions
	var lastSXidErr *sxidErrorEventDetail
	lastHealth := StateHealthy
	sxidRebootMap := make(map[uint64]int)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		log.Logger.Debugf("EvolveHealthyState: event: %v %v %+v %+v %+v", event.Time, event.Name, lastSuggestedAction, sxidRebootMap, lastSXidErr)
		if event.Name == EventNameErrorSXid {
			resolvedEvent := resolveSXIDEvent(event)
			var currSXidErr sxidErrorEventDetail
			if err := json.Unmarshal([]byte(resolvedEvent.DeprecatedExtraInfo[EventKeyErrorSXidData]), &currSXidErr); err != nil {
				log.Logger.Errorf("failed to unmarshal event %s %s extra info: %s", resolvedEvent.Name, resolvedEvent.Message, err)
				continue
			}

			currEvent := StateHealthy
			switch resolvedEvent.Type {
			case apiv1.EventTypeCritical:
				currEvent = StateDegraded
			case apiv1.EventTypeFatal:
				currEvent = StateUnhealthy
			}
			if currEvent < lastHealth {
				continue
			}
			lastHealth = currEvent
			lastSXidErr = &currSXidErr
			if currSXidErr.SuggestedActionsByGPUd != nil && len(currSXidErr.SuggestedActionsByGPUd.RepairActions) > 0 {
				if currSXidErr.SuggestedActionsByGPUd.RepairActions[0] == apiv1.RepairActionTypeRebootSystem {
					if count, ok := sxidRebootMap[currSXidErr.SXid]; !ok {
						sxidRebootMap[currSXidErr.SXid] = 0
					} else if count >= rebootThreshold {
						currSXidErr.SuggestedActionsByGPUd.RepairActions[0] = apiv1.RepairActionTypeHardwareInspection
					}
				}
				lastSXidErr.SuggestedActionsByGPUd.RepairActions = lastSXidErr.SuggestedActionsByGPUd.RepairActions[:1]
				lastSuggestedAction = currSXidErr.SuggestedActionsByGPUd
			}
		} else if event.Name == "reboot" {
			if lastSuggestedAction != nil && len(lastSuggestedAction.RepairActions) > 0 && (lastSuggestedAction.RepairActions[0] == apiv1.RepairActionTypeRebootSystem || lastSuggestedAction.RepairActions[0] == apiv1.RepairActionTypeCheckUserAppAndGPU) {
				lastHealth = StateHealthy
				lastSuggestedAction = nil
				lastSXidErr = nil
			}
			for v, count := range sxidRebootMap {
				sxidRebootMap[v] = count + 1
			}
		} else if event.Name == "SetHealthy" {
			lastHealth = StateHealthy
			lastSuggestedAction = nil
			lastSXidErr = nil
			sxidRebootMap = make(map[uint64]int)
		}
	}
	var reason string
	if lastSXidErr == nil {
		reason = "SXIDComponent is healthy"
	} else {
		if sxidDetail, ok := sxid.GetDetail(int(lastSXidErr.SXid)); ok {
			reason = fmt.Sprintf("SXID %d(%s) detected on %s", lastSXidErr.SXid, sxidDetail.Name, lastSXidErr.DeviceUUID)
		} else {
			reason = fmt.Sprintf("SXID %d detected on %s", lastSXidErr.SXid, lastSXidErr.DeviceUUID)
		}
	}
	return apiv1.HealthState{
		Name:             StateNameErrorSXid,
		Health:           translateToStateHealth(lastHealth),
		Reason:           reason,
		SuggestedActions: lastSuggestedAction,
	}
}

func translateToStateHealth(health int) apiv1.HealthStateType {
	switch health {
	case StateHealthy:
		return apiv1.StateTypeHealthy
	case StateDegraded:
		return apiv1.StateTypeDegraded
	case StateUnhealthy:
		return apiv1.StateTypeUnhealthy
	default:
		return apiv1.StateTypeHealthy
	}
}

func resolveSXIDEvent(event apiv1.Event) apiv1.Event {
	ret := event
	if event.DeprecatedExtraInfo != nil {
		if currSXid, err := strconv.Atoi(event.DeprecatedExtraInfo[EventKeyErrorSXidData]); err == nil {
			detail, ok := sxid.GetDetail(currSXid)
			if !ok {
				return ret
			}
			ret.Type = detail.EventType
			ret.Message = fmt.Sprintf("SXID %d(%s) detected on %s", currSXid, detail.Name, event.DeprecatedExtraInfo[EventKeyDeviceUUID])
			ret.DeprecatedSuggestedActions = detail.SuggestedActionsByGPUd

			sxidErr := sxidErrorEventDetail{
				Time:                      event.Time,
				DataSource:                "kmsg",
				DeviceUUID:                event.DeprecatedExtraInfo[EventKeyDeviceUUID],
				SXid:                      uint64(currSXid),
				SuggestedActionsByGPUd:    detail.SuggestedActionsByGPUd,
				CriticalErrorMarkedByGPUd: detail.CriticalErrorMarkedByGPUd,
			}
			raw, _ := sxidErr.JSON()

			ret.DeprecatedExtraInfo[EventKeyErrorSXidData] = string(raw)
		}
	}
	return ret
}

// sxidErrorEventDetail represents an SXid error from kmsg.
type sxidErrorEventDetail struct {
	// Time is the time of the event.
	Time metav1.Time `json:"time"`

	// DataSource is the source of the data.
	DataSource string `json:"data_source"`

	// DeviceUUID is the UUID of the device that has the error.
	DeviceUUID string `json:"device_uuid"`

	// SXid is the corresponding SXid from the raw event.
	// The monitoring component can use this SXid to decide its own action.
	SXid uint64 `json:"sxid"`

	// SuggestedActionsByGPUd are the suggested actions for the error.
	SuggestedActionsByGPUd *apiv1.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`
	// CriticalErrorMarkedByGPUd is true if the GPUd marks this error as a critical error.
	// You may use this field to decide whether to alert or not.
	CriticalErrorMarkedByGPUd bool `json:"critical_error_marked_by_gpud"`
}

func (d *sxidErrorEventDetail) JSON() ([]byte, error) {
	return json.Marshal(d)
}
