package xid

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/kmsg"
	kmsgsignals "github.com/dsx-ai-factory/health-validation/collect/signals/kmsg"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
)

func TestKernelSensorDeliveryAndClose(t *testing.T) {
	c := &component{ctx: t.Context()}
	var consume func(context.Context, *observation.ObservationBatch)
	stops := 0
	messages, stop, err := c.observeKernel(func(_ context.Context, callback func(context.Context, *observation.ObservationBatch)) (func(), error) {
		consume = callback
		return func() { stops++ }, nil
	})
	require.NoError(t, err)
	defer stop()
	at := time.Unix(1700000000, 0).UTC()
	const line = "NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus."
	batch := kernelDeliveryBatch(t, []kmsgsource.Message{
		{BootID: "boot", Sequence: 71, Timestamp: at, Priority: 3, Body: line},
		{BootID: "boot", Sequence: 72, Timestamp: at, Priority: 6, Body: "ordinary kernel record"},
	})
	consume(t.Context(), batch)
	consume(t.Context(), batch)
	require.Len(t, messages, 2, "replayed observations must not duplicate queued events")
	first := <-messages
	assert.Equal(t, line, first.Message)
	assert.Equal(t, 71, first.SequenceNumber)
	assert.Equal(t, 3, first.Priority)
	assert.Equal(t, at, first.Timestamp.Time)
	require.NotNil(t, first.xid)
	assert.Equal(t, 79, first.xid.Xid)
	assert.Equal(t, "ordinary kernel record", (<-messages).Message)
	stop()
	stop()
	assert.Equal(t, 1, stops)
	_, open := <-messages
	assert.False(t, open)
}

func TestKernelSensorSubscriptionFailure(t *testing.T) {
	failure := errors.New("cannot open kernel source")
	c := &component{ctx: t.Context()}
	messages, stop, err := c.observeKernel(func(context.Context, func(context.Context, *observation.ObservationBatch)) (func(), error) {
		return nil, failure
	})
	require.ErrorIs(t, err, failure)
	assert.Nil(t, messages)
	assert.Nil(t, stop)
}

func TestKernelSensorCancellationUnblocksFullQueue(t *testing.T) {
	c := &component{ctx: t.Context()}
	var consume func(context.Context, *observation.ObservationBatch)
	messages, stop, err := c.observeKernel(func(_ context.Context, callback func(context.Context, *observation.ObservationBatch)) (func(), error) {
		consume = callback
		return func() {}, nil
	})
	require.NoError(t, err)
	records := make([]kmsgsource.Message, cap(messages)+1)
	for i := range records {
		records[i] = kmsgsource.Message{BootID: "boot", Sequence: int64(i), Timestamp: time.Unix(int64(1700000000+i*60), 0), Priority: 6, Body: "record"}
	}
	consume(t.Context(), kernelDeliveryBatch(t, records[:cap(messages)]))
	require.Len(t, messages, cap(messages))
	batch := kernelDeliveryBatch(t, records[cap(messages):])
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan struct{})
	go func() {
		consume(ctx, batch)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled delivery blocked on a full component queue")
	}
	stop()
}

func kernelDeliveryBatch(t *testing.T, records []kmsgsource.Message) *observation.ObservationBatch {
	t.Helper()
	sensor, err := kmsgsensor.New(func(context.Context) ([]kmsgsource.Message, error) {
		return records, errors.New("partial kernel read")
	}, map[string]string{"gpud.kernel_record": `(?s).*`})
	require.NoError(t, err)
	batch, err := components.MeasureSensor(t.Context(), sensor, kmsgsignals.Message.ID(), kmsgsignals.Priority.ID())
	require.NoError(t, err)
	return batch
}
