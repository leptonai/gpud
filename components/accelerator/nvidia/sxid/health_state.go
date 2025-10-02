package sxid

import (
	"encoding/json"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

const (
	healthStateHealthy   = 0
	healthStateDegraded  = 1
	healthStateUnhealthy = 2
)

func translateToStateHealth(health int) apiv1.HealthStateType {
	switch health {
	case healthStateHealthy:
		return apiv1.HealthStateTypeHealthy
	case healthStateDegraded:
		return apiv1.HealthStateTypeDegraded
	case healthStateUnhealthy:
		return apiv1.HealthStateTypeUnhealthy
	default:
		return apiv1.HealthStateTypeHealthy
	}
}

const rebootThreshold = 2

// evolveHealthyState resolves the state of the SXID error component.
// note: assume events are sorted by time in descending order
func evolveHealthyState(events eventstore.Events) (ret apiv1.HealthState) {
	defer func() {
		log.Logger.Debugf("EvolveHealthyState: %v", ret)
	}()
	var lastSuggestedAction *apiv1.SuggestedActions
	var lastSXidErr *sxidErrorEventDetail
	lastHealth := healthStateHealthy
	sxidRebootMap := make(map[uint64]int)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		log.Logger.Debugf("EvolveHealthyState: event: %v %v %+v %+v %+v", event.Time, event.Name, lastSuggestedAction, sxidRebootMap, lastSXidErr)
		if event.Name == EventNameErrorSXid {
			resolvedEvent := resolveSXIDEvent(event)
			var currSXidErr sxidErrorEventDetail
			if err := json.Unmarshal([]byte(resolvedEvent.ExtraInfo[EventKeyErrorSXidData]), &currSXidErr); err != nil {
				log.Logger.Errorf("failed to unmarshal event %s %s extra info: %s", resolvedEvent.Name, resolvedEvent.Message, err)
				continue
			}

			currEvent := healthStateHealthy
			switch resolvedEvent.Type {
			case string(apiv1.EventTypeCritical):
				currEvent = healthStateDegraded
			case string(apiv1.EventTypeFatal):
				currEvent = healthStateUnhealthy
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
				lastHealth = healthStateHealthy
				lastSuggestedAction = nil
				lastSXidErr = nil
			}
			for v, count := range sxidRebootMap {
				sxidRebootMap[v] = count + 1
			}
		} else if event.Name == "SetHealthy" {
			lastHealth = healthStateHealthy
			lastSuggestedAction = nil
			lastSXidErr = nil
			sxidRebootMap = make(map[uint64]int)
		}
	}
	var reason string
	if lastSXidErr == nil {
		reason = "SXIDComponent is healthy"
	} else {
		if sxidDetail, ok := GetDetail(int(lastSXidErr.SXid)); ok {
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

func resolveSXIDEvent(event eventstore.Event) eventstore.Event {
	ret := event
	if event.ExtraInfo != nil {
		if currSXid, err := strconv.Atoi(event.ExtraInfo[EventKeyErrorSXidData]); err == nil {
			detail, ok := GetDetail(currSXid)
			if !ok {
				return ret
			}
			ret.Type = string(detail.EventType)
			ret.Message = fmt.Sprintf("SXID %d(%s) detected on %s", currSXid, detail.Name, event.ExtraInfo[EventKeyDeviceUUID])

			sxidErr := sxidErrorEventDetail{
				Time:                   metav1.NewTime(event.Time),
				DataSource:             "kmsg",
				DeviceUUID:             event.ExtraInfo[EventKeyDeviceUUID],
				SXid:                   uint64(currSXid),
				SuggestedActionsByGPUd: detail.SuggestedActionsByGPUd,
			}
			raw, _ := json.Marshal(sxidErr)

			ret.ExtraInfo[EventKeyErrorSXidData] = string(raw)
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
}
