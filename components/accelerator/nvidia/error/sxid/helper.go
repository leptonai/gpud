package sxid

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/leptonai/gpud/common"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/log"
	nvidia_query_sxid "github.com/leptonai/gpud/nvidia-query/sxid"
)

const (
	StateHealthy   = 0
	StateDegraded  = 1
	StateUnhealthy = 2
)

const rebootThreshold = 2

// EvolveHealthyState resolves the state of the SXID error component.
// note: assume events are sorted by time in descending order
func EvolveHealthyState(events []components.Event) (ret components.State) {
	defer func() {
		log.Logger.Debugf("EvolveHealthyState: %v", ret)
	}()
	var lastSuggestedAction *common.SuggestedActions
	var lastSXidErr *SXidError
	lastHealth := StateHealthy
	sxidRebootMap := make(map[uint64]int)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		log.Logger.Debugf("EvolveHealthyState: event: %v %v %+v %+v %+v", event.Time, event.Name, lastSuggestedAction, sxidRebootMap, lastSXidErr)
		if event.Name == EventNameErroSXid {
			resolvedEvent := resolveSXIDEvent(event)
			var currSXidErr SXidError
			if err := json.Unmarshal([]byte(resolvedEvent.ExtraInfo[EventKeyErroSXidData]), &currSXidErr); err != nil {
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
			lastSXidErr = &currSXidErr
			if currSXidErr.SuggestedActionsByGPUd != nil && len(currSXidErr.SuggestedActionsByGPUd.RepairActions) > 0 {
				if currSXidErr.SuggestedActionsByGPUd.RepairActions[0] == common.RepairActionTypeRebootSystem {
					if count, ok := sxidRebootMap[currSXidErr.SXid]; !ok {
						sxidRebootMap[currSXidErr.SXid] = 0
					} else if count >= rebootThreshold {
						currSXidErr.SuggestedActionsByGPUd.RepairActions[0] = common.RepairActionTypeHardwareInspection
					}
				}
				lastSuggestedAction = currSXidErr.SuggestedActionsByGPUd
			}
		} else if event.Name == "reboot" {
			if lastSuggestedAction != nil && len(lastSuggestedAction.RepairActions) > 0 && (lastSuggestedAction.RepairActions[0] == common.RepairActionTypeRebootSystem || lastSuggestedAction.RepairActions[0] == common.RepairActionTypeCheckUserAppAndGPU) {
				lastHealth = StateHealthy
				lastSuggestedAction = nil
				lastSXidErr = nil
			}
			for sxid, count := range sxidRebootMap {
				sxidRebootMap[sxid] = count + 1
			}
		} else if event.Name == "SetHealthy" {
			lastHealth = StateHealthy
			lastSuggestedAction = nil
			lastSXidErr = nil
			sxidRebootMap = make(map[uint64]int)
		}
	}
	var reason string
	var stateError string
	if lastSXidErr == nil {
		reason = "SXIDComponent is healthy"
	} else {
		xidErrBytes, _ := lastSXidErr.JSON()
		reason = string(xidErrBytes)
		stateError = fmt.Sprintf("sxid %d detected by %s", lastSXidErr.SXid, lastSXidErr.DataSource)
	}
	return components.State{
		Name:             StateNameErrorSXid,
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

func resolveSXIDEvent(event components.Event) components.Event {
	ret := event
	if event.ExtraInfo != nil {
		if currSXid, err := strconv.Atoi(event.ExtraInfo[EventKeyErroSXidData]); err == nil {
			detail, ok := nvidia_query_sxid.GetDetail(currSXid)
			if !ok {
				return ret
			}
			ret.Type = detail.EventType
			ret.Message = fmt.Sprintf("XID %d detected on %s", currSXid, event.ExtraInfo[EventKeyDeviceUUID])
			ret.SuggestedActions = detail.SuggestedActionsByGPUd
			raw, _ := json.Marshal(&SXidError{
				Time:                      event.Time,
				DataSource:                "dmesg",
				DeviceUUID:                event.ExtraInfo[EventKeyDeviceUUID],
				SXid:                      uint64(currSXid),
				SuggestedActionsByGPUd:    detail.SuggestedActionsByGPUd,
				CriticalErrorMarkedByGPUd: detail.CriticalErrorMarkedByGPUd,
			})
			ret.ExtraInfo[EventKeyErroSXidData] = string(raw)
		}
	}
	return ret
}
