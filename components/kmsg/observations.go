package kmsg

import (
	"errors"
	"fmt"

	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsignals "github.com/dsx-ai-factory/health-validation/collect/signals/kmsg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

type recordID struct {
	bootID   string
	sequence int64
}

// Messages is the shared kernel-record shim for GPUd components.
// It preserves encounter order and keeps partial data separate from failures.
func Messages(batch *observation.ObservationBatch) ([]apiv1.KernelMessage, error) {
	if batch == nil {
		return nil, fmt.Errorf("kmsg sensor returned no observation batch")
	}
	type entry struct {
		id             recordID
		message        apiv1.KernelMessage
		body, priority bool
	}
	entries := make([]entry, 0, len(batch.Observations)/2)
	indexes := make(map[recordID]int)
	var resultErr error
	for _, reading := range batch.Observations {
		if failure := reading.GetCollectionError(); failure != nil {
			if resultErr == nil {
				resultErr = fmt.Errorf("kmsg collection %s: %s", failure.Category, failure.Detail)
			}
			continue
		}
		target := reading.GetTarget().GetKernelRecord()
		if target.GetBootId() == "" || target.GetSequence() < 0 || reading.GetObservedAt() == nil || reading.GetObservedAt().CheckValid() != nil {
			return nil, fmt.Errorf("kmsg observation requires boot identity, sequence, and timestamp")
		}
		id := recordID{bootID: target.GetBootId(), sequence: target.GetSequence()}
		sequence := int(id.sequence)
		if int64(sequence) != id.sequence {
			return nil, fmt.Errorf("kmsg sequence number %d overflows int", id.sequence)
		}
		index, found := indexes[id]
		if !found {
			index = len(entries)
			indexes[id] = index
			entries = append(entries, entry{id: id, message: apiv1.KernelMessage{BootID: id.bootID, SequenceNumber: sequence, Timestamp: metav1.NewTime(reading.ObservedAt.AsTime())}})
		}
		record := &entries[index]
		switch reading.SignalId {
		case kmsgsignals.Message.ID():
			value, ok := reading.GetValue().GetKind().(*observation.Value_StringValue)
			if !ok || record.body {
				return nil, fmt.Errorf("kmsg record %v has invalid or duplicate message", id)
			}
			record.message.Message, record.body = value.StringValue, true
		case kmsgsignals.Priority.ID():
			value, ok := reading.GetValue().GetKind().(*observation.Value_IntValue)
			if !ok || record.priority || int64(int(value.IntValue)) != value.IntValue {
				return nil, fmt.Errorf("kmsg record %v has invalid or duplicate priority", id)
			}
			record.message.Priority, record.priority = int(value.IntValue), true
		default:
			return nil, fmt.Errorf("unexpected kmsg signal %q", reading.SignalId)
		}
	}
	records := make([]apiv1.KernelMessage, 0, len(entries))
	for _, entry := range entries {
		if !entry.body || !entry.priority {
			resultErr = errors.Join(resultErr, fmt.Errorf("incomplete kmsg record %v", entry.id))
			continue
		}
		records = append(records, entry.message)
	}
	return records, resultErr
}
