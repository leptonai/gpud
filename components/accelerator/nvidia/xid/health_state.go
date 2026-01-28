package xid

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
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

var (
	catalogMnemonicOnce sync.Once
	catalogMnemonicMap  map[int]string
)

func init() {
	catalogMnemonicOnce.Do(func() {
		catalogMnemonicMap = make(map[int]string, len(catalogEntries))
		for _, entry := range catalogEntries {
			catalogMnemonicMap[entry.Code] = entry.Mnemonic
		}
	})
}

// evolveHealthyState resolves the state of the XID error component.
// note: assume events are sorted by time in descending order
func evolveHealthyState(events eventstore.Events, devices map[string]device.Device, rebootThreshold int) (ret apiv1.HealthState) {
	defer func() {
		log.Logger.Debugf("EvolveHealthyState: %v", ret)
	}()
	var lastSuggestedAction *apiv1.SuggestedActions
	var lastXidErr *xidErrorEventDetail
	lastHealth := healthStateHealthy
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
			// Clear the error state on reboot ONLY if:
			// 1. lastSuggestedAction is not nil (XID must have SuggestedActionsByGPUd defined)
			// 2. The first repair action is RebootSystem or CheckUserAppAndGPU
			//
			// IMPORTANT: XIDs with SuggestedActionsByGPUd=nil will NOT be cleared on reboot.
			// This is why NVLink XIDs (144-150) must have SuggestedActionsByGPUd set in their
			// base catalog entries - even when the extended log format isn't matched.
			if lastSuggestedAction != nil && len(lastSuggestedAction.RepairActions) > 0 && (lastSuggestedAction.RepairActions[0] == apiv1.RepairActionTypeRebootSystem || lastSuggestedAction.RepairActions[0] == apiv1.RepairActionTypeCheckUserAppAndGPU) {
				lastHealth = healthStateHealthy
				lastSuggestedAction = nil
				lastXidErr = nil
			}
			for v, count := range xidRebootMap {
				xidRebootMap[v] = count + 1
			}
		}
	}
	var reason string
	if lastXidErr == nil {
		reason = "XIDComponent is healthy"
	} else {
		reason = lastXidErr.buildMessage(devices)
	}
	return apiv1.HealthState{
		Name:             StateNameErrorXid,
		Health:           translateToStateHealth(lastHealth),
		Reason:           reason,
		SuggestedActions: lastSuggestedAction,
	}
}

func (xidErr *xidErrorEventDetail) buildMessage(devices map[string]device.Device) string {
	if xidErr == nil {
		// should never happen
		log.Logger.Errorw("buildMessage: xidErrorEventDetail is nil; returning unknown")
		return "unknown"
	}

	header := fmt.Sprintf("XID %d", xidErr.Xid)

	// NVLink (144-150): always show dotted sub-code (even 0) and error status.
	// only 144-150 has subcode information
	if xidErr.Xid >= 144 && xidErr.Xid <= 150 {
		header = fmt.Sprintf("XID %d.%d (err status 0x%08x)", xidErr.Xid, xidErr.SubCode, xidErr.ErrorStatus)
	}

	desc := catalogMnemonicMap[int(xidErr.Xid)]
	if desc == "" {
		// mnemonic identifier not found, use the description
		desc = xidErr.Description
	} else if xidErr.Description != "" && xidErr.Description != "Unused" && desc != xidErr.Description {
		// in addition to mnemonic identifier,
		// we append the description to the mnemonic
		// to make it more readable
		desc += " " + xidErr.Description
	}
	// else: mnemonic exists and Description is empty/Unused/same - just use mnemonic alone

	gpuID := fmt.Sprintf("GPU %s", xidErr.DeviceUUID)
	uuid := convertBusIDToUUID(xidErr.DeviceUUID, devices)
	if uuid != "" {
		gpuID = fmt.Sprintf("GPU %s UUID:%s", xidErr.DeviceUUID, uuid)
	}

	return fmt.Sprintf("%s %s detected on %s", header, desc, gpuID)
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

func resolveXIDEvent(event eventstore.Event, devices map[string]device.Device) eventstore.Event {
	ret := event
	if event.ExtraInfo == nil {
		return ret
	}

	rawData := event.ExtraInfo[EventKeyErrorXidData]

	// First, attempt to unmarshal the new JSON payload format.
	var xidErr xidErrorEventDetail
	if err := json.Unmarshal([]byte(rawData), &xidErr); err == nil && xidErr.Xid != 0 {
		ret = addEventDetails(ret, &xidErr, devices)
		return ret
	}

	// Fallback: legacy format stores only the XID code as a string.
	if currXid, err := strconv.Atoi(rawData); err == nil {
		detail, ok := GetDetail(currXid)
		if !ok {
			return ret
		}

		xidErr := xidErrorEventDetail{
			Time:                   metav1.NewTime(event.Time),
			DataSource:             "kmsg",
			DeviceUUID:             event.ExtraInfo[EventKeyDeviceUUID],
			Xid:                    uint64(currXid),
			SuggestedActionsByGPUd: detail.SuggestedActionsByGPUd,
		}

		ret = addEventDetails(ret, &xidErr, devices)
	}

	return ret
}

// addEventDetails populates event fields/message from parsed XID detail and
// rewrites the stored ExtraInfo payload in JSON form for downstream consumers.
func addEventDetails(ev eventstore.Event, xidErr *xidErrorEventDetail, devices map[string]device.Device) eventstore.Event {
	detail, ok := getDetailWithSubCodeAndStatus(int(xidErr.Xid), xidErr.SubCode, xidErr.ErrorStatus)
	if !ok {
		detail = nil
	}

	if detail != nil {
		// Only set ev.Type from detail if not already set.
		// The event may already have the correct Type from Match() which uses
		// lookupNVLinkRule for precise unit-based matching. The detail from
		// getDetailWithSubCodeAndStatus may have incorrect severity due to
		// merging of different units with the same SubCode/ErrorStatus.
		if ev.Type == "" && detail.EventType != apiv1.EventTypeUnknown {
			ev.Type = string(detail.EventType)
		}
		if xidErr.Description == "" {
			xidErr.Description = detail.Description
		}
		if xidErr.SubCode == 0 {
			xidErr.SubCode = detail.SubCode
		}
		if xidErr.SubCodeDescription == "" {
			xidErr.SubCodeDescription = detail.SubCodeDescription
		}
		if xidErr.SuggestedActionsByGPUd == nil {
			xidErr.SuggestedActionsByGPUd = detail.SuggestedActionsByGPUd
		}
	}

	if detail == nil && ev.Type == "" {
		ev.Type = string(apiv1.EventTypeUnknown)
	}

	ev.Message = xidErr.buildMessage(devices)

	// Ensure time/data source are populated for JSON consumers.
	if xidErr.Time.IsZero() {
		xidErr.Time = metav1.NewTime(ev.Time)
	}
	if xidErr.DataSource == "" {
		xidErr.DataSource = "kmsg"
	}

	if ev.ExtraInfo == nil {
		ev.ExtraInfo = make(map[string]string)
	}
	raw, _ := json.Marshal(xidErr)
	ev.ExtraInfo[EventKeyErrorXidData] = string(raw)

	return ev
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

	// SubCode represents the NVLink sub-code extracted from intrinfo (bits 20-25).
	SubCode int `json:"sub_code,omitempty"`

	// SubCodeDescription provides the NVLink sub-component mnemonic (e.g., RLW_CTRL).
	SubCodeDescription string `json:"sub_code_description,omitempty"`

	// ErrorStatus holds the NVLink error status word (second hex value) used to pick rule-specific severity/actions.
	ErrorStatus uint32 `json:"error_status,omitempty"`

	// InvestigatoryHint is a short hint indicating the investigation focus (e.g., "peer", "software").
	InvestigatoryHint string `json:"investigatory_hint,omitempty"`

	// Description is the human readable XID detail description, including NVLink context when available.
	Description string `json:"description,omitempty"`

	// SuggestedActionsByGPUd are the suggested actions for the error.
	SuggestedActionsByGPUd *apiv1.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`
}
