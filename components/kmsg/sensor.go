package kmsg

import (
	"context"
	"fmt"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect"
	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsignals "github.com/dsx-ai-factory/health-validation/collect/signals/kmsg"
)

// Measure collects raw kernel message and priority observations for a GPUd cycle.
func Measure(ctx context.Context, sensor collect.Sensor) (*observation.ObservationBatch, error) {
	now := time.Now()
	cycle, err := collect.NewCycle("gpud."+sensor.ID()+"."+now.Format(time.RFC3339Nano), now)
	if err != nil {
		return nil, err
	}
	batch, err := sensor.Measure(ctx, collect.WithCycle(cycle), collect.WithSignalID(kmsgsignals.Message.ID(), kmsgsignals.Priority.ID()))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if batch == nil {
		return nil, fmt.Errorf("sensor %s returned no observations", sensor.ID())
	}
	return batch, nil
}
