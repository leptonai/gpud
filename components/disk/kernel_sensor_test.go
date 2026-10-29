package disk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect/observation"
	xidsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/xid"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestKernelSensorPersistsPartialBatchAndDeduplicatesReplay(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)
	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()
	c := &component{ctx: t.Context(), eventBucket: bucket}
	at := time.Date(2026, 10, 29, 1, 0, 0, 0, time.UTC)
	sensor, err := xidsensor.NewKmsg(func(context.Context) ([]kmsgsource.Message, error) {
		return []kmsgsource.Message{
			{BootID: "boot", Sequence: 1, Timestamp: at, Priority: 3, Body: "EXT4-fs (md0): Remounting filesystem read-only"},
			{BootID: "boot", Sequence: 2, Timestamp: at, Priority: 6, Body: "unrelated kernel record"},
			{BootID: "boot", Sequence: 3, Timestamp: at, Priority: 3, Body: "NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus."},
		}, errors.New("partial kernel read")
	})
	require.NoError(t, err)
	var ids []string
	for _, signal := range sensor.Signals() {
		ids = append(ids, signal.ID())
	}
	batch, err := components.MeasureSensor(t.Context(), sensor, ids...)
	require.NoError(t, err)
	_, err = components.KernelMessages(batch)
	require.ErrorContains(t, err, "partial kernel read")

	// Replay within one subscription, then after the in-memory dedup state is lost.
	for range 2 {
		var consume func(context.Context, *observation.ObservationBatch)
		stopped := false
		stop, err := c.observeKernel(func(_ context.Context, callback func(context.Context, *observation.ObservationBatch)) (func(), error) {
			consume = callback
			return func() { stopped = true }, nil
		}, Match, eventstore.DedupPolicy{Window: 5 * time.Minute})
		require.NoError(t, err)
		consume(t.Context(), batch)
		consume(t.Context(), batch)
		stop()
		require.True(t, stopped)
	}
	events, err := bucket.Get(t.Context(), time.Unix(0, 0))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, eventFilesystemReadOnly, events[0].Name)
	assert.Equal(t, messageFilesystemReadOnly, events[0].Message)
	assert.Equal(t, string(apiv1.EventTypeWarning), events[0].Type)
	assert.True(t, at.Equal(events[0].Time))
}
