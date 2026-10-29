package kmsg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect"
	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/kmsg"
	kmsgsignals "github.com/dsx-ai-factory/health-validation/collect/signals/kmsg"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Select every record, including empty and multiline bodies. Component matchers
// need continuation lines that would be lost by filtering only event starts.
func newSensor(read func(context.Context) ([]kmsgsource.Message, error)) (collect.Sensor, error) {
	return kmsgsensor.New(read, map[string]string{"gpud.kernel_record": `(?s).*`})
}

type recordID struct {
	bootID   string
	sequence int64
}

func collectMessages(ctx context.Context, sensor collect.Sensor) ([]Message, error) {
	now := time.Now()
	cycle, err := collect.NewCycle("gpud.kmsg."+now.Format(time.RFC3339Nano), now)
	if err != nil {
		return nil, err
	}
	batch, err := sensor.Measure(ctx, collect.WithCycle(cycle), collect.WithSignalID(kmsgsignals.Message.ID(), kmsgsignals.Priority.ID()))
	if err != nil {
		return nil, err
	}
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return messagesFromObservations(batch)
}

// messagesFromObservations is the shared kernel-record shim for GPUd components.
// It preserves encounter order and keeps partial data separate from failures.
func messagesFromObservations(batch *observation.ObservationBatch) ([]Message, error) {
	if batch == nil {
		return nil, fmt.Errorf("kmsg sensor returned no observation batch")
	}
	type entry struct {
		id             recordID
		message        Message
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
			entries = append(entries, entry{id: id, message: Message{bootID: id.bootID, SequenceNumber: sequence, Timestamp: metav1.NewTime(reading.ObservedAt.AsTime())}})
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
	records := make([]Message, 0, len(entries))
	for _, entry := range entries {
		if !entry.body || !entry.priority {
			resultErr = errors.Join(resultErr, fmt.Errorf("incomplete kmsg record %v", entry.id))
			continue
		}
		records = append(records, entry.message)
	}
	return records, resultErr
}
