package xid

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/dsx-ai-factory/health-validation/collect/observation"
	xidsignals "github.com/dsx-ai-factory/health-validation/collect/signals/xid"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
)

// kernelRecord is the component's conversion of one sensor record. The raw
// message remains available for event storage and content deduplication.
type kernelRecord struct {
	apiv1.KernelMessage
	xid *Error
}

type kernelID struct {
	boot     string
	sequence int64
}

// kernelRecords joins parsed Xid facts to their original kernel records. It
// performs no collection and never reparses a log line to extract Xid fields.
func kernelRecords(batch *observation.ObservationBatch) ([]kernelRecord, error) {
	messages, resultErr := components.KernelMessages(batch)
	if batch == nil {
		return nil, resultErr
	}
	facts := make(map[kernelID]map[string]*observation.Observation)
	failed := make(map[kernelID]bool)
	for _, o := range batch.Observations {
		switch o.GetSignalId() {
		case xidsignals.Code.ID(), xidsignals.PCIAddress.ID(), xidsignals.NVLink5SubComponent.ID(), xidsignals.NVLink5Fatal.ID(), xidsignals.NVLink5IntrInfo.ID(), xidsignals.NVLink5ErrorStatus.ID():
		default:
			continue
		}
		target := o.GetTarget().GetKernelRecord()
		// Untargeted source failures are already handled by KernelMessages.
		if target == nil {
			continue
		}
		id := kernelID{boot: target.BootId, sequence: target.Sequence}
		if failure := o.GetCollectionError(); failure != nil {
			if !failed[id] {
				resultErr = errors.Join(resultErr, fmt.Errorf("xid record %v: %s", id, failure.Detail))
			}
			failed[id] = true
			continue
		}
		if facts[id] == nil {
			facts[id] = make(map[string]*observation.Observation)
		}
		if facts[id][o.SignalId] != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("duplicate Xid field %s for %v", o.SignalId, id))
			failed[id] = true
		}
		facts[id][o.SignalId] = o
	}
	records := make([]kernelRecord, 0, len(messages))
	for _, message := range messages {
		id := kernelID{boot: message.BootID, sequence: int64(message.SequenceNumber)}
		record := kernelRecord{KernelMessage: message}
		if fields := facts[id]; len(fields) > 0 && !failed[id] {
			var err error
			record.xid, err = xidFromFields(fields)
			if err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("xid record %v: %w", id, err))
			}
		} else if !failed[id] {
			record.xid = fallenOffBusError(message.Message)
		}
		records = append(records, record)
	}
	return records, resultErr
}

func xidFromFields(fields map[string]*observation.Observation) (*Error, error) {
	code, ok := fields[xidsignals.Code.ID()].GetValue().GetKind().(*observation.Value_IntValue)
	if !ok {
		return nil, fmt.Errorf("missing or invalid Xid code")
	}
	address, ok := fields[xidsignals.PCIAddress.ID()].GetValue().GetKind().(*observation.Value_StringValue)
	if !ok || address.StringValue == "" {
		return nil, fmt.Errorf("missing or invalid PCI address")
	}
	var extended *nvlinkInfo
	if field := fields[xidsignals.NVLink5SubComponent.ID()]; field != nil {
		unit, ok := field.GetValue().GetKind().(*observation.Value_StringValue)
		if !ok {
			return nil, fmt.Errorf("invalid NVLink subcomponent")
		}
		fatal, ok := fields[xidsignals.NVLink5Fatal.ID()].GetValue().GetKind().(*observation.Value_BoolValue)
		if !ok {
			return nil, fmt.Errorf("missing or invalid NVLink fatal flag")
		}
		// Missing or non-register tails use GPUd's base Xid policy, as before.
		intr, intrErr := strconv.ParseUint(fields[xidsignals.NVLink5IntrInfo.ID()].GetValue().GetStringValue(), 0, 32)
		status, statusErr := strconv.ParseUint(fields[xidsignals.NVLink5ErrorStatus.ID()].GetValue().GetStringValue(), 0, 32)
		if intrErr == nil && statusErr == nil {
			severity := "Nonfatal"
			if fatal.BoolValue {
				severity = "Fatal"
			}
			extended = &nvlinkInfo{SubCodeName: unit.StringValue, Severity: severity, Intrinfo: uint32(intr), ErrorStatus: uint32(status), SubCode: calculateSubCode(uint32(intr))}
		}
	}
	return errorFromFacts(code.IntValue, address.StringValue, extended), nil
}
