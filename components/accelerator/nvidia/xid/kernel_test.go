package xid

import (
	"context"
	"errors"
	"time"

	kmsgsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/kmsg"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"

	apiv1 "github.com/leptonai/gpud/api/v1"
	componentkmsg "github.com/leptonai/gpud/components/kmsg"
)

// Existing policy fixtures pass through the same sensor and shim as production.
func kernelReaderForTest(read func(context.Context) ([]apiv1.KernelMessage, error)) func(context.Context) ([]kernelRecord, error) {
	return func(ctx context.Context) ([]kernelRecord, error) {
		sensor, err := kmsgsensor.New(func(ctx context.Context) ([]kmsgsource.Message, error) {
			messages, err := read(ctx)
			source := make([]kmsgsource.Message, 0, len(messages))
			for i, message := range messages {
				at := message.Timestamp.Time
				if at.IsZero() {
					at = time.Unix(1700000000, 0)
				}
				sequence := message.SequenceNumber
				if sequence == 0 {
					sequence = i
				}
				source = append(source, kmsgsource.Message{BootID: "fixture-boot", Sequence: int64(sequence), Timestamp: at, Priority: message.Priority, Body: message.Message})
			}
			return source, err
		}, map[string]string{"gpud.kernel_record": `(?s).*`})
		if err != nil {
			return nil, err
		}
		batch, err := componentkmsg.Measure(ctx, sensor)
		if err != nil {
			return nil, err
		}
		return kernelRecords(batch)
	}
}

func (c *component) startKernelTest(raw <-chan apiv1.KernelMessage, period time.Duration) {
	if raw == nil {
		c.start(nil, period)
		return
	}
	parsed := make(chan kernelRecord)
	go func() {
		defer close(parsed)
		for {
			select {
			case <-c.ctx.Done():
				return
			case message, ok := <-raw:
				if !ok {
					return
				}
				read := kernelReaderForTest(func(context.Context) ([]apiv1.KernelMessage, error) { return []apiv1.KernelMessage{message}, nil })
				records, err := read(c.ctx)
				if err != nil && !errors.Is(err, context.Canceled) {
					panic(err)
				}
				for _, record := range records {
					select {
					case parsed <- record:
					case <-c.ctx.Done():
						return
					}
				}
			}
		}
	}()
	c.start(parsed, period)
}
