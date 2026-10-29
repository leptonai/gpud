package components

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsignals "github.com/dsx-ai-factory/health-validation/collect/signals/kmsg"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

type kernelFixture struct {
	mu      sync.Mutex
	records []kmsgsource.Message
	err     error
	reads   atomic.Int32
	closes  atomic.Int32
}

func (s *kernelFixture) read(context.Context) ([]kmsgsource.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads.Add(1)
	return append([]kmsgsource.Message(nil), s.records...), s.err
}
func (s *kernelFixture) instance(t *testing.T) *GPUdInstance {
	t.Helper()
	sensor, err := newKernelTestSensor(s.read)
	require.NoError(t, err)
	return &GPUdInstance{RootCtx: t.Context(), kernel: &sensorCycle{sensor: sensor, close: func() error { s.closes.Add(1); return nil }}}
}
func receiveKernel(t *testing.T, ch <-chan apiv1.KernelMessage) apiv1.KernelMessage {
	t.Helper()
	select {
	case record := <-ch:
		return record
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for component observation")
		return apiv1.KernelMessage{}
	}
}
func kernelConsumer(t *testing.T, ch chan<- apiv1.KernelMessage) func(context.Context, *observation.ObservationBatch) {
	return func(ctx context.Context, batch *observation.ObservationBatch) {
		records, err := KernelMessages(batch)
		assert.NoError(t, err)
		for _, r := range records {
			select {
			case ch <- r:
			case <-ctx.Done():
				return
			}
		}
	}
}

func TestObserveKernelIndependentComponents(t *testing.T) {
	source := &kernelFixture{records: []kmsgsource.Message{kernelTestRecord(0, "same"), kernelTestRecord(1, "same")}}
	g := source.instance(t)
	slow, fast := make(chan apiv1.KernelMessage, 1), make(chan apiv1.KernelMessage, 8)
	stopSlow, err := g.ObserveKernel(t.Context(), kernelConsumer(t, slow))
	require.NoError(t, err)
	t.Cleanup(stopSlow)
	stopFast, err := g.ObserveKernel(t.Context(), kernelConsumer(t, fast))
	require.NoError(t, err)
	t.Cleanup(stopFast)
	// Content dedup belongs to component policy: collection preserves both bodies.
	assert.Equal(t, 0, receiveKernel(t, fast).SequenceNumber)
	assert.Equal(t, 1, receiveKernel(t, fast).SequenceNumber)
	require.Eventually(t, func() bool { return len(slow) == 1 }, 5*time.Second, time.Millisecond)
	source.mu.Lock()
	source.records = append(source.records, kernelTestRecord(2, "new"))
	source.mu.Unlock()
	assert.Equal(t, 2, receiveKernel(t, fast).SequenceNumber)
	// The slow callback is blocked on delivery; cancellation must release it.
	done := make(chan struct{})
	go func() { stopSlow(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stop blocked on component delivery")
	}
	assert.Zero(t, source.closes.Load())
	late := make(chan apiv1.KernelMessage, 8)
	stopLate, err := g.ObserveKernel(t.Context(), kernelConsumer(t, late))
	require.NoError(t, err)
	t.Cleanup(stopLate)
	for i := range 3 {
		assert.Equal(t, i, receiveKernel(t, late).SequenceNumber)
	}
	stopFast()
	assert.Zero(t, source.closes.Load())
	stopLate()
	assert.EqualValues(t, 1, source.closes.Load())
	stopLate()
	assert.EqualValues(t, 1, source.closes.Load())
	require.Nil(t, g.kernel)
}

func TestObserveKernelSharesPartialFailures(t *testing.T) {
	source := &kernelFixture{records: []kmsgsource.Message{kernelTestRecord(9, "retained")}, err: errors.New("read failed")}
	g := source.instance(t)
	// Block the first callback until both subscriptions have acquired the source.
	release := make(chan struct{})
	received := make(chan apiv1.KernelMessage, 2)
	consume := func(ctx context.Context, batch *observation.ObservationBatch) {
		select {
		case <-release:
		case <-ctx.Done():
			return
		}
		records, err := KernelMessages(batch)
		assert.ErrorContains(t, err, "read failed")
		if assert.Len(t, records, 1) {
			received <- records[0]
		}
	}
	stop1, err := g.ObserveKernel(t.Context(), consume)
	require.NoError(t, err)
	t.Cleanup(stop1)
	stop2, err := g.ObserveKernel(t.Context(), consume)
	require.NoError(t, err)
	t.Cleanup(stop2)
	close(release)
	for range 2 {
		assert.Equal(t, "retained", receiveKernel(t, received).Message)
	}
	stop1()
	stop2()
	assert.EqualValues(t, 1, source.reads.Load())
	assert.EqualValues(t, 1, source.closes.Load())
	require.Nil(t, g.kernel)
}

func TestKernelSensorSerializesMeasurement(t *testing.T) {
	source := &kernelFixture{records: []kmsgsource.Message{kernelTestRecord(1, "shared")}}
	g := source.instance(t)
	_, err := g.kernel.measure(t.Context())
	require.NoError(t, err)
	g.kernel.next = time.Now().Add(time.Hour)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			batch, err := g.kernel.measure(t.Context())
			assert.NoError(t, err)
			assert.Len(t, batch.Observations, 2)
		})
	}
	wg.Wait()
	assert.EqualValues(t, 1, source.reads.Load())
}

func TestObserveKernelOpenFailureAndCancellation(t *testing.T) {
	mockey.PatchConvey("source errors leave no active subscription", t, func() {
		failure := errors.New("cannot open kernel stream")
		mockey.Mock(kmsgsource.NewStream).To(func(int) (*kmsgsource.Stream, error) { return nil, failure }).Build()
		g := &GPUdInstance{RootCtx: t.Context()}
		stop, err := g.ObserveKernel(t.Context(), nil)
		require.ErrorIs(t, err, failure)
		assert.Nil(t, stop)
		assert.Zero(t, g.kernelUsers)
		assert.Nil(t, g.kernel)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = g.ObserveKernel(ctx, nil)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestFreshKernelObservationsPreservesPairsAndBootIdentity(t *testing.T) {
	source := &kernelFixture{records: []kmsgsource.Message{kernelTestRecord(0, "first"), kernelTestRecord(1, "next")}}
	sensor, err := newKernelTestSensor(source.read)
	require.NoError(t, err)
	batch, err := MeasureSensor(t.Context(), sensor, kmsgsignals.Message.ID(), kmsgsignals.Priority.ID())
	require.NoError(t, err)
	seen := make(map[string]int64)
	first := freshKernelObservations(batch, seen)
	assert.Len(t, first.Observations, len(batch.Observations))
	assert.Empty(t, freshKernelObservations(batch, seen).Observations)
	next := kernelTestRecord(0, "new boot")
	next.BootID = "boot-b"
	source.records = []kmsgsource.Message{next}
	batch, err = MeasureSensor(t.Context(), sensor, kmsgsignals.Message.ID(), kmsgsignals.Priority.ID())
	require.NoError(t, err)
	assert.NotEmpty(t, freshKernelObservations(batch, seen).Observations)
}
