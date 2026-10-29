package xid

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/dsx-ai-factory/health-validation/collect/observation"
	xidsignals "github.com/dsx-ai-factory/health-validation/collect/signals/xid"

	apiv1 "github.com/leptonai/gpud/api/v1"
	componentkmsg "github.com/leptonai/gpud/components/kmsg"
)

// kernelRecord retains the raw message for event storage and content deduplication.
type kernelRecord struct {
	apiv1.KernelMessage
	xid *Error
}

// kernelRecords interprets shared raw observations using the library's Xid parser.
// Parse failures stay local to this component and preserve the original record.
func kernelRecords(batch *observation.ObservationBatch) ([]kernelRecord, error) {
	messages, resultErr := componentkmsg.Messages(batch)
	records := make([]kernelRecord, 0, len(messages))
	for _, message := range messages {
		record := kernelRecord{KernelMessage: message}
		if xidsignals.Match(message.Message) {
			event, err := xidsignals.Parse(message.Message)
			if err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("xid record %s/%d: %w", message.BootID, message.SequenceNumber, err))
			} else {
				record.xid = xidFromEvent(event)
			}
		} else {
			record.xid = fallenOffBusError(message.Message)
		}
		if record.xid != nil && record.xid.Xid == 154 {
			record.xid.RecoveryAction.Raw = message.Message
		}
		records = append(records, record)
	}
	return records, resultErr
}

func xidFromEvent(event xidsignals.Event) *Error {
	var extended *ExtractedInfo
	if detail := event.NVLink5; detail != nil {
		// Missing or non-register tails use GPUd's base Xid policy, as before.
		intr, intrErr := strconv.ParseUint(detail.IntrInfo, 0, 32)
		status, statusErr := strconv.ParseUint(detail.ErrorStatus, 0, 32)
		if intrErr == nil && statusErr == nil {
			severity := "Nonfatal"
			if detail.Fatal {
				severity = "Fatal"
			}
			extended = &ExtractedInfo{SubCodeName: detail.SubComponent, Severity: severity, Intrinfo: uint32(intr), ErrorStatus: uint32(status), SubCode: calculateSubCode(uint32(intr))}
		}
	}
	parsed := errorFromFacts(event.Code, event.PCIAddress, extended)
	if parsed != nil && parsed.Xid == 154 {
		transition := &RecoveryActionTransition{Raw: event.Message}
		if action := event.RecoveryAction; action != nil {
			from, fromErr := strconv.ParseUint(action.FromCode, 0, 32)
			to, toErr := strconv.ParseUint(action.ToCode, 0, 32)
			transition.FromName, transition.ToName = action.FromName, action.ToName
			if fromErr == nil && toErr == nil {
				transition.From, transition.To, transition.Valid = uint32(from), uint32(to), true
			}
		}
		parsed.RecoveryAction = transition
		parsed.Detail = recoveryDetail(transition)
	}
	return parsed
}

// errorFromFacts applies GPUd's catalog and recovery policy to parsed facts.
func errorFromFacts(code int64, address string, extended *ExtractedInfo) *Error {
	xid := int(code)
	if int64(xid) != code {
		return nil
	}
	detail, ok := GetDetail(xid)
	if !ok {
		return nil
	}
	if extended != nil {
		extended.Xid = xid
		if decoded, ok := detailFromNVLinkInfo(extended); ok {
			detail = decoded
		}
	}
	return &Error{Xid: xid, DeviceUUID: "PCI:" + address, Detail: detail}
}

// A fallen-off-bus line without an explicit Xid is interpreted as Xid 79 by
// GPUd policy. It is not a parsed Xid fact from the sensor.
func fallenOffBusError(line string) *Error {
	code, device := extractFallenOffBusXidInfo(line)
	if code == 0 {
		return nil
	}
	detail, ok := GetDetail(code)
	if !ok {
		return nil
	}
	return &Error{Xid: code, DeviceUUID: device, Detail: detail}
}

// dedupKey preserves distinct recovery transitions while coalescing ordinary faults.
// The kernel identity suppresses replay without collapsing request -> None -> request.
func (r kernelRecord) dedupKey() string {
	if r.xid != nil && r.xid.Xid == 154 {
		return fmt.Sprintf("%s/%d/%d/%s", r.BootID, r.Timestamp.UnixNano(), r.SequenceNumber, r.Message)
	}
	return r.Message
}
