package sxid

import (
	"context"
	"sync"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect/observation"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

// observeKernel converts shared sensor observations for this component's event
// loop. Native and NVSentinel events continue to use the same GPUd health policy.
func (c *component) observeKernel(subscribe components.KernelObserver) (<-chan apiv1.KernelMessage, func(), error) {
	messages := make(chan apiv1.KernelMessage, 2048)
	dedup := eventstore.NewDeduplicator(time.Minute)
	stop, err := subscribe(c.ctx, func(ctx context.Context, batch *observation.ObservationBatch) {
		records, err := components.KernelMessages(batch)
		if err != nil {
			log.Logger.Errorw("kernel observation conversion failed", "component", Name, "error", err)
		}
		for _, record := range records {
			if !dedup.Accept(record.Timestamp.Time, record.Message) {
				continue
			}
			select {
			case messages <- record:
			case <-ctx.Done():
				return
			}
		}
	})
	if err != nil {
		return nil, nil, err
	}
	var once sync.Once
	return messages, func() { once.Do(func() { stop(); close(messages) }) }, nil
}
