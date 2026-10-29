package peermem

import (
	"context"

	"github.com/dsx-ai-factory/health-validation/collect/observation"

	apiv1 "github.com/leptonai/gpud/api/v1"
	componentkmsg "github.com/leptonai/gpud/components/kmsg"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

// observeKernel is this component's observation-to-event shim. Collection is
// supplied by the shared health-validation sensor owned by the GPUd runtime.
func (c *component) observeKernel(subscribe componentkmsg.Observer, match func(string) (string, string), policy eventstore.DedupPolicy) (func(), error) {
	recorder := eventstore.NewRecorder(c.eventBucket, policy)
	return subscribe(c.ctx, func(ctx context.Context, batch *observation.ObservationBatch) {
		messages, err := componentkmsg.Messages(batch)
		if err != nil {
			log.Logger.Errorw("kernel observation conversion failed", "component", Name, "error", err)
		}
		for _, message := range messages {
			if !recorder.AcceptRaw(message.Timestamp.Time, message.Message) {
				continue
			}
			name, body := match(message.Message)
			if name == "" {
				continue
			}
			event := eventstore.Event{Time: message.Timestamp.UTC(), Name: name, Message: body, Type: string(apiv1.EventTypeWarning)}
			if err := recorder.Record(ctx, event); err != nil {
				log.Logger.Errorw("failed to record kernel event", "component", Name, "error", err)
			}
		}
	})
}
