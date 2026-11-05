package xid

import (
	"context"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/kmsg"
)

func readLegacyKernelMessages(ctx context.Context) ([]kernelRecord, error) {
	messages, err := kmsg.ReadAllWithOptions(ctx, kmsg.WithExactDedupMatch(isRecoveryMessage))
	records := make([]kernelRecord, 0, len(messages))
	for _, message := range messages {
		records = append(records, kernelRecord{KernelMessage: apiv1.KernelMessage{Timestamp: message.Timestamp, Priority: message.Priority, SequenceNumber: message.SequenceNumber, Message: message.Message}, xid: Match(message.Message)})
	}
	return records, err
}

func (c *component) convertLegacyKernel(input <-chan apiv1.KernelMessage) <-chan kernelRecord {
	records := make(chan kernelRecord)
	go func() {
		defer close(records)
		for {
			select {
			case <-c.ctx.Done():
				return
			case message, ok := <-input:
				if !ok {
					return
				}
				record := kernelRecord{KernelMessage: message, xid: Match(message.Message)}
				select {
				case records <- record:
				case <-c.ctx.Done():
					return
				}
			}
		}
	}()
	return records
}
