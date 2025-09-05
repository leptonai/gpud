package xid

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia-query/xid"
)

const (
	StateHealthy   = 0
	StateDegraded  = 1
	StateUnhealthy = 2

	rebootThreshold = 2
)

// evolveHealthyState resolves the state of the XID error component.
// note: assume events are sorted by time in descending order
func evolveHealthyState(events eventstore.Events, devices map[string]device.Device) (ret apiv1.HealthState) {
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
			resolvedEvent := resolveXIDEvent(event, devices)
			var currXidErr xidErrorEventDetail
			if err := json.Unmarshal([]byte(resolvedEvent.ExtraInfo[EventKeyErrorXidData]), &currXidErr); err != nil {
				log.Logger.Errorf("failed to unmarshal event %s %s extra info: %s", resolvedEvent.Name, resolvedEvent.Message, err)
				continue
			}

			currEvent := StateHealthy
			switch resolvedEvent.Type {
			case string(apiv1.EventTypeCritical):
				currEvent = StateDegraded
			case string(apiv1.EventTypeFatal):
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
		reason = newXIDErrorReason(int(lastXidErr.Xid), lastXidErr.DeviceUUID, devices)
	}
	return apiv1.HealthState{
		Name:             StateNameErrorXid,
		Health:           translateToStateHealth(lastHealth),
		Reason:           reason,
		SuggestedActions: lastSuggestedAction,
	}
}

func newXIDErrorReason(xidVal int, deviceID string, devices map[string]device.Device) string {
	var suffix string
	uuid := convertBusIDToUUID(deviceID, devices)
	if uuid != "" {
		suffix = fmt.Sprintf("GPU %s UUID:%s", deviceID, uuid)
	} else {
		suffix = fmt.Sprintf("GPU %s", deviceID)
	}
	var reason string
	if xidDetail, ok := xid.GetDetail(xidVal); ok {
		reason = fmt.Sprintf("XID %d (%s) detected on %s", xidVal, xidDetail.Name, suffix)
	} else {
		reason = fmt.Sprintf("XID %d detected on %s", xidVal, suffix)
	}
	return reason
}

func convertBusIDToUUID(busID string, devices map[string]device.Device) string {
	busID = fmt.Sprintf("%s.", strings.TrimPrefix(busID, "PCI:"))
	var uuid string
	for k, v := range devices {
		if strings.HasPrefix(v.PCIBusID(), busID) {
			uuid = k
			break
		}
	}
	return uuid
}

func translateToStateHealth(health int) apiv1.HealthStateType {
	switch health {
	case StateHealthy:
		return apiv1.HealthStateTypeHealthy

	case StateDegraded:
		return apiv1.HealthStateTypeDegraded

	case StateUnhealthy:
		return apiv1.HealthStateTypeUnhealthy

	default:
		return apiv1.HealthStateTypeHealthy
	}
}

func resolveXIDEvent(event eventstore.Event, devices map[string]device.Device) eventstore.Event {
	ret := event
	if event.ExtraInfo != nil {
		if currXid, err := strconv.Atoi(event.ExtraInfo[EventKeyErrorXidData]); err == nil {
			detail, ok := xid.GetDetail(currXid)
			if !ok {
				return ret
			}
			ret.Type = string(detail.EventType)
			ret.Message = newXIDErrorReason(currXid, event.ExtraInfo[EventKeyDeviceUUID], devices)

			xidErr := xidErrorEventDetail{
				Time:                   metav1.NewTime(event.Time),
				DataSource:             "kmsg",
				DeviceUUID:             event.ExtraInfo[EventKeyDeviceUUID],
				Xid:                    uint64(currXid),
				SuggestedActionsByGPUd: detail.SuggestedActionsByGPUd,
			}

			raw, _ := json.Marshal(xidErr)
			ret.ExtraInfo[EventKeyErrorXidData] = string(raw)
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
}
