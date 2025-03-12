package sxid

import (
	"encoding/json"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/log"
	nvidia_query_sxid "github.com/leptonai/gpud/pkg/nvidia-query/sxid"
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
	var lastSXidErr *sxidErrorFromDmesg
	lastHealth := StateHealthy
	sxidRebootMap := make(map[uint64]int)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		log.Logger.Debugf("EvolveHealthyState: event: %v %v %+v %+v %+v", event.Time, event.Name, lastSuggestedAction, sxidRebootMap, lastSXidErr)
		if event.Name == EventNameErroSXid {
			resolvedEvent := resolveSXIDEvent(event)
			var currSXidErr sxidErrorFromDmesg
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
		reason = fmt.Sprintf("sxid %d detected by %s", lastSXidErr.SXid, lastSXidErr.DataSource)
		if sxidDetail, ok := nvidia_query_sxid.GetDetail(int(lastSXidErr.SXid)); ok {
			stateError = sxidDetail.Name
		}
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
			ret.Message = fmt.Sprintf("SXID %d detected on %s", currSXid, event.ExtraInfo[EventKeyDeviceUUID])
			ret.SuggestedActions = detail.SuggestedActionsByGPUd

			sxidErr := sxidErrorFromDmesg{
				Time:                      event.Time,
				DataSource:                "dmesg",
				DeviceUUID:                event.ExtraInfo[EventKeyDeviceUUID],
				SXid:                      uint64(currSXid),
				SuggestedActionsByGPUd:    detail.SuggestedActionsByGPUd,
				CriticalErrorMarkedByGPUd: detail.CriticalErrorMarkedByGPUd,
			}
			raw, _ := sxidErr.JSON()

			ret.ExtraInfo[EventKeyErroSXidData] = string(raw)
		}
	}
	return ret
}

// sxidErrorFromDmesg represents an SXid error from dmesg.
type sxidErrorFromDmesg struct {
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
	SuggestedActionsByGPUd *common.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`
	// CriticalErrorMarkedByGPUd is true if the GPUd marks this error as a critical error.
	// You may use this field to decide whether to alert or not.
	CriticalErrorMarkedByGPUd bool `json:"critical_error_marked_by_gpud"`
}

func (sxidErr sxidErrorFromDmesg) JSON() ([]byte, error) {
	return json.Marshal(sxidErr)
}
