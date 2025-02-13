package xid

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/log"
	nvidia_query_xid "github.com/leptonai/gpud/pkg/nvidia-query/xid"
)

const (
	StateHealthy   = 0
	StateDegraded  = 1
	StateUnhealthy = 2
)

const rebootThreshold = 2

// EvolveHealthyState resolves the state of the XID error component.
// note: assume events are sorted by time in descending order
func EvolveHealthyState(events []components.Event) (ret components.State) {
	defer func() {
		log.Logger.Debugf("EvolveHealthyState: %v", ret)
	}()
	var lastSuggestedAction *common.SuggestedActions
	var lastXidErr *XidError
	lastHealth := StateHealthy
	xidRebootMap := make(map[uint64]int)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		log.Logger.Debugf("EvolveHealthyState: event: %v %v %+v %+v %+v", event.Time, event.Name, lastSuggestedAction, xidRebootMap, lastXidErr)
		if event.Name == EventNameErroXid {
			resolvedEvent := resolveXIDEvent(event)
			var currXidErr XidError
			if err := json.Unmarshal([]byte(resolvedEvent.ExtraInfo[EventKeyErroXidData]), &currXidErr); err != nil {
				log.Logger.Errorf("failed to unmarshal event %s %s extra info: %s", resolvedEvent.Name, resolvedEvent.Message, err)
				continue
			}

			currEvent := StateHealthy
			switch resolvedEvent.Type {
			case common.EventTypeCritical:
				currEvent = StateDegraded
			case common.EventTypeFatal:
				currEvent = StateUnhealthy
			}
			if currEvent < lastHealth {
				continue
			}
			lastHealth = currEvent
			lastXidErr = &currXidErr
			if currXidErr.SuggestedActionsByGPUd != nil && len(currXidErr.SuggestedActionsByGPUd.RepairActions) > 0 {
				if currXidErr.SuggestedActionsByGPUd.RepairActions[0] == common.RepairActionTypeRebootSystem {
					if count, ok := xidRebootMap[currXidErr.Xid]; !ok {
						xidRebootMap[currXidErr.Xid] = 0
					} else if count >= rebootThreshold {
						currXidErr.SuggestedActionsByGPUd.RepairActions[0] = common.RepairActionTypeHardwareInspection
					}
				}
				lastSuggestedAction = currXidErr.SuggestedActionsByGPUd
			}
		} else if event.Name == "reboot" {
			if lastSuggestedAction != nil && len(lastSuggestedAction.RepairActions) > 0 && (lastSuggestedAction.RepairActions[0] == common.RepairActionTypeRebootSystem || lastSuggestedAction.RepairActions[0] == common.RepairActionTypeCheckUserAppAndGPU) {
				lastHealth = StateHealthy
				lastSuggestedAction = nil
				lastXidErr = nil
			}
			for xid, count := range xidRebootMap {
				xidRebootMap[xid] = count + 1
			}
		} else if event.Name == "SetHealthy" {
			lastHealth = StateHealthy
			lastSuggestedAction = nil
			lastXidErr = nil
			xidRebootMap = make(map[uint64]int)
		}
	}
	var reason string
	var stateError string
	if lastXidErr == nil {
		reason = "XIDComponent is healthy"
	} else {
		xidErrBytes, _ := lastXidErr.JSON()
		reason = string(xidErrBytes)
		stateError = fmt.Sprintf("xid %d detected by %s", lastXidErr.Xid, lastXidErr.DataSource)
	}
	return components.State{
		Name:             StateNameErrorXid,
		Healthy:          lastHealth == StateHealthy,
		Health:           translateToStateHealth(lastHealth),
		Reason:           reason,
		Error:            stateError,
		SuggestedActions: lastSuggestedAction,
	}
}

func translateToStateHealth(health int) string {
	switch health {
	case StateHealthy:
		return components.StateHealthy
	case StateDegraded:
		return components.StateDegraded
	case StateUnhealthy:
		return components.StateUnhealthy
	default:
		return components.StateHealthy
	}
}

func resolveXIDEvent(event components.Event) components.Event {
	ret := event
	if event.ExtraInfo != nil {
		if currXid, err := strconv.Atoi(event.ExtraInfo[EventKeyErroXidData]); err == nil {
			detail, ok := nvidia_query_xid.GetDetail(currXid)
			if !ok {
				return ret
			}
			ret.Type = detail.EventType
			ret.Message = fmt.Sprintf("XID %d detected on %s", currXid, event.ExtraInfo[EventKeyDeviceUUID])
			ret.SuggestedActions = detail.SuggestedActionsByGPUd
			raw, _ := json.Marshal(&XidError{
				Time:                      event.Time,
				DataSource:                "dmesg",
				DeviceUUID:                event.ExtraInfo[EventKeyDeviceUUID],
				Xid:                       uint64(currXid),
				SuggestedActionsByGPUd:    detail.SuggestedActionsByGPUd,
				CriticalErrorMarkedByGPUd: detail.CriticalErrorMarkedByGPUd,
			})
			ret.ExtraInfo[EventKeyErroXidData] = string(raw)
		}
	}
	return ret
}
