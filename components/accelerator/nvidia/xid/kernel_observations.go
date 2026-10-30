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
		records = append(records, record)
	}
	return records, resultErr
}

func xidFromEvent(event xidsignals.Event) *Error {
	var extended *nvlinkInfo
	if detail := event.NVLink5; detail != nil {
		// Missing or non-register tails use GPUd's base Xid policy, as before.
		intr, intrErr := strconv.ParseUint(detail.IntrInfo, 0, 32)
		status, statusErr := strconv.ParseUint(detail.ErrorStatus, 0, 32)
		if intrErr == nil && statusErr == nil {
			severity := "Nonfatal"
			if detail.Fatal {
				severity = "Fatal"
			}
			extended = &nvlinkInfo{SubCodeName: detail.SubComponent, Severity: severity, Intrinfo: uint32(intr), ErrorStatus: uint32(status), SubCode: calculateSubCode(uint32(intr))}
		}
	}
	return errorFromFacts(event.Code, event.PCIAddress, extended)
}
