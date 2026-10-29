package kmsg

import (
	"context"

	apiv1 "github.com/leptonai/gpud/api/v1"
	legacy "github.com/leptonai/gpud/pkg/kmsg"
)

func legacyMessage(message legacy.Message) apiv1.KernelMessage {
	return apiv1.KernelMessage{Timestamp: message.Timestamp, Priority: message.Priority, SequenceNumber: message.SequenceNumber, Message: message.Message}
}

// ReadLegacy preserves the original one-shot collector for the default mode.
func ReadLegacy(ctx context.Context) ([]apiv1.KernelMessage, error) {
	messages, err := legacy.ReadAll(ctx)
	records := make([]apiv1.KernelMessage, 0, len(messages))
	for _, message := range messages {
		records = append(records, legacyMessage(message))
	}
	return records, err
}

// WatchLegacy forwards the original watcher's records to component event loops.
// The component remains responsible for closing the watcher.
func WatchLegacy(ctx context.Context, watcher legacy.Watcher) (<-chan apiv1.KernelMessage, error) {
	input, err := watcher.Watch()
	if err != nil {
		return nil, err
	}
	records := make(chan apiv1.KernelMessage)
	go func() {
		defer close(records)
		for {
			select {
			case <-ctx.Done():
				return
			case message, ok := <-input:
				if !ok {
					return
				}
				select {
				case records <- legacyMessage(message):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return records, nil
}
