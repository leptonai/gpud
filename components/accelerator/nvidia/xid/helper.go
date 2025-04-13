package xid

import (
	"encoding/json"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/xid"
)

const (
	StateHealthy   = 0
	StateDegraded  = 1
	StateUnhealthy = 2

	rebootThreshold = 2
)

// EvolveHealthyState resolves the state of the XID error component.
// note: assume events are sorted by time in descending order
func EvolveHealthyState(events []apiv1.Event) (ret apiv1.State) {
	defer func() {
		log.Logger.Debugf("EvolveHealthyState: %v", ret)
	}()
	var lastSuggestedAction *apiv1.SuggestedActions
	var lastXidErr *xidErrorEventDetail
	lastHealth := StateHealthy
	xidRebootMap := make(map[uint64]int)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		log.Logger.Debugf("EvolveHealthyState: event: %v %v %+v %+v %+v", event.Time, event.Name, lastSuggestedAction, xidRebootMap, lastXidErr)
		if event.Name == EventNameErrorXid {
			resolvedEvent := resolveXIDEvent(event)
			var currXidErr xidErrorEventDetail
			if err := json.Unmarshal([]byte(resolvedEvent.DeprecatedExtraInfo[EventKeyErrorXidData]), &currXidErr); err != nil {
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
			lastXidErr = &currXidErr
			if currXidErr.SuggestedActionsByGPUd != nil && len(currXidErr.SuggestedActionsByGPUd.RepairActions) > 0 {
				if currXidErr.SuggestedActionsByGPUd.RepairActions[0] == apiv1.RepairActionTypeRebootSystem {
					if count, ok := xidRebootMap[currXidErr.Xid]; !ok {
						xidRebootMap[currXidErr.Xid] = 0
					} else if count >= rebootThreshold {
						currXidErr.SuggestedActionsByGPUd.RepairActions[0] = apiv1.RepairActionTypeHardwareInspection
					}
				}
				currXidErr.SuggestedActionsByGPUd.RepairActions = currXidErr.SuggestedActionsByGPUd.RepairActions[:1]
				lastSuggestedAction = currXidErr.SuggestedActionsByGPUd
			}
		} else if event.Name == "reboot" {
			if lastSuggestedAction != nil && len(lastSuggestedAction.RepairActions) > 0 && (lastSuggestedAction.RepairActions[0] == apiv1.RepairActionTypeRebootSystem || lastSuggestedAction.RepairActions[0] == apiv1.RepairActionTypeCheckUserAppAndGPU) {
				lastHealth = StateHealthy
				lastSuggestedAction = nil
				lastXidErr = nil
			}
			for v, count := range xidRebootMap {
				xidRebootMap[v] = count + 1
			}
		} else if event.Name == "SetHealthy" {
			lastHealth = StateHealthy
			lastSuggestedAction = nil
			lastXidErr = nil
			xidRebootMap = make(map[uint64]int)
		}
	}
	var reason string
	if lastXidErr == nil {
		reason = "XIDComponent is healthy"
	} else {
		if xidDetail, ok := xid.GetDetail(int(lastXidErr.Xid)); ok {
			reason = fmt.Sprintf("XID %d(%s) detected on %s", lastXidErr.Xid, xidDetail.Name, lastXidErr.DeviceUUID)
		} else {
			reason = fmt.Sprintf("XID %d detected on %s", lastXidErr.Xid, lastXidErr.DeviceUUID)
		}
	}
	return apiv1.State{
		Name:              StateNameErrorXid,
		DeprecatedHealthy: lastHealth == StateHealthy,
		Health:            translateToStateHealth(lastHealth),
		Reason:            reason,
		SuggestedActions:  lastSuggestedAction,
	}
}

func translateToStateHealth(health int) apiv1.StateType {
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

func resolveXIDEvent(event apiv1.Event) apiv1.Event {
	ret := event
	if event.DeprecatedExtraInfo != nil {
		if currXid, err := strconv.Atoi(event.DeprecatedExtraInfo[EventKeyErrorXidData]); err == nil {
			detail, ok := xid.GetDetail(currXid)
			if !ok {
				return ret
			}
			ret.Type = detail.EventType
			ret.Message = fmt.Sprintf("XID %d(%s) detected on %s", currXid, detail.Name, event.DeprecatedExtraInfo[EventKeyDeviceUUID])
			ret.DeprecatedSuggestedActions = detail.SuggestedActionsByGPUd

			xidErr := xidErrorEventDetail{
				Time:                      event.Time,
				DataSource:                "kmsg",
				DeviceUUID:                event.DeprecatedExtraInfo[EventKeyDeviceUUID],
				Xid:                       uint64(currXid),
				SuggestedActionsByGPUd:    detail.SuggestedActionsByGPUd,
				CriticalErrorMarkedByGPUd: detail.CriticalErrorMarkedByGPUd,
			}

			raw, _ := xidErr.JSON()
			ret.DeprecatedExtraInfo[EventKeyErrorXidData] = string(raw)
		}
	}
	return ret
}

// xidErrorEventDetail represents an Xid error from kmsg.
type xidErrorEventDetail struct {
	// Time is the time of the event.
	Time metav1.Time `json:"time"`

	// DataSource is the source of the data.
	DataSource string `json:"data_source"`

	// DeviceUUID is the UUID of the device that has the error.
	DeviceUUID string `json:"device_uuid"`

	// Xid is the corresponding Xid from the raw event.
	// The monitoring component can use this Xid to decide its own action.
	Xid uint64 `json:"xid"`

	// SuggestedActionsByGPUd are the suggested actions for the error.
	SuggestedActionsByGPUd *apiv1.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`
	// CriticalErrorMarkedByGPUd is true if the GPUd marks this error as a critical error.
	// You may use this field to decide whether to alert or not.
	CriticalErrorMarkedByGPUd bool `json:"critical_error_marked_by_gpud"`
}

func (d *xidErrorEventDetail) JSON() ([]byte, error) {
	return json.Marshal(d)
}
